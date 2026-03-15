package governance

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/internal/receipt"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ReceiptAgent generates tamper-evident run receipts and stores them in the evidence vault.
type ReceiptAgent struct {
	logger    *slog.Logger
	deps      agentsdk.AgentDeps
	store     *evidence.Store
	generator *receipt.Generator
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

	if key, ok := deps.ReceiptHMACKey.([]byte); ok && len(key) > 0 {
		a.generator = receipt.NewGenerator(key)
		a.logger.Info("receipt agent using HMAC-SHA256 signing")
	} else {
		a.logger.Warn("receipt agent has no HMAC key, receipts will not be signed")
	}

	a.logger.Info("receipt agent initialized")
	return nil
}

func (a *ReceiptAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("receipt agent generating receipt",
		"task_id", task.ID,
		"run_id", task.RunID,
	)

	// Parse inputs for step results and scope snapshot
	var inputs map[string]any
	if task.Inputs != nil {
		_ = json.Unmarshal(task.Inputs, &inputs)
	}

	// Build proper receipt struct
	runReceipt := receipt.RunReceipt{
		RunID:        task.RunID,
		EngagementID: task.EngagementID,
		OrgID:        task.OrgID,
		StartedAt:    task.CreatedAt,
		CompletedAt:  time.Now().UTC(),
		Outcome:      "completed",
		ToolVersions: map[string]string{"aegisclaw": "1.0"},
	}

	// Extract scope snapshot
	if scopeRaw, ok := inputs["scope_snapshot"]; ok {
		scopeBytes, _ := json.Marshal(scopeRaw)
		_ = json.Unmarshal(scopeBytes, &runReceipt.ScopeSnapshot)
	}

	// Extract step results
	if stepsRaw, ok := inputs["step_results"]; ok {
		stepsBytes, _ := json.Marshal(stepsRaw)
		var steps []receipt.StepRecord
		_ = json.Unmarshal(stepsBytes, &steps)
		runReceipt.Steps = steps
	}

	// Extract evidence manifest
	if evidenceRaw, ok := inputs["evidence_ids"]; ok {
		evidenceBytes, _ := json.Marshal(evidenceRaw)
		var ids []string
		_ = json.Unmarshal(evidenceBytes, &ids)
		runReceipt.EvidenceManifest = ids
	}

	if outcomeStr, ok := inputs["outcome"].(string); ok {
		runReceipt.Outcome = outcomeStr
	}

	// Sign with HMAC if generator available
	var findings []agentsdk.FindingOutput
	if a.generator != nil {
		if err := a.generator.Generate(&runReceipt); err != nil {
			a.logger.Error("failed to sign receipt", "error", err)
			findings = append(findings, agentsdk.FindingOutput{
				Title:       "Receipt signing failed",
				Description: "HMAC-SHA256 receipt signing failed — receipt is not tamper-evident",
				Severity:    "high",
				Confidence:  "confirmed",
				Remediation: "Check the HMAC key configuration and retry the run",
			})
		}
	} else {
		runReceipt.ReceiptID = "rcpt_" + uuid.New().String()[:12]
		runReceipt.GeneratedAt = time.Now().UTC()
		a.logger.Warn("receipt generated WITHOUT signature — not tamper-evident", "run_id", task.RunID)
		findings = append(findings, agentsdk.FindingOutput{
			Title:       "Unsigned receipt generated",
			Description: "No HMAC key configured — receipt is not signed and cannot be verified for tampering",
			Severity:    "medium",
			Confidence:  "confirmed",
			Remediation: "Configure AEGISCLAW_AUTH_RECEIPT_HMAC_KEY to enable receipt signing",
		})
	}

	receiptData, _ := json.MarshalIndent(runReceipt, "", "  ")

	// Store receipt in evidence vault
	var evidenceIDs []string
	if a.store != nil {
		if err := a.store.UploadReceipt(ctx, task.RunID.String(), receiptData); err != nil {
			a.logger.Error("failed to upload receipt", "error", err)
		} else {
			a.logger.Info("receipt stored in evidence vault", "run_id", task.RunID, "receipt_id", runReceipt.ReceiptID)
			evidenceIDs = append(evidenceIDs, "receipt_"+task.RunID.String()[:8])
		}
	}

	outputs, _ := json.Marshal(map[string]any{
		"receipt_id":  runReceipt.ReceiptID,
		"run_id":      task.RunID.String(),
		"signature":   runReceipt.Signature,
		"signed":      a.generator != nil,
		"steps_count": len(runReceipt.Steps),
		"timestamp":   runReceipt.GeneratedAt,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		EvidenceIDs: evidenceIDs,
		Findings:    findings,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *ReceiptAgent) Shutdown(_ context.Context) error {
	a.logger.Info("receipt agent shutting down")
	return nil
}
