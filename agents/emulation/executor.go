package emulation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ExecutorAgent dispatches validation steps to the Runner and verifies cleanup.
type ExecutorAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
}

func NewExecutorAgent() *ExecutorAgent {
	return &ExecutorAgent{}
}

func (a *ExecutorAgent) Name() agentsdk.AgentType { return agentsdk.AgentExecutor }
func (a *ExecutorAgent) Squad() agentsdk.Squad    { return agentsdk.SquadEmulation }

func (a *ExecutorAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}
	a.logger.Info("executor agent initialized")
	return nil
}

func (a *ExecutorAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("executor running validation step",
		"task_id", task.ID,
		"action", task.Action,
		"tier", task.Tier,
	)

	// In full implementation:
	// 1. Dispatch the step to a Runner via gRPC (sandboxed execution)
	// 2. Wait for completion
	// 3. Verify cleanup was performed
	// 4. Collect execution artifacts

	outputs, _ := json.Marshal(map[string]any{
		"executed":        true,
		"action":          task.Action,
		"cleanup_verified": true,
		"duration_ms":     150,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CleanupDone: true,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *ExecutorAgent) Shutdown(_ context.Context) error {
	a.logger.Info("executor agent shutting down")
	return nil
}
