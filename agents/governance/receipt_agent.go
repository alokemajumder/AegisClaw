package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ReceiptAgent generates tamper-evident run receipts and stores them in the evidence vault.
type ReceiptAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
	store  *evidence.Store
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

	if s, ok := deps.EvidenceStore.(*evidence.Store); ok {
		a.store = s
		a.logger.Info("receipt agent connected to evidence store")
	} else {
		a.logger.Warn("receipt agent has no evidence store, receipts will not be persisted")
	}

	a.logger.Info("receipt agent initialized")
	return nil
}

func (a *ReceiptAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("receipt agent generating receipt",
		"task_id", task.ID,
		"run_id", task.RunID,
	)

	receipt := map[string]any{
		"run_id":       task.RunID.String(),
		"org_id":       task.OrgID.String(),
		"generated_at": time.Now().UTC(),
		"version":      "1.0",
	}

	// Parse step results from inputs if available
	var inputs map[string]any
	if task.Inputs != nil {
		_ = json.Unmarshal(task.Inputs, &inputs)
	}
	if steps, ok := inputs["step_results"]; ok {
		receipt["steps"] = steps
	}
	if evidenceIDs, ok := inputs["evidence_ids"]; ok {
		receipt["evidence_ids"] = evidenceIDs
	}

	receiptData, _ := json.MarshalIndent(receipt, "", "  ")

	// Compute content hash for tamper detection
	hash := sha256.Sum256(receiptData)
	receipt["content_hash"] = hex.EncodeToString(hash[:])

	// Re-marshal with hash included
	receiptData, _ = json.MarshalIndent(receipt, "", "  ")

	// Store receipt in evidence vault
	var evidenceIDs []string
	if a.store != nil {
		if err := a.store.UploadReceipt(ctx, task.RunID.String(), receiptData); err != nil {
			a.logger.Error("failed to upload receipt", "error", err)
		} else {
			a.logger.Info("receipt stored in evidence vault", "run_id", task.RunID)
			evidenceIDs = append(evidenceIDs, "receipt_"+task.RunID.String()[:8])
		}
	}

	outputs, _ := json.Marshal(map[string]any{
		"receipt_generated": true,
		"run_id":            task.RunID.String(),
		"content_hash":      receipt["content_hash"],
		"timestamp":         time.Now().UTC(),
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		EvidenceIDs: evidenceIDs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *ReceiptAgent) Shutdown(_ context.Context) error {
	a.logger.Info("receipt agent shutting down")
	return nil
}
