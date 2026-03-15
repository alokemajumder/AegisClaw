package playbook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// connectorQuerier is the subset of connector.Service used by the executor.
// Using an interface avoids a circular import with internal/connector.
type connectorQuerier interface {
	QueryEvents(ctx context.Context, instanceID uuid.UUID, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error)
}

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
	logger       *slog.Logger
	connectorSvc any // *connector.Service — avoids circular import
}

// NewExecutor creates a new playbook step executor.
// connectorSvc should be a *connector.Service (or nil if unavailable).
func NewExecutor(connectorSvc any, logger *slog.Logger) *Executor {
	return &Executor{
		connectorSvc: connectorSvc,
		logger:       logger,
	}
}

// safeCommands is the strict allowlist for execute_encoded_command.
var safeCommands = map[string]bool{
	"whoami":     true,
	"hostname":   true,
	"ipconfig":   true,
	"systeminfo": true,
	"dir":        true,
	"ls":         true,
	"uname":      true,
	"id":         true,
	"pwd":        true,
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
		e.executeQueryTelemetry(ctx, step, result)

	case "check_edr_agents":
		e.executeCheckEDRAgents(ctx, step, result)

	case "drop_marker_file":
		e.executeDropMarkerFile(ctx, step, result)

	case "execute_encoded_command":
		e.executeEncodedCommand(ctx, step, result)

	case "verify_detection":
		e.executeVerifyDetection(ctx, step, result)

	case "verify_cleanup":
		e.executeVerifyCleanup(ctx, step, result)

	default:
		result.Status = "failed"
		result.Error = fmt.Sprintf("unknown action: %s", step.Action)
		outputs := map[string]any{
			"action": step.Action,
			"error":  result.Error,
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

// getConnectorSvc returns the connector service as a connectorQuerier, or nil.
func (e *Executor) getConnectorSvc() connectorQuerier {
	if e.connectorSvc == nil {
		return nil
	}
	if svc, ok := e.connectorSvc.(connectorQuerier); ok {
		return svc
	}
	return nil
}

// executeQueryTelemetry queries SIEM for recent telemetry via a connector.
func (e *Executor) executeQueryTelemetry(ctx context.Context, step *PlaybookStep, result *StepResult) {
	connectorID, _ := step.Inputs["connector_id"].(string)
	query, _ := step.Inputs["query"].(string)

	svc := e.getConnectorSvc()
	if connectorID == "" || svc == nil {
		result.Status = "completed"
		outputs := map[string]any{
			"action":          "query_telemetry",
			"telemetry_found": false,
			"no_connector":    true,
			"event_count":     0,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	connUUID, err := uuid.Parse(connectorID)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid connector_id: %v", err)
		return
	}

	now := time.Now().UTC()
	eventQuery := connectorsdk.EventQuery{
		TimeRange: connectorsdk.TimeRange{
			Start: now.Add(-15 * time.Minute),
			End:   now,
		},
		Query:      query,
		MaxResults: 100,
	}

	eventResult, err := svc.QueryEvents(ctx, connUUID, eventQuery)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("query_telemetry: %v", err)
		outputs := map[string]any{
			"action":          "query_telemetry",
			"telemetry_found": false,
			"error":           err.Error(),
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	result.Status = "completed"
	outputs := map[string]any{
		"action":          "query_telemetry",
		"telemetry_found": eventResult.TotalCount > 0,
		"event_count":     eventResult.TotalCount,
		"truncated":       eventResult.Truncated,
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data
}

// executeCheckEDRAgents checks EDR agent health via a connector.
func (e *Executor) executeCheckEDRAgents(ctx context.Context, step *PlaybookStep, result *StepResult) {
	connectorID, _ := step.Inputs["connector_id"].(string)

	svc := e.getConnectorSvc()
	if connectorID == "" || svc == nil {
		result.Status = "completed"
		outputs := map[string]any{
			"action":       "check_edr_agents",
			"agents_found": 0,
			"no_connector": true,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	connUUID, err := uuid.Parse(connectorID)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid connector_id: %v", err)
		return
	}

	now := time.Now().UTC()
	eventQuery := connectorsdk.EventQuery{
		TimeRange: connectorsdk.TimeRange{
			Start: now.Add(-30 * time.Minute),
			End:   now,
		},
		Query:      "agent health status",
		MaxResults: 500,
	}

	eventResult, err := svc.QueryEvents(ctx, connUUID, eventQuery)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("check_edr_agents: %v", err)
		outputs := map[string]any{
			"action":       "check_edr_agents",
			"agents_found": 0,
			"error":        err.Error(),
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	result.Status = "completed"
	healthStatus := "healthy"
	if eventResult.TotalCount == 0 {
		healthStatus = "unknown"
	}
	outputs := map[string]any{
		"action":        "check_edr_agents",
		"agents_found":  eventResult.TotalCount,
		"health_status": healthStatus,
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data
}

// executeDropMarkerFile creates a safe marker file for EDR detection testing.
func (e *Executor) executeDropMarkerFile(_ context.Context, _ *PlaybookStep, result *StepResult) {
	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	markerPath := filepath.Join(tmpDir, "eicar-test.txt")
	eicar := `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`

	if err := os.WriteFile(markerPath, []byte(eicar), 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing marker file: %v", err)
		// Best-effort cleanup of temp dir
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false // cleanup happens in verify_cleanup
	outputs := map[string]any{
		"action":      "drop_marker_file",
		"marker_path": markerPath,
		"marker_dir":  tmpDir,
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("marker file created", "path", markerPath)
}

// executeEncodedCommand runs a benign command after strict allowlist validation.
func (e *Executor) executeEncodedCommand(ctx context.Context, step *PlaybookStep, result *StepResult) {
	encodedCmd, _ := step.Inputs["command"].(string)
	if encodedCmd == "" {
		result.Status = "failed"
		result.Error = "missing required input: command"
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(encodedCmd)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("decoding base64 command: %v", err)
		return
	}

	cmdStr := strings.TrimSpace(string(decoded))
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		result.Status = "failed"
		result.Error = "decoded command is empty"
		return
	}

	// SECURITY: Extract only the base name from the command path to prevent
	// PATH traversal bypass (e.g., /usr/bin/rm would resolve to "rm").
	baseName := filepath.Base(parts[0])
	if !safeCommands[baseName] {
		result.Status = "failed"
		result.Error = fmt.Sprintf("command %q not in allowlist [whoami, hostname, ipconfig, systeminfo, dir, ls, uname, id, pwd]", baseName)
		outputs := map[string]any{
			"action":  "execute_encoded_command",
			"blocked": true,
			"command": baseName,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	// 30-second timeout for command execution
	cmdCtx, cmdCancel := context.WithTimeout(ctx, 30*time.Second)
	defer cmdCancel()

	// SECURITY: Use the validated baseName, not the user-supplied path,
	// to prevent PATH traversal (e.g., /tmp/evil/ls).
	// Also reject any arguments beyond the command itself to prevent flag injection.
	if len(parts) > 1 {
		result.Status = "failed"
		result.Error = fmt.Sprintf("command arguments not allowed for safety: %q", cmdStr)
		outputs := map[string]any{
			"action":  "execute_encoded_command",
			"blocked": true,
			"command": baseName,
			"reason":  "arguments not permitted in allowlisted commands",
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	startTime := time.Now()
	cmd := exec.CommandContext(cmdCtx, baseName)
	stdout, err := cmd.CombinedOutput()
	elapsed := time.Since(startTime)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			result.Status = "failed"
			result.Error = fmt.Sprintf("executing command: %v", err)
			outputs := map[string]any{
				"action":          "execute_encoded_command",
				"command":         baseName,
				"error":           err.Error(),
				"execution_time":  elapsed.String(),
			}
			data, _ := json.Marshal(outputs)
			result.Outputs = data
			return
		}
	}

	result.Status = "completed"
	result.CleanupDone = true
	outputs := map[string]any{
		"action":         "execute_encoded_command",
		"command":        baseName,
		"output":         string(stdout),
		"exit_code":      exitCode,
		"execution_time": elapsed.String(),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("encoded command executed",
		"command", baseName,
		"exit_code", exitCode,
		"duration", elapsed,
	)
}

// executeVerifyDetection checks if SIEM/EDR detected the emulation activity.
func (e *Executor) executeVerifyDetection(ctx context.Context, step *PlaybookStep, result *StepResult) {
	connectorID, _ := step.Inputs["connector_id"].(string)
	techniqueID, _ := step.Inputs["technique_id"].(string)

	svc := e.getConnectorSvc()
	if connectorID == "" || svc == nil {
		result.Status = "completed"
		outputs := map[string]any{
			"action":       "verify_detection",
			"detected":     false,
			"no_connector": true,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	connUUID, err := uuid.Parse(connectorID)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid connector_id: %v", err)
		return
	}

	now := time.Now().UTC()
	query := techniqueID
	if query == "" {
		query = "alert"
	}

	eventQuery := connectorsdk.EventQuery{
		TimeRange: connectorsdk.TimeRange{
			Start: now.Add(-5 * time.Minute),
			End:   now,
		},
		Query:      query,
		MaxResults: 50,
	}

	startTime := time.Now()
	eventResult, err := svc.QueryEvents(ctx, connUUID, eventQuery)
	latency := time.Since(startTime)

	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("verify_detection: %v", err)
		outputs := map[string]any{
			"action":   "verify_detection",
			"detected": false,
			"error":    err.Error(),
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	detected := eventResult.TotalCount > 0
	result.Status = "completed"
	outputs := map[string]any{
		"action":       "verify_detection",
		"detected":     detected,
		"alert_count":  eventResult.TotalCount,
		"latency":      latency.String(),
		"technique_id": techniqueID,
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data
}

// aegisclawTempPrefix is the required prefix for marker file paths to prevent
// arbitrary filesystem deletion via the verify_cleanup action.
const aegisclawTempPrefix = "aegisclaw-marker-"

// executeVerifyCleanup verifies that a marker file was cleaned up (by EDR or self).
func (e *Executor) executeVerifyCleanup(_ context.Context, step *PlaybookStep, result *StepResult) {
	markerPath, _ := step.Inputs["marker_path"].(string)
	if markerPath == "" {
		result.Status = "failed"
		result.Error = "missing required input: marker_path"
		return
	}

	// SECURITY: Validate the marker path is within our temp directory to prevent
	// arbitrary filesystem deletion. The path must be under os.TempDir() and the
	// parent directory must match our naming convention.
	absPath, err := filepath.Abs(markerPath)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid marker_path: %v", err)
		return
	}
	parentDir := filepath.Dir(absPath)
	parentBase := filepath.Base(parentDir)
	tempDir := os.TempDir()
	if !strings.HasPrefix(parentDir, tempDir) || !strings.HasPrefix(parentBase, aegisclawTempPrefix) {
		result.Status = "failed"
		result.Error = fmt.Sprintf("marker_path %q is not in an aegisclaw temp directory — refusing cleanup to prevent arbitrary deletion", markerPath)
		e.logger.Error("verify_cleanup blocked: path outside aegisclaw temp dir", "path", markerPath, "parent", parentDir)
		return
	}

	_, err = os.Stat(absPath)
	if os.IsNotExist(err) {
		// File is gone — EDR or another process removed it
		result.Status = "completed"
		result.CleanupDone = true
		outputs := map[string]any{
			"action":            "verify_cleanup",
			"cleanup_confirmed": true,
			"self_cleaned":      false,
			"marker_path":       markerPath,
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		e.logger.Info("marker file already removed (likely by EDR)", "path", markerPath)
		return
	}

	if err != nil {
		// Non-IsNotExist error (e.g. EACCES from AV quarantine) — treat as
		// successful detection: the file was likely quarantined by AV.
		result.Status = "completed"
		result.CleanupDone = true
		outputs := map[string]any{
			"action":            "verify_cleanup",
			"cleanup_confirmed": true,
			"self_cleaned":      false,
			"av_quarantined":    true,
			"marker_path":       markerPath,
			"stat_error":        err.Error(),
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		e.logger.Info("marker file likely quarantined by AV", "path", markerPath, "error", err)
		return
	}

	// File still exists (err == nil) — self-cleanup (only the validated parent dir)
	removeErr := os.RemoveAll(parentDir)
	selfCleaned := removeErr == nil

	result.Status = "completed"
	result.CleanupDone = selfCleaned
	outputs := map[string]any{
		"action":            "verify_cleanup",
		"cleanup_confirmed": selfCleaned,
		"self_cleaned":      true,
		"marker_path":       markerPath,
	}
	if removeErr != nil {
		outputs["cleanup_error"] = removeErr.Error()
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	if selfCleaned {
		e.logger.Info("marker file self-cleaned", "path", markerPath)
	} else {
		e.logger.Warn("failed to self-clean marker file", "path", markerPath, "error", removeErr)
	}
}
