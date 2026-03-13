package emulation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alokemajumder/AegisClaw/internal/playbook"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ExecutorAgent dispatches validation steps to the playbook executor and verifies cleanup.
type ExecutorAgent struct {
	logger   *slog.Logger
	deps     agentsdk.AgentDeps
	executor *playbook.Executor
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

	// Get playbook executor from deps
	if exec, ok := deps.PlaybookExecutor.(*playbook.Executor); ok {
		a.executor = exec
		a.logger.Info("executor agent using real playbook executor")
	} else {
		a.executor = playbook.NewExecutor(a.logger)
		a.logger.Info("executor agent created local playbook executor")
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

	// Parse playbook info from inputs if available
	var inputs map[string]any
	if task.Inputs != nil {
		_ = json.Unmarshal(task.Inputs, &inputs)
	}

	// Build a playbook step for the executor
	pbStep := &playbook.PlaybookStep{
		Name:    task.Action,
		Action:  task.Action,
		Inputs:  make(map[string]any),
	}

	// Copy inputs from the task into the playbook step
	if stepInputs, ok := inputs["inputs"].(map[string]any); ok {
		pbStep.Inputs = stepInputs
	}
	if stepName, ok := inputs["step_name"].(string); ok {
		pbStep.Name = stepName
	}

	pb := &playbook.Playbook{
		Tier: task.Tier,
	}
	if pbID, ok := inputs["playbook_id"].(string); ok {
		pb.ID = pbID
	}
	if pbName, ok := inputs["playbook_name"].(string); ok {
		pb.Name = pbName
	}

	// Execute via playbook executor
	stepResult, err := a.executor.ExecuteStep(ctx, pb, pbStep)
	if err != nil {
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusFailed,
			Error:       err.Error(),
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Map step result status to agent result status
	status := agentsdk.StatusCompleted
	if stepResult.Status == "failed" {
		status = agentsdk.StatusFailed
	}

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      status,
		Outputs:     stepResult.Outputs,
		EvidenceIDs: stepResult.EvidenceIDs,
		CleanupDone: stepResult.CleanupDone,
		Error:       stepResult.Error,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *ExecutorAgent) Shutdown(_ context.Context) error {
	a.logger.Info("executor agent shutting down")
	return nil
}
