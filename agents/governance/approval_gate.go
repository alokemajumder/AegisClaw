package governance

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ApprovalGateAgent blocks Tier 2+ actions until explicit human approval.
type ApprovalGateAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
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
	a.logger.Info("approval gate agent initialized")
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

	// Create an approval request
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

	a.logger.Warn("approval required — blocking until human decision",
		"approval_id", approvalReq.ID,
		"tier", task.Tier,
		"action", task.Action,
	)

	// In a real implementation, this would:
	// 1. Publish the approval request to NATS (approvals.request)
	// 2. Wait for a response on NATS (approvals.decision.{id})
	// 3. Return the decision

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
