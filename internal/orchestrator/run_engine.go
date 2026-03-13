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
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// KillSwitch tracks the global kill switch state.
type KillSwitch struct {
	mu      sync.RWMutex
	engaged bool
}

func NewKillSwitch() *KillSwitch {
	return &KillSwitch{}
}

func (ks *KillSwitch) IsEngaged() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.engaged
}

func (ks *KillSwitch) Engage() {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.engaged = true
}

func (ks *KillSwitch) Disengage() {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.engaged = false
}

// RunEngine executes runs end-to-end.
type RunEngine struct {
	agents     *AgentRegistry
	runs       *repository.RunRepo
	steps      *repository.RunStepRepo
	findings   *repository.FindingRepo
	engagements *repository.EngagementRepo
	killSwitch *KillSwitch
	logger     *slog.Logger
}

// NewRunEngine creates a new run engine.
func NewRunEngine(
	agents *AgentRegistry,
	runs *repository.RunRepo,
	steps *repository.RunStepRepo,
	findings *repository.FindingRepo,
	engagements *repository.EngagementRepo,
	killSwitch *KillSwitch,
	logger *slog.Logger,
) *RunEngine {
	return &RunEngine{
		agents:      agents,
		runs:        runs,
		steps:       steps,
		findings:    findings,
		engagements: engagements,
		killSwitch:  killSwitch,
		logger:      logger,
	}
}

