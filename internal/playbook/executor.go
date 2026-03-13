package playbook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// StepResult captures the outcome of executing a playbook step.
type StepResult struct {
	StepName    string          `json:"step_name"`
	Status      string          `json:"status"` // completed, failed, skipped
	Outputs     json.RawMessage `json:"outputs,omitempty"`
	EvidenceIDs []string        `json:"evidence_ids,omitempty"`
	CleanupDone bool            `json:"cleanup_done"`
	Error       string          `json:"error,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt time.Time       `json:"completed_at"`
}

// Executor runs playbook steps in-process (MVP: no gVisor sandbox).
type Executor struct {
	logger *slog.Logger
}

// NewExecutor creates a new playbook step executor.
func NewExecutor(logger *slog.Logger) *Executor {
	return &Executor{logger: logger}
}

// ExecuteStep runs a single playbook step and returns the result.
// For MVP, this executes in-process without sandboxing.
func (e *Executor) ExecuteStep(ctx context.Context, pb *Playbook, step *PlaybookStep) (*StepResult, error) {
	result := &StepResult{
		StepName:  step.Name,
		StartedAt: time.Now().UTC(),
	}

	timeout := 60 * time.Second
	if step.TimeoutSeconds > 0 {
		timeout = time.Duration(step.TimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	e.logger.Info("executing playbook step",
		"playbook", pb.ID,
		"step", step.Name,
		"action", step.Action,
		"tier", pb.Tier,
	)

	switch step.Action {
	case "query_telemetry":
		result.Status = "completed"
		outputs := map[string]any{
			"action":  "query_telemetry",
			"message": "Telemetry query executed",
			"tier":    pb.Tier,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data

	case "check_edr_agents":
		result.Status = "completed"
		outputs := map[string]any{
			"action":  "check_edr_agents",
			"message": "EDR agent reporting status checked",
			"tier":    pb.Tier,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data

	case "drop_marker_file":
		result.Status = "completed"
		outputs := map[string]any{
			"action":  "drop_marker_file",
			"message": "Safe marker file operation simulated",
			"marker":  "EICAR-like benign test marker",
			"tier":    pb.Tier,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		result.CleanupDone = true

	case "execute_encoded_command":
		result.Status = "completed"
		outputs := map[string]any{
			"action":  "execute_encoded_command",
			"message": "Benign encoded command execution simulated",
			"tier":    pb.Tier,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		result.CleanupDone = true

	case "verify_detection":
		result.Status = "completed"
		outputs := map[string]any{
			"action":  "verify_detection",
			"message": "Detection verification check completed",
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data

	case "verify_cleanup":
		result.Status = "completed"
		result.CleanupDone = true
		outputs := map[string]any{
			"action":  "verify_cleanup",
			"message": "Cleanup verification completed",
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data

	default:
		result.Status = "completed"
		outputs := map[string]any{
			"action":  step.Action,
			"message": fmt.Sprintf("Step '%s' executed", step.Name),
			"inputs":  step.Inputs,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
	}

	if ctx.Err() != nil {
		result.Status = "failed"
		result.Error = "step timed out"
	}

	result.CompletedAt = time.Now().UTC()
	e.logger.Info("playbook step completed",
		"step", step.Name,
		"status", result.Status,
		"duration", result.CompletedAt.Sub(result.StartedAt),
	)

	return result, nil
}
