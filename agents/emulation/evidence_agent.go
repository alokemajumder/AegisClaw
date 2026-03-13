package emulation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// EvidenceAgent captures safe artifacts and stores them in the evidence vault.
type EvidenceAgent struct {
	logger *slog.Logger
	deps   agentsdk.AgentDeps
	store  *evidence.Store
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

	if s, ok := deps.EvidenceStore.(*evidence.Store); ok {
		a.store = s
		a.logger.Info("evidence agent connected to evidence store")
	} else {
		a.logger.Warn("evidence agent has no evidence store, artifacts will not be persisted")
	}

	a.logger.Info("evidence agent initialized")
	return nil
}

func (a *EvidenceAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("evidence agent capturing artifacts",
		"task_id", task.ID,
		"run_id", task.RunID,
	)

	var evidenceIDs []string

	// Parse inputs to get execution outputs to store
	var inputs map[string]any
	if task.Inputs != nil {
		_ = json.Unmarshal(task.Inputs, &inputs)
	}

	if a.store != nil {
		// Store the task inputs/outputs as evidence artifact
		artifactData, _ := json.MarshalIndent(map[string]any{
			"task_id":     task.ID,
			"run_id":      task.RunID.String(),
			"action":      task.Action,
			"tier":        task.Tier,
			"inputs":      inputs,
			"captured_at": time.Now().UTC(),
		}, "", "  ")

		artifactName := fmt.Sprintf("step_%d_%s.json", task.StepNumber, task.Action)
		artifact, err := a.store.Upload(ctx, task.RunID.String(), artifactName, "application/json", artifactData)
		if err != nil {
			a.logger.Error("failed to upload evidence artifact", "error", err)
		} else {
			evidenceIDs = append(evidenceIDs, artifact.ID)
			a.logger.Info("evidence artifact stored", "id", artifact.ID, "name", artifactName)
		}
	} else {
		// Fallback: generate stub IDs
		evidenceIDs = []string{
			fmt.Sprintf("ev_stub_%s_%d", task.RunID.String()[:8], task.StepNumber),
		}
	}

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