// ExecuteRun runs an engagement end-to-end.
func (e *RunEngine) ExecuteRun(ctx context.Context, run *models.Run) error {
	e.logger.Info("starting run execution", "run_id", run.ID, "engagement_id", run.EngagementID)

	// Update status to running
	if err := e.runs.UpdateStatus(ctx, run.ID, models.RunRunning); err != nil {
		return fmt.Errorf("updating run status: %w", err)
	}

	eng, err := e.engagements.GetByID(ctx, run.EngagementID)
	if err != nil {
		e.failRun(ctx, run.ID, "engagement not found")
		return fmt.Errorf("getting engagement: %w", err)
	}

	// Step 1: Plan — use planner agent to generate steps
	planTask := &agentsdk.Task{
		ID:           uuid.New().String(),
		RunID:        run.ID,
		EngagementID: eng.ID,
		OrgID:        eng.OrgID,
		StepNumber:   0,
		Action:       "plan",
		Tier:         0,
		PolicyContext: &agentsdk.PolicyContext{
			AllowedTiers:     eng.AllowedTiers,
			TargetAllowlist:  eng.TargetAllowlist,
			TargetExclusions: eng.TargetExclusions,
			RateLimit:        eng.RateLimit,
			ConcurrencyCap:   eng.ConcurrencyCap,
		},
		CreatedAt: time.Now().UTC(),
	}

	planner, err := e.agents.Get(agentsdk.AgentPlanner)
	if err != nil {
		e.failRun(ctx, run.ID, "planner agent not found")
		return err
	}

	planResult, err := planner.HandleTask(ctx, planTask)
	if err != nil {
		e.failRun(ctx, run.ID, "planning failed: "+err.Error())
		return err
	}

	if planResult.Status != agentsdk.StatusCompleted {
		e.failRun(ctx, run.ID, "planning returned non-completed status")
		return fmt.Errorf("planning failed: %s", planResult.Error)
	}

	// Get planned steps
	plannedSteps := planResult.NextSteps
	if len(plannedSteps) == 0 {
		e.logger.Info("no steps planned, completing run", "run_id", run.ID)
		return e.runs.UpdateStatus(ctx, run.ID, models.RunCompleted)
	}

	// Update total steps
	if err := e.runs.SetStepsTotal(ctx, run.ID, len(plannedSteps)); err != nil {
		e.logger.Error("setting steps total", "error", err)
	}

	// Step 2: Execute each step
	for _, stepTask := range plannedSteps {
		// Check kill switch before each step
		if e.killSwitch.IsEngaged() {
			e.logger.Warn("kill switch engaged, stopping run", "run_id", run.ID)
			return e.runs.UpdateStatus(ctx, run.ID, models.RunKilled)
		}

		if ctx.Err() != nil {
			return e.runs.UpdateStatus(ctx, run.ID, models.RunCancelled)
		}

		// Create step record
		stepRecord := &models.RunStep{
			RunID:      run.ID,
			StepNumber: stepTask.StepNumber,
			AgentType:  string(stepTask.Action),
			Action:     stepTask.Action,
			Tier:       stepTask.Tier,
			Status:     models.StepPending,
			Inputs:     stepTask.Inputs,
		}
		if stepRecord.Inputs == nil {
			stepRecord.Inputs = json.RawMessage(`{}`)
		}
		if err := e.steps.Create(ctx, stepRecord); err != nil {
			e.logger.Error("creating step record", "error", err)
			continue
		}

		// Mark step running
		_ = e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepRunning, nil)

		// Policy check
		enforcer, _ := e.agents.Get(agentsdk.AgentPolicyEnforcer)
		if enforcer != nil {
			policyResult, _ := enforcer.HandleTask(ctx, &agentsdk.Task{
				ID:            uuid.New().String(),
				RunID:         run.ID,
				EngagementID:  eng.ID,
				OrgID:         eng.OrgID,
				StepNumber:    stepTask.StepNumber,
				Action:        stepTask.Action,
				Tier:          stepTask.Tier,
				PolicyContext: planTask.PolicyContext,
				CreatedAt:     time.Now().UTC(),
			})
			if policyResult != nil && policyResult.Status == agentsdk.StatusBlocked {
				errMsg := "blocked by policy: " + policyResult.Error
				_ = e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepBlocked, &errMsg)
				_ = e.runs.IncrementSteps(ctx, run.ID, 0, 1)
				continue
			}
		}

		// Execute step
		executor, err := e.agents.Get(agentsdk.AgentExecutor)
		if err != nil {
			errMsg := "executor agent not found"
			_ = e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg)
			_ = e.runs.IncrementSteps(ctx, run.ID, 0, 1)
			continue
		}

		result, err := executor.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			StepNumber:   stepTask.StepNumber,
			Action:       stepTask.Action,
			Tier:         stepTask.Tier,
			Inputs:       stepTask.Inputs,
			CreatedAt:    time.Now().UTC(),
		})
		if err != nil {
			errMsg := err.Error()
			_ = e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg)
			_ = e.runs.IncrementSteps(ctx, run.ID, 0, 1)
			continue
		}

		// Update step with results
		_ = e.steps.SetOutputs(ctx, stepRecord.ID, result.Outputs, result.EvidenceIDs, result.CleanupDone)

		if result.Status == agentsdk.StatusCompleted {
			_ = e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepCompleted, nil)
			_ = e.runs.IncrementSteps(ctx, run.ID, 1, 0)
		} else {
			errMsg := result.Error
			_ = e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg)
			_ = e.runs.IncrementSteps(ctx, run.ID, 0, 1)
		}

		// Create findings from agent result
		for _, f := range result.Findings {
			finding := &models.Finding{
				OrgID:          eng.OrgID,
				RunID:          &run.ID,
				RunStepID:      &stepRecord.ID,
				Title:          f.Title,
				Description:    &f.Description,
				Severity:       models.Severity(f.Severity),
				Confidence:     models.Confidence(f.Confidence),
				Status:         models.FindingObserved,
				TechniqueIDs:   f.TechniqueIDs,
				EvidenceIDs:    f.EvidenceIDs,
				Remediation:    &f.Remediation,
				Metadata:       json.RawMessage(`{}`),
			}
			if finding.TechniqueIDs == nil {
				finding.TechniqueIDs = []string{}
			}
			if finding.EvidenceIDs == nil {
				finding.EvidenceIDs = []string{}
			}
			if finding.AffectedAssets == nil {
				finding.AffectedAssets = []uuid.UUID{}
			}
			if err := e.findings.Create(ctx, finding); err != nil {
				e.logger.Error("creating finding", "error", err)
			}
		}
	}

	// Step 3: Generate receipt
	receiptAgent, _ := e.agents.Get(agentsdk.AgentReceipt)
	if receiptAgent != nil {
		_, _ = receiptAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			Action:       "generate_receipt",
			CreatedAt:    time.Now().UTC(),
		})
	}

	// Complete run
	return e.runs.UpdateStatus(ctx, run.ID, models.RunCompleted)
}

func (e *RunEngine) failRun(ctx context.Context, runID uuid.UUID, reason string) {
	e.logger.Error("run failed", "run_id", runID, "reason", reason)
	_ = e.runs.UpdateStatus(ctx, runID, models.RunFailed)
}
