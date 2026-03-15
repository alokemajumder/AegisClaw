package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
)

// Orchestrator subscribes to run triggers and dispatches them to the RunEngine.
type Orchestrator struct {
	engine      *RunEngine
	consumer    *natspkg.Consumer
	runs        *repository.RunRepo
	engagements *repository.EngagementRepo
	approvals   *repository.ApprovalRepo
	killSwitch  *KillSwitch
	logger      *slog.Logger
	mu          sync.Mutex
	cancelFns   map[uuid.UUID]context.CancelFunc
}

// NewOrchestrator creates a new orchestrator.
func NewOrchestrator(
	engine *RunEngine,
	consumer *natspkg.Consumer,
	runs *repository.RunRepo,
	engagements *repository.EngagementRepo,
	approvals *repository.ApprovalRepo,
	killSwitch *KillSwitch,
	logger *slog.Logger,
) *Orchestrator {
	return &Orchestrator{
		engine:      engine,
		consumer:    consumer,
		runs:        runs,
		engagements: engagements,
		approvals:   approvals,
		killSwitch:  killSwitch,
		logger:      logger,
		cancelFns:   make(map[uuid.UUID]context.CancelFunc),
	}
}

// Start subscribes to NATS topics and begins processing.
func (o *Orchestrator) Start(ctx context.Context) error {
	// Subscribe to run triggers
	_, err := o.consumer.Subscribe(ctx, natspkg.StreamRuns, "orchestrator-triggers", natspkg.SubjectRunTrigger, o.handleRunTrigger)
	if err != nil {
		return err
	}

	// Subscribe to kill switch
	_, err = o.consumer.Subscribe(ctx, natspkg.StreamRuns, "orchestrator-killswitch", natspkg.SubjectKillSwitch, o.handleKillSwitch)
	if err != nil {
		return err
	}

	// Subscribe to approval-granted events to resume blocked steps
	_, err = o.consumer.Subscribe(ctx, natspkg.StreamApprovals, "orchestrator-approvals", natspkg.SubjectApprovalGranted, o.handleApprovalGranted)
	if err != nil {
		return err
	}

	o.logger.Info("orchestrator started, listening for run triggers")
	return nil
}

func (o *Orchestrator) handleRunTrigger(ctx context.Context, data []byte) error {
	env, err := natspkg.DecodeEnvelope[natspkg.RunTriggerMsg](data)
	if err != nil {
		o.logger.Error("decoding run trigger", "error", err)
		return err
	}

	msg := env.Payload
	o.logger.Info("received run trigger",
		"engagement_id", msg.EngagementID,
		"triggered_by", msg.TriggeredBy,
	)

	if o.killSwitch.IsEngaged() {
		o.logger.Warn("kill switch engaged, rejecting run trigger")
		return nil
	}

	eng, err := o.engagements.GetByID(ctx, msg.EngagementID)
	if err != nil {
		o.logger.Error("engagement not found", "id", msg.EngagementID, "error", err)
		return nil
	}

	// Find queued runs for this engagement
	runs, _, err := o.runs.ListByEngagementID(ctx, msg.EngagementID, models.PaginationParams{Page: 1, PerPage: 100})
	if err != nil {
		o.logger.Error("listing runs", "error", err)
		return nil
	}

	// Find the most recent queued run
	var targetRun *models.Run
	for i := range runs {
		if runs[i].Status == models.RunQueued {
			targetRun = &runs[i]
			break
		}
	}

	if targetRun == nil {
		// Create a new run
		maxTier := 0
		for _, t := range eng.AllowedTiers {
			if t > maxTier {
				maxTier = t
			}
		}
		targetRun = &models.Run{
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			Status:       models.RunQueued,
			Tier:         maxTier,
			Metadata:     json.RawMessage(`{}`),
		}
		if err := o.runs.Create(ctx, targetRun); err != nil {
			o.logger.Error("creating run", "error", err)
			return nil
		}
	}

	// Execute run in background
	runCtx, cancelFn := context.WithCancel(ctx)
	o.mu.Lock()
	o.cancelFns[targetRun.ID] = cancelFn
	o.mu.Unlock()

	go func() {
		defer func() {
			o.mu.Lock()
			delete(o.cancelFns, targetRun.ID)
			o.mu.Unlock()
		}()

		if err := o.engine.ExecuteRun(runCtx, targetRun); err != nil {
			o.logger.Error("run execution failed", "run_id", targetRun.ID, "error", err)
		}
	}()

	return nil
}

func (o *Orchestrator) handleApprovalGranted(ctx context.Context, data []byte) error {
	env, err := natspkg.DecodeEnvelope[natspkg.ApprovalGrantedMsg](data)
	if err != nil {
		o.logger.Error("decoding approval granted message", "error", err)
		return err
	}

	msg := env.Payload
	o.logger.Info("received approval granted",
		"run_id", msg.RunID,
		"step_number", msg.StepNumber,
		"approval_id", msg.ApprovalID,
	)

	run, err := o.runs.GetByID(ctx, msg.RunID)
	if err != nil {
		o.logger.Error("loading run for approved step", "run_id", msg.RunID, "error", err)
		return nil
	}

	eng, err := o.engagements.GetByID(ctx, msg.EngagementID)
	if err != nil {
		o.logger.Error("loading engagement for approved step", "engagement_id", msg.EngagementID, "error", err)
		return nil
	}

	// SECURITY: Verify the approval has not expired before executing.
	if o.approvals != nil {
		approval, err := o.approvals.GetByID(ctx, msg.ApprovalID)
		if err != nil {
			o.logger.Error("failed to load approval record — blocking execution",
				"approval_id", msg.ApprovalID, "error", err)
			return fmt.Errorf("approval record not found: %w", err)
		}
		if approval.ExpiresAt != nil && time.Now().UTC().After(*approval.ExpiresAt) {
			o.logger.Warn("approval has expired — refusing to execute step",
				"approval_id", msg.ApprovalID,
				"expires_at", approval.ExpiresAt,
				"run_id", msg.RunID,
				"step_number", msg.StepNumber,
			)
			return nil
		}
		if approval.Status != models.ApprovalApproved {
			o.logger.Warn("approval status is not approved — refusing to execute step",
				"approval_id", msg.ApprovalID,
				"status", approval.Status,
			)
			return nil
		}
	}

	go func() {
		if err := o.engine.ExecuteApprovedStep(ctx, run, eng, msg.StepNumber); err != nil {
			o.logger.Error("executing approved step failed", "run_id", msg.RunID, "step_number", msg.StepNumber, "error", err)
		}
	}()

	return nil
}

func (o *Orchestrator) handleKillSwitch(ctx context.Context, data []byte) error {
	env, err := natspkg.DecodeEnvelope[natspkg.KillSwitchMsg](data)
	if err != nil {
		return err
	}

	msg := env.Payload
	if msg.Engaged {
		o.logger.Warn("kill switch ENGAGED", "reason", msg.Reason, "actor", msg.ActorID)
		o.killSwitch.Engage()

		// Cancel all in-flight runs
		o.mu.Lock()
		for runID, cancel := range o.cancelFns {
			o.logger.Info("cancelling run due to kill switch", "run_id", runID)
			cancel()
		}
		o.mu.Unlock()
	} else {
		o.logger.Info("kill switch DISENGAGED", "actor", msg.ActorID)
		o.killSwitch.Disengage()
	}

	return nil
}
