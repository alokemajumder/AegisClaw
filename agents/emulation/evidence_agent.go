package emulation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// EvidenceAgent captures safe artifacts and expected telemetry signatures.
type EvidenceAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
}

func NewEvidenceAgent() *EvidenceAgent {
	return &EvidenceAgent{}
}

func (a *EvidenceAgent) Name() agentsdk.AgentType { return agentsdk.AgentEvidence }
func (a *EvidenceAgent) Squad() agentsdk.Squad    { return agentsdk.SquadEmulation }

func (a *EvidenceAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}
	a.logger.Info("evidence agent initialized")
	return nil
}

func (a *EvidenceAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("evidence agent capturing artifacts",
		"task_id", task.ID,
		"run_id", task.RunID,
	)

	// In full implementation:
	// 1. Collect execution outputs from the Executor
	// 2. Sanitize and redact sensitive data per policy
	// 3. Store artifacts in evidence vault (MinIO)
	// 4. Record expected telemetry signatures for validation

	evidenceIDs := []string{"ev_stub_001", "ev_stub_002"}

	outputs, _ := json.Marshal(map[string]any{
		"artifacts_captured": len(evidenceIDs),
		"evidence_ids":       evidenceIDs,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		EvidenceIDs: evidenceIDs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *EvidenceAgent) Shutdown(_ context.Context) error {
	a.logger.Info("evidence agent shutting down")
	return nil
}
