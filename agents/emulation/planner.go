package emulation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// PlannerAgent selects validations based on exposure graph, asset type, and policy.
type PlannerAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
}

func NewPlannerAgent() *PlannerAgent {
	return &PlannerAgent{}
}

func (a *PlannerAgent) Name() agentsdk.AgentType { return agentsdk.AgentPlanner }
func (a *PlannerAgent) Squad() agentsdk.Squad    { return agentsdk.SquadEmulation }

func (a *PlannerAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}
	a.logger.Info("planner agent initialized")
	return nil
}

func (a *PlannerAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("planner generating validation campaign",
		"task_id", task.ID,
		"engagement_id", task.EngagementID,
	)

	// In full implementation:
	// 1. Query assets in scope from engagement allowlist
	// 2. Consult Ollama for exposure graph analysis
	// 3. Select playbooks matching asset types and allowed tiers
	// 4. Order steps by priority and dependency
	// 5. Return ordered list of AgentTasks for execution

	// Stub: generate example validation steps
	steps := []agentsdk.Task{
		{
			ID:           uuid.New().String(),
			RunID:        task.RunID,
			EngagementID: task.EngagementID,
			OrgID:        task.OrgID,
			StepNumber:   1,
			Action:       "verify_siem_telemetry_health",
			Tier:         0,
			CreatedAt:    time.Now().UTC(),
		},
		{
			ID:           uuid.New().String(),
			RunID:        task.RunID,
			EngagementID: task.EngagementID,
			OrgID:        task.OrgID,
			StepNumber:   2,
			Action:       "verify_edr_agent_reporting",
			Tier:         0,
			CreatedAt:    time.Now().UTC(),
		},
		{
			ID:           uuid.New().String(),
			RunID:        task.RunID,
			EngagementID: task.EngagementID,
			OrgID:        task.OrgID,
			StepNumber:   3,
			Action:       "execute_benign_marker_test",
			Tier:         1,
			CreatedAt:    time.Now().UTC(),
		},
	}

	outputs, _ := json.Marshal(map[string]any{
		"planned_steps": len(steps),
		"tiers_used":    []int{0, 1},
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		NextSteps:   steps,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *PlannerAgent) Shutdown(_ context.Context) error {
	a.logger.Info("planner agent shutting down")
	return nil
}
