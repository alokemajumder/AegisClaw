package governance

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ApprovalGateAgent blocks Tier 2+ actions until explicit human approval.
type ApprovalGateAgent struct {
	logger       *slog.Logger
	deps         agentsdk.AgentDeps
	approvalRepo *repository.ApprovalRepo
}

func NewApprovalGateAgent() *ApprovalGateAgent {
	return &ApprovalGateAgent{}
}

func (a *ApprovalGateAgent) Name() agentsdk.AgentType { return agentsdk.AgentApprovalGate }
func (a *ApprovalGateAgent) Squad() agentsdk.Squad    { return agentsdk.SquadGovernance }

func (a *ApprovalGateAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	if pool, ok := deps.DB.(*pgxpool.Pool); ok {
		a.approvalRepo = repository.NewApprovalRepo(pool)
		a.logger.Info("approval gate agent initialized with DB")
	} else {
		a.logger.Warn("approval gate agent initialized without DB — approval records will not be persisted")
	}

	return nil
}

func (a *ApprovalGateAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("approval gate processing task",
		"task_id", task.ID,
		"tier", task.Tier,
		"action", task.Action,
	)

	// Only Tier 2+ needs approval gate
	if task.Tier < 2 {
		outputs, _ := json.Marshal(map[string]string{
			"decision": "auto_approved",
			"reason":   "tier 0-1 does not require human approval",
		})
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusCompleted,
			Outputs:     outputs,
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Build the approval request for the SDK response
	approvalReq := agentsdk.ApprovalRequest{
		ID:          uuid.New().String(),
		TaskID:      task.ID,
		RunID:       task.RunID,
		OrgID:       task.OrgID,
		AgentType:   agentsdk.AgentApprovalGate,
		Description: "Tier 2+ action requires human approval: " + task.Action,
		Tier:        task.Tier,
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		CreatedAt:   time.Now().UTC(),
	}

	// Persist the approval record in the database
	if a.approvalRepo != nil {
		tier := task.Tier
		expiresAt := approvalReq.ExpiresAt
		runID := task.RunID
		approval := &models.Approval{
			OrgID:            task.OrgID,
			RequestType:      "tier_gate",
			RequestedBy:      string(agentsdk.AgentApprovalGate),
			TargetEntityID:   &runID,
			TargetEntityType: strPtr("run"),
			Description:      approvalReq.Description,
			Tier:             &tier,
			Status:           models.ApprovalPending,
			ExpiresAt:        &expiresAt,
		}
		if err := a.approvalRepo.Create(ctx, approval); err != nil {
			a.logger.Error("failed to persist approval record",
				"error", err,
				"task_id", task.ID,
			)
			// Continue — the approval is still logically needed even if persistence fails
		} else {
			a.logger.Info("approval record persisted",
				"approval_id", approval.ID,
				"task_id", task.ID,
				"tier", task.Tier,
			)
			// Update the SDK approval request ID to match the DB record
			approvalReq.ID = approval.ID.String()
		}
	} else {
		a.logger.Warn("approval required but no DB available — record not persisted",
			"approval_id", approvalReq.ID,
			"tier", task.Tier,
			"action", task.Action,
		)
	}

	outputs, _ := json.Marshal(map[string]any{
		"approval_request": approvalReq,
		"status":           "awaiting_approval",
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusNeedsApproval,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *ApprovalGateAgent) Shutdown(_ context.Context) error {
	a.logger.Info("approval gate agent shutting down")
	return nil
}

// strPtr is a helper to create a pointer to a string.
func strPtr(s string) *string {
	return &s
}
