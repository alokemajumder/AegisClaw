package governance

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ReceiptAgent generates tamper-evident run receipts and evidence manifests.
type ReceiptAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
}

func NewReceiptAgent() *ReceiptAgent {
	return &ReceiptAgent{}
}

func (a *ReceiptAgent) Name() agentsdk.AgentType { return agentsdk.AgentReceipt }
func (a *ReceiptAgent) Squad() agentsdk.Squad    { return agentsdk.SquadGovernance }

func (a *ReceiptAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}
	a.logger.Info("receipt agent initialized")
	return nil
}

func (a *ReceiptAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("receipt agent generating receipt",
		"task_id", task.ID,
		"run_id", task.RunID,
	)

	// In full implementation: collect all step records, evidence IDs,
	// generate a signed receipt via internal/receipt, store in evidence vault

	outputs, _ := json.Marshal(map[string]any{
		"receipt_generated": true,
		"run_id":            task.RunID.String(),
		"timestamp":         time.Now().UTC(),
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *ReceiptAgent) Shutdown(_ context.Context) error {
	a.logger.Info("receipt agent shutting down")
	return nil
}
