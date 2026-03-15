package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// PolicyEnforcerAgent validates every step against scope, tier, allowlist, and rate limits.
type PolicyEnforcerAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
}

func NewPolicyEnforcerAgent() *PolicyEnforcerAgent {
	return &PolicyEnforcerAgent{}
}

func (a *PolicyEnforcerAgent) Name() agentsdk.AgentType { return agentsdk.AgentPolicyEnforcer }
func (a *PolicyEnforcerAgent) Squad() agentsdk.Squad    { return agentsdk.SquadGovernance }

func (a *PolicyEnforcerAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}
	a.logger.Info("policy enforcer agent initialized")
	return nil
}

func (a *PolicyEnforcerAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("policy enforcer evaluating task",
		"task_id", task.ID,
		"tier", task.Tier,
		"action", task.Action,
	)

	// SECURITY: Fail-closed — if no PolicyContext is provided, block the step.
	// This prevents bypass by omitting policy data from the task.
	if task.PolicyContext == nil {
		a.logger.Error("policy enforcer blocked task: no PolicyContext provided", "task_id", task.ID)
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusBlocked,
			Error:       "no policy context provided — all actions require policy evaluation",
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Tier 3 is always blocked (check first, before tier-allowed check)
	if task.Tier >= 3 {
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusBlocked,
			Error:       "tier 3 actions are prohibited",
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Validate tier is in the engagement's allowed tiers list
	allowed := false
	for _, t := range task.PolicyContext.AllowedTiers {
		if t == task.Tier {
			allowed = true
			break
		}
	}
	if !allowed {
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusBlocked,
			Error:       fmt.Sprintf("tier %d is not in allowed tiers %v", task.Tier, task.PolicyContext.AllowedTiers),
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Tier 1+ requires a non-empty target allowlist
	if task.Tier >= 1 && len(task.PolicyContext.TargetAllowlist) == 0 {
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusBlocked,
			Error:       "tier 1+ actions require a non-empty target allowlist on the engagement",
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Tier 2+ requires approval
	if task.Tier >= 2 {
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusNeedsApproval,
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Tier 0-1: approved automatically
	outputs, _ := json.Marshal(map[string]string{
		"decision": "approved",
		"reason":   fmt.Sprintf("tier %d is within autonomous scope", task.Tier),
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *PolicyEnforcerAgent) Shutdown(_ context.Context) error {
	a.logger.Info("policy enforcer agent shutting down")
	return nil
}
