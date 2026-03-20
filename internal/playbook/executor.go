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

// SandboxExecutor is the subset of sandbox.Manager used by the playbook executor.
// Defined here as an interface to avoid a circular import with internal/sandbox.
type SandboxExecutor interface {
	IsEnabled() bool
	IsGatewayConnected() bool
	ExecutePlaybookStep(ctx context.Context, tier int, action string, inputs json.RawMessage, timeoutSecs int) (*SandboxStepResult, error)
}

// SandboxStepResult is the result returned from sandbox execution,
// mapped to a format the playbook executor can consume.
type SandboxStepResult struct {
	Status   string          `json:"status"`
	ExitCode int             `json:"exit_code"`
	Outputs  json.RawMessage `json:"outputs,omitempty"`
	Duration time.Duration   `json:"duration"`
	Error    string          `json:"error,omitempty"`
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

// Executor runs playbook steps, routing simulation actions through the sandbox
// when enabled, and running verification/query actions in-process.
type Executor struct {
	logger       *slog.Logger
	connectorSvc any // *connector.Service — avoids circular import
	sandboxMgr   SandboxExecutor
}

// NewExecutor creates a new playbook step executor.
// connectorSvc should be a *connector.Service (or nil if unavailable).
func NewExecutor(connectorSvc any, logger *slog.Logger) *Executor {
	return &Executor{
		connectorSvc: connectorSvc,
		logger:       logger,
	}
}

// SetSandbox attaches a sandbox manager for isolated execution of simulation actions.
func (e *Executor) SetSandbox(mgr SandboxExecutor) {
	e.sandboxMgr = mgr
}

// sandboxActions lists actions that should be routed through the sandbox when
// sandbox execution is enabled. These are simulation actions that produce
// side-effects on the host. Verification and query actions always run in-process
// because they make connector API calls that need direct network access.
var sandboxActions = map[string]bool{
	"simulate_lsass_access":          true,
	"simulate_powershell_execution":  true,
	"simulate_process_injection":     true,
	"create_registry_key":            true,
	"simulate_registry_persistence":  true,
	"execute_wmi_query":              true,
	"simulate_wmi_execution":         true,
	"request_service_tickets":        true,
	"simulate_kerberoasting":         true,
	"simulate_dcsync":                true,
	"simulate_smb_connection":        true,
	"simulate_lateral_movement":      true,
	"drop_marker_file":               true,
	"execute_encoded_command":         true,
}

// isSandboxAction returns true if the action should be routed to the sandbox.
func isSandboxAction(action string) bool {
	return sandboxActions[action]
}

// trySandboxExecution attempts to run an action in the sandbox. Returns the
// StepResult and true if the sandbox handled it, or nil and false if the
// caller should fall back to in-process execution.
func (e *Executor) trySandboxExecution(ctx context.Context, pb *Playbook, step *PlaybookStep) (*StepResult, bool) {
	if e.sandboxMgr == nil || !e.sandboxMgr.IsEnabled() || !e.sandboxMgr.IsGatewayConnected() {
		return nil, false
	}

	if !isSandboxAction(step.Action) {
		return nil, false
	}

	inputsJSON, err := json.Marshal(step.Inputs)
	if err != nil {
		e.logger.Warn("sandbox: failed to marshal step inputs, falling back to in-process",
			"step", step.Name, "error", err)
		return nil, false
	}

	timeoutSecs := step.TimeoutSeconds
	if timeoutSecs == 0 {
		timeoutSecs = 60
	}

	e.logger.Info("routing step to sandbox",
		"playbook", pb.ID,
		"step", step.Name,
		"action", step.Action,
		"tier", pb.Tier,
	)

	sbResult, err := e.sandboxMgr.ExecutePlaybookStep(ctx, pb.Tier, step.Action, inputsJSON, timeoutSecs)
	if err != nil {
		e.logger.Warn("sandbox execution failed, falling back to in-process",
			"step", step.Name,
			"action", step.Action,
			"error", err,
		)
		return nil, false
	}

	result := &StepResult{
		StepName:    step.Name,
		Status:      sbResult.Status,
		Outputs:     sbResult.Outputs,
		CleanupDone: sbResult.Status == "completed",
		Error:       sbResult.Error,
		StartedAt:   time.Now().UTC().Add(-sbResult.Duration),
		CompletedAt: time.Now().UTC(),
	}

	e.logger.Info("sandbox execution completed",
		"step", step.Name,
		"status", sbResult.Status,
		"duration", sbResult.Duration,
	)

	return result, true
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
// When sandbox is enabled, simulation actions are routed through the sandbox
// manager. Verification and query actions always run in-process.
func (e *Executor) ExecuteStep(ctx context.Context, pb *Playbook, step *PlaybookStep) (*StepResult, error) {
	// Try sandbox execution for simulation actions first.
	if sbResult, handled := e.trySandboxExecution(ctx, pb, step); handled {
		return sbResult, nil
	}

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

	e.logger.Info("executing playbook step in-process",
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

	// Tier 1 simulation actions
	case "simulate_lsass_access":
		e.executeSimulateLsassAccess(ctx, step, result)

	case "simulate_powershell_execution":
		e.executeSimulatePowershellExecution(ctx, step, result)

	case "simulate_process_injection":
		e.executeSimulateProcessInjection(ctx, step, result)

	case "create_registry_key", "simulate_registry_persistence":
		e.executeSimulateRegistryPersistence(ctx, step, result)

	case "execute_wmi_query", "simulate_wmi_execution":
		e.executeSimulateWMIExecution(ctx, step, result)

	// Tier 2 simulation actions
	case "request_service_tickets", "simulate_kerberoasting":
		e.executeSimulateKerberoasting(ctx, step, result)

	case "simulate_dcsync":
		e.executeSimulateDCSync(ctx, step, result)

	case "simulate_smb_connection", "simulate_lateral_movement":
		e.executeSimulateSMBLateralMovement(ctx, step, result)

	// Generic verification actions
	case "verify_credential_access_alert":
		e.executeVerifyCredentialAccessAlert(ctx, step, result)

	case "verify_siem_event_correlation":
		e.executeVerifySIEMEventCorrelation(ctx, step, result)

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

// ---------------------------------------------------------------------------
// Tier 1 simulation actions — safe telemetry generation for detection testing
// ---------------------------------------------------------------------------

// executeSimulateLsassAccess creates telemetry artifacts that mimic an LSASS
// access pattern (T1003.001). No real credential dumping occurs — only JSON
// artifacts representing the event are written to a temp directory.
func (e *Executor) executeSimulateLsassAccess(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1003.001"
	accessType, _ := step.Inputs["access_type"].(string)
	if accessType == "" {
		accessType = "benign_handle_request"
	}
	targetProcess, _ := step.Inputs["target_process"].(string)
	if targetProcess == "" {
		targetProcess = "lsass.exe"
	}
	accessRights, _ := step.Inputs["access_rights"].(string)
	if accessRights == "" {
		accessRights = "PROCESS_QUERY_INFORMATION"
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()

	// Artifact 1: Process access event (mimics Sysmon Event ID 10)
	processAccessEvent := map[string]any{
		"event_id":         10,
		"event_type":       "ProcessAccess",
		"timestamp":        now.Format(time.RFC3339),
		"source_process":   "aegisclaw_cred_test.exe",
		"source_pid":       os.Getpid(),
		"target_process":   targetProcess,
		"target_pid":       688,
		"granted_access":   accessRights,
		"call_trace":       "C:\\Windows\\SYSTEM32\\ntdll.dll+a5e14|C:\\Windows\\System32\\KERNELBASE.dll+2c145",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	eventPath := filepath.Join(tmpDir, "process_access_event.json")
	eventData, _ := json.MarshalIndent(processAccessEvent, "", "  ")
	if err := os.WriteFile(eventPath, eventData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing process access event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: Security log entry (mimics Windows Security Event ID 4663)
	securityEvent := map[string]any{
		"event_id":         4663,
		"event_type":       "ObjectAccess",
		"timestamp":        now.Format(time.RFC3339),
		"subject_user":     "aegisclaw-test",
		"object_name":      "\\Device\\HarddiskVolume2\\Windows\\System32\\lsass.exe",
		"object_type":      "Process",
		"access_mask":      "0x1000",
		"process_name":     "aegisclaw_cred_test.exe",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	secPath := filepath.Join(tmpDir, "security_event_4663.json")
	secData, _ := json.MarshalIndent(securityEvent, "", "  ")
	if err := os.WriteFile(secPath, secData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing security event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              "simulate_lsass_access",
		"technique_id":        techniqueID,
		"access_type":         accessType,
		"target_process":      targetProcess,
		"access_rights":       accessRights,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{eventPath, secPath},
		"marker_dir":          tmpDir,
		"marker_path":         eventPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("LSASS access simulation completed",
		"technique", techniqueID,
		"artifacts_dir", tmpDir,
	)
}

// executeSimulatePowershellExecution creates telemetry artifacts that mimic
// encoded PowerShell command execution (T1059.001). No real PowerShell process
// is spawned — only JSON artifacts representing the event are written.
func (e *Executor) executeSimulatePowershellExecution(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1059.001"
	payload, _ := step.Inputs["payload"].(string)
	if payload == "" {
		payload = "V3JpdGUtSG9zdCAiQWVnaXNDbGF3IFRlc3Qi"
	}
	commandType, _ := step.Inputs["command_type"].(string)
	if commandType == "" {
		commandType = "benign_encoded_ps"
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()

	// Artifact 1: Process creation event (mimics Sysmon Event ID 1)
	processEvent := map[string]any{
		"event_id":         1,
		"event_type":       "ProcessCreate",
		"timestamp":        now.Format(time.RFC3339),
		"process_name":     "powershell.exe",
		"process_id":       os.Getpid(),
		"parent_process":   "aegisclaw_runner.exe",
		"command_line":     fmt.Sprintf("powershell.exe -NoProfile -EncodedCommand %s", payload),
		"current_dir":      tmpDir,
		"user":             "aegisclaw-test",
		"integrity_level":  "Medium",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	eventPath := filepath.Join(tmpDir, "process_create_event.json")
	eventData, _ := json.MarshalIndent(processEvent, "", "  ")
	if err := os.WriteFile(eventPath, eventData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing process event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: Script block logging event (mimics PowerShell Event ID 4104)
	decoded, _ := base64.StdEncoding.DecodeString(payload)
	scriptBlockEvent := map[string]any{
		"event_id":         4104,
		"event_type":       "ScriptBlockLogging",
		"timestamp":        now.Format(time.RFC3339),
		"script_block":     string(decoded),
		"script_path":      "",
		"level":            "Warning",
		"message_number":   1,
		"message_total":    1,
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	scriptPath := filepath.Join(tmpDir, "script_block_event_4104.json")
	scriptData, _ := json.MarshalIndent(scriptBlockEvent, "", "  ")
	if err := os.WriteFile(scriptPath, scriptData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing script block event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              "simulate_powershell_execution",
		"technique_id":        techniqueID,
		"command_type":        commandType,
		"encoded_payload":     payload,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{eventPath, scriptPath},
		"marker_dir":          tmpDir,
		"marker_path":         eventPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("PowerShell execution simulation completed",
		"technique", techniqueID,
		"artifacts_dir", tmpDir,
	)
}

// executeSimulateProcessInjection creates telemetry artifacts that mimic
// process injection behavior (T1055). No real injection occurs — only JSON
// artifacts representing cross-process memory operations are written.
func (e *Executor) executeSimulateProcessInjection(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1055"
	injectionType, _ := step.Inputs["injection_type"].(string)
	if injectionType == "" {
		injectionType = "benign_marker"
	}
	targetProcess, _ := step.Inputs["target_process"].(string)
	if targetProcess == "" {
		targetProcess = "aegisclaw_target_process"
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()

	// Artifact 1: CreateRemoteThread event (mimics Sysmon Event ID 8)
	crossProcessEvent := map[string]any{
		"event_id":         8,
		"event_type":       "CreateRemoteThread",
		"timestamp":        now.Format(time.RFC3339),
		"source_process":   "aegisclaw_injector_test.exe",
		"source_pid":       os.Getpid(),
		"target_process":   targetProcess,
		"target_pid":       4200,
		"new_thread_id":    7104,
		"start_address":    "0x00007FFA1B2C0000",
		"start_module":     "ntdll.dll",
		"start_function":   "RtlUserThreadStart",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	eventPath := filepath.Join(tmpDir, "create_remote_thread_event.json")
	eventData, _ := json.MarshalIndent(crossProcessEvent, "", "  ")
	if err := os.WriteFile(eventPath, eventData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing cross-process event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: Cross-process access event (mimics Sysmon Event ID 10)
	memAllocEvent := map[string]any{
		"event_id":         10,
		"event_type":       "ProcessAccess",
		"timestamp":        now.Format(time.RFC3339),
		"source_process":   "aegisclaw_injector_test.exe",
		"source_pid":       os.Getpid(),
		"target_process":   targetProcess,
		"target_pid":       4200,
		"granted_access":   "0x1F0FFF",
		"call_trace":       "C:\\Windows\\SYSTEM32\\ntdll.dll+a5e14|C:\\Windows\\System32\\KERNELBASE.dll+VirtualAllocEx",
		"api_calls":        []string{"OpenProcess", "VirtualAllocEx", "WriteProcessMemory", "CreateRemoteThread"},
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	memPath := filepath.Join(tmpDir, "virtual_alloc_event.json")
	memData, _ := json.MarshalIndent(memAllocEvent, "", "  ")
	if err := os.WriteFile(memPath, memData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing memory alloc event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              "simulate_process_injection",
		"technique_id":        techniqueID,
		"injection_type":      injectionType,
		"target_process":      targetProcess,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{eventPath, memPath},
		"marker_dir":          tmpDir,
		"marker_path":         eventPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("process injection simulation completed",
		"technique", techniqueID,
		"artifacts_dir", tmpDir,
	)
}

// executeSimulateRegistryPersistence creates telemetry artifacts that mimic
// autorun registry key creation (T1547.001). No real registry modification
// occurs — only JSON artifacts representing the registry events are written.
func (e *Executor) executeSimulateRegistryPersistence(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1547.001"
	registryHive, _ := step.Inputs["registry_hive"].(string)
	if registryHive == "" {
		registryHive = "HKCU"
	}
	registryPath, _ := step.Inputs["registry_path"].(string)
	if registryPath == "" {
		registryPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	}
	valueName, _ := step.Inputs["value_name"].(string)
	if valueName == "" {
		valueName = "AegisClawTest"
	}
	valueData, _ := step.Inputs["value_data"].(string)
	if valueData == "" {
		valueData = `C:\Windows\System32\calc.exe`
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()
	fullKeyPath := fmt.Sprintf("%s\\%s", registryHive, registryPath)

	// Artifact 1: Registry value set event (mimics Sysmon Event ID 13)
	registryEvent := map[string]any{
		"event_id":         13,
		"event_type":       "RegistryValueSet",
		"timestamp":        now.Format(time.RFC3339),
		"process_name":     "aegisclaw_persistence_test.exe",
		"process_id":       os.Getpid(),
		"event_subtype":    "SetValue",
		"target_object":    fmt.Sprintf("%s\\%s", fullKeyPath, valueName),
		"value_name":       valueName,
		"value_data":       valueData,
		"value_type":       "REG_SZ",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	eventPath := filepath.Join(tmpDir, "registry_set_value_event.json")
	eventData, _ := json.MarshalIndent(registryEvent, "", "  ")
	if err := os.WriteFile(eventPath, eventData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing registry event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: Registry audit event (mimics Windows Security Event ID 4657)
	auditEvent := map[string]any{
		"event_id":         4657,
		"event_type":       "RegistryAudit",
		"timestamp":        now.Format(time.RFC3339),
		"subject_user":     "aegisclaw-test",
		"object_name":      fullKeyPath,
		"object_value":     valueName,
		"operation_type":   "Existing registry value modified",
		"old_value_type":   "REG_SZ",
		"old_value":        "",
		"new_value_type":   "REG_SZ",
		"new_value":        valueData,
		"process_name":     "aegisclaw_persistence_test.exe",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	auditPath := filepath.Join(tmpDir, "registry_audit_event_4657.json")
	auditData, _ := json.MarshalIndent(auditEvent, "", "  ")
	if err := os.WriteFile(auditPath, auditData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing audit event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              step.Action,
		"technique_id":        techniqueID,
		"registry_key":        fullKeyPath,
		"value_name":          valueName,
		"value_data":          valueData,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{eventPath, auditPath},
		"marker_dir":          tmpDir,
		"marker_path":         eventPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("registry persistence simulation completed",
		"technique", techniqueID,
		"key", fullKeyPath,
		"artifacts_dir", tmpDir,
	)
}

// executeSimulateWMIExecution creates telemetry artifacts that mimic WMI
// command execution (T1047). No real WMI calls are made — only JSON artifacts
// representing the WMI process creation events are written.
func (e *Executor) executeSimulateWMIExecution(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1047"
	wmiClass, _ := step.Inputs["wmi_class"].(string)
	if wmiClass == "" {
		wmiClass = "Win32_OperatingSystem"
	}
	wmiMethod, _ := step.Inputs["wmi_method"].(string)
	if wmiMethod == "" {
		wmiMethod = "get"
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()
	commandLine := fmt.Sprintf("wmic.exe %s %s", wmiClass, wmiMethod)

	// Artifact 1: Process creation event for wmic.exe (mimics Sysmon Event ID 1)
	processEvent := map[string]any{
		"event_id":         1,
		"event_type":       "ProcessCreate",
		"timestamp":        now.Format(time.RFC3339),
		"process_name":     "wmic.exe",
		"process_id":       os.Getpid(),
		"parent_process":   "aegisclaw_runner.exe",
		"command_line":     commandLine,
		"current_dir":      tmpDir,
		"user":             "aegisclaw-test",
		"integrity_level":  "Medium",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	eventPath := filepath.Join(tmpDir, "wmi_process_create_event.json")
	eventData, _ := json.MarshalIndent(processEvent, "", "  ")
	if err := os.WriteFile(eventPath, eventData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing WMI process event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: WMI activity event (mimics WMI-Activity ETW Event ID 5857)
	wmiActivityEvent := map[string]any{
		"event_id":         5857,
		"event_type":       "WMIActivity",
		"timestamp":        now.Format(time.RFC3339),
		"provider_name":    "WMI-Activity",
		"provider_path":    "%SystemRoot%\\System32\\wbem\\WmiPrvSE.exe",
		"query":            fmt.Sprintf("SELECT * FROM %s", wmiClass),
		"operation":        wmiMethod,
		"result_code":      0,
		"user":             "aegisclaw-test",
		"client_machine":   "aegisclaw-test-host",
		"namespace":        "root\\cimv2",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	wmiPath := filepath.Join(tmpDir, "wmi_activity_event_5857.json")
	wmiData, _ := json.MarshalIndent(wmiActivityEvent, "", "  ")
	if err := os.WriteFile(wmiPath, wmiData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing WMI activity event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              step.Action,
		"technique_id":        techniqueID,
		"wmi_class":           wmiClass,
		"wmi_method":          wmiMethod,
		"command_line":        commandLine,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{eventPath, wmiPath},
		"marker_dir":          tmpDir,
		"marker_path":         eventPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("WMI execution simulation completed",
		"technique", techniqueID,
		"wmi_class", wmiClass,
		"artifacts_dir", tmpDir,
	)
}

// ---------------------------------------------------------------------------
// Tier 2 simulation actions — more detailed telemetry for advanced techniques
// ---------------------------------------------------------------------------

// executeSimulateKerberoasting creates telemetry artifacts that mimic
// Kerberoasting activity (T1558.003). No real Kerberos tickets are requested —
// only JSON artifacts representing ticket request events are written.
func (e *Executor) executeSimulateKerberoasting(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1558.003"
	spnTarget, _ := step.Inputs["spn_target"].(string)
	if spnTarget == "" {
		spnTarget = "MSSQLSvc/aegisclaw-test.local:1433"
	}
	encryptionType, _ := step.Inputs["encryption_type"].(string)
	if encryptionType == "" {
		encryptionType = "RC4_HMAC_MD5"
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()

	// Artifact 1: Kerberos TGS request event (mimics Windows Security Event ID 4769)
	tgsEvent := map[string]any{
		"event_id":               4769,
		"event_type":             "KerberosServiceTicketRequest",
		"timestamp":              now.Format(time.RFC3339),
		"account_name":           "aegisclaw-test-account",
		"account_domain":         "AEGISCLAW-TEST",
		"service_name":           spnTarget,
		"service_id":             "S-1-5-21-0000000000-0000000000-0000000000-1234",
		"ticket_encryption_type": encryptionType,
		"ticket_options":         "0x40810000",
		"client_address":         "::ffff:10.0.0.100",
		"client_port":            49152,
		"failure_code":           "0x0",
		"logon_guid":             uuid.New().String(),
		"technique_id":           techniqueID,
		"simulation":             true,
		"aegisclaw_marker":       true,
	}

	tgsPath := filepath.Join(tmpDir, "kerberos_tgs_request_4769.json")
	tgsData, _ := json.MarshalIndent(tgsEvent, "", "  ")
	if err := os.WriteFile(tgsPath, tgsData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing TGS event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: Multiple SPN enumeration events (Kerberoasting pattern)
	spnEnumEvent := map[string]any{
		"event_id":         4769,
		"event_type":       "KerberosServiceTicketRequest",
		"timestamp":        now.Add(100 * time.Millisecond).Format(time.RFC3339),
		"account_name":     "aegisclaw-test-account",
		"account_domain":   "AEGISCLAW-TEST",
		"service_names":    []string{spnTarget, "HTTP/web.aegisclaw-test.local", "MSSQLSvc/db2.aegisclaw-test.local:1433"},
		"encryption_type":  encryptionType,
		"pattern":          "rapid_tgs_requests",
		"request_count":    3,
		"time_window_ms":   500,
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	enumPath := filepath.Join(tmpDir, "spn_enumeration_pattern.json")
	enumData, _ := json.MarshalIndent(spnEnumEvent, "", "  ")
	if err := os.WriteFile(enumPath, enumData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing SPN enumeration event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 3: LDAP SPN query (pre-Kerberoasting reconnaissance)
	ldapQueryEvent := map[string]any{
		"event_id":          3,
		"event_type":        "NetworkConnection",
		"timestamp":         now.Add(-1 * time.Second).Format(time.RFC3339),
		"source_process":    "aegisclaw_kerb_test.exe",
		"source_pid":        os.Getpid(),
		"destination_ip":    "10.0.0.1",
		"destination_port":  389,
		"protocol":          "tcp",
		"ldap_filter":       "(&(servicePrincipalName=*)(!(objectClass=computer)))",
		"description":       "LDAP query for SPN-registered non-computer accounts",
		"technique_id":      techniqueID,
		"simulation":        true,
		"aegisclaw_marker":  true,
	}

	ldapPath := filepath.Join(tmpDir, "ldap_spn_query_event.json")
	ldapData, _ := json.MarshalIndent(ldapQueryEvent, "", "  ")
	if err := os.WriteFile(ldapPath, ldapData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing LDAP query event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              step.Action,
		"technique_id":        techniqueID,
		"spn_target":          spnTarget,
		"encryption_type":     encryptionType,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{tgsPath, enumPath, ldapPath},
		"marker_dir":          tmpDir,
		"marker_path":         tgsPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("Kerberoasting simulation completed",
		"technique", techniqueID,
		"spn_target", spnTarget,
		"artifacts_dir", tmpDir,
	)
}

// executeSimulateDCSync creates telemetry artifacts that mimic a DCSync
// replication request (T1003.006). No real directory replication occurs —
// only JSON artifacts representing the replication events are written.
func (e *Executor) executeSimulateDCSync(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1003.006"
	targetDomain, _ := step.Inputs["target_domain"].(string)
	if targetDomain == "" {
		targetDomain = "aegisclaw-test.local"
	}
	sourceAccount, _ := step.Inputs["source_account"].(string)
	if sourceAccount == "" {
		sourceAccount = "aegisclaw-test-account"
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()
	domainParts := strings.Split(targetDomain, ".")
	domainUpper := strings.ToUpper(domainParts[0])
	namingContext := "DC=" + strings.Join(domainParts, ",DC=")

	// Artifact 1: Directory service access event (mimics Security Event ID 4662)
	replEvent := map[string]any{
		"event_id":         4662,
		"event_type":       "DirectoryServiceAccess",
		"timestamp":        now.Format(time.RFC3339),
		"subject_user":     sourceAccount,
		"subject_domain":   domainUpper,
		"object_type":      "domainDNS",
		"object_name":      namingContext,
		"access_mask":      "0x100",
		"properties":       []string{"{1131f6aa-9c07-11d1-f79f-00c04fc2dcd2}", "{1131f6ad-9c07-11d1-f79f-00c04fc2dcd2}"},
		"property_names":   []string{"DS-Replication-Get-Changes", "DS-Replication-Get-Changes-All"},
		"operation_type":   "Object Access",
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	replPath := filepath.Join(tmpDir, "directory_replication_event_4662.json")
	replData, _ := json.MarshalIndent(replEvent, "", "  ")
	if err := os.WriteFile(replPath, replData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing replication event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: DRSGetNCChanges RPC event (network-level replication indicator)
	drsEvent := map[string]any{
		"event_id":            4662,
		"event_type":          "DRSGetNCChanges",
		"timestamp":           now.Add(50 * time.Millisecond).Format(time.RFC3339),
		"source_address":      "10.0.0.100",
		"source_account":      sourceAccount,
		"destination_address": "10.0.0.1",
		"destination_port":    135,
		"rpc_interface":       "e3514235-4b06-11d1-ab04-00c04fc2dcd2",
		"rpc_operation":       "DRSGetNCChanges",
		"naming_context":      namingContext,
		"source_is_dc":        false,
		"flags":               "DRSUAPI_DRS_INIT_SYNC|DRSUAPI_DRS_WRIT_REP",
		"technique_id":        techniqueID,
		"simulation":          true,
		"aegisclaw_marker":    true,
	}

	drsPath := filepath.Join(tmpDir, "drs_get_nc_changes_event.json")
	drsData, _ := json.MarshalIndent(drsEvent, "", "  ")
	if err := os.WriteFile(drsPath, drsData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing DRS event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 3: Authentication anomaly (non-DC performing replication)
	anomalyEvent := map[string]any{
		"event_id":            4624,
		"event_type":          "LogonEvent",
		"timestamp":           now.Add(-500 * time.Millisecond).Format(time.RFC3339),
		"account_name":        sourceAccount,
		"account_domain":      domainUpper,
		"logon_type":          3,
		"source_address":      "10.0.0.100",
		"source_port":         49200,
		"logon_process":       "NtLmSsp",
		"authentication_pkg":  "NTLM",
		"anomaly_flags":       []string{"non_dc_replication_source", "sensitive_privilege_use"},
		"technique_id":        techniqueID,
		"simulation":          true,
		"aegisclaw_marker":    true,
	}

	anomalyPath := filepath.Join(tmpDir, "auth_anomaly_event_4624.json")
	anomalyData, _ := json.MarshalIndent(anomalyEvent, "", "  ")
	if err := os.WriteFile(anomalyPath, anomalyData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing anomaly event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              "simulate_dcsync",
		"technique_id":        techniqueID,
		"target_domain":       targetDomain,
		"source_account":      sourceAccount,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{replPath, drsPath, anomalyPath},
		"marker_dir":          tmpDir,
		"marker_path":         replPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("DCSync simulation completed",
		"technique", techniqueID,
		"target_domain", targetDomain,
		"artifacts_dir", tmpDir,
	)
}

// executeSimulateSMBLateralMovement creates telemetry artifacts that mimic
// SMB lateral movement (T1021.002). No real network connections are made —
// only JSON artifacts representing SMB connection events are written.
func (e *Executor) executeSimulateSMBLateralMovement(_ context.Context, step *PlaybookStep, result *StepResult) {
	techniqueID := "T1021.002"
	targetShare, _ := step.Inputs["target_share"].(string)
	if targetShare == "" {
		targetShare = `\\aegisclaw-target\C$`
	}
	authMethod, _ := step.Inputs["authentication_method"].(string)
	if authMethod == "" {
		authMethod = "ntlm"
	}

	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-*")
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("creating temp directory: %v", err)
		return
	}

	now := time.Now().UTC()

	// Artifact 1: Network connection event to port 445 (mimics Sysmon Event ID 3)
	networkEvent := map[string]any{
		"event_id":            3,
		"event_type":          "NetworkConnection",
		"timestamp":           now.Format(time.RFC3339),
		"source_process":      "aegisclaw_lateral_test.exe",
		"source_pid":          os.Getpid(),
		"source_address":      "10.0.0.100",
		"source_port":         49300,
		"destination_address": "10.0.0.200",
		"destination_port":    445,
		"protocol":            "tcp",
		"initiated":           true,
		"technique_id":        techniqueID,
		"simulation":          true,
		"aegisclaw_marker":    true,
	}

	netPath := filepath.Join(tmpDir, "smb_network_connection_event.json")
	netData, _ := json.MarshalIndent(networkEvent, "", "  ")
	if err := os.WriteFile(netPath, netData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing network event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 2: SMB share access event (mimics Security Event ID 5140)
	shareAccessEvent := map[string]any{
		"event_id":         5140,
		"event_type":       "NetworkShareAccess",
		"timestamp":        now.Add(50 * time.Millisecond).Format(time.RFC3339),
		"subject_user":     "aegisclaw-test",
		"subject_domain":   "AEGISCLAW-TEST",
		"object_type":      "File",
		"share_name":       targetShare,
		"share_path":       `\??\C:\`,
		"access_mask":      "0x1",
		"access_list":      "ReadData (or ListDirectory)",
		"source_address":   "10.0.0.100",
		"source_port":      49300,
		"technique_id":     techniqueID,
		"simulation":       true,
		"aegisclaw_marker": true,
	}

	sharePath := filepath.Join(tmpDir, "smb_share_access_event_5140.json")
	shareData, _ := json.MarshalIndent(shareAccessEvent, "", "  ")
	if err := os.WriteFile(sharePath, shareData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing share access event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	// Artifact 3: Network logon event (mimics Security Event ID 4624 Type 3)
	logonEvent := map[string]any{
		"event_id":           4624,
		"event_type":         "LogonEvent",
		"timestamp":          now.Add(-200 * time.Millisecond).Format(time.RFC3339),
		"account_name":       "aegisclaw-test",
		"account_domain":     "AEGISCLAW-TEST",
		"logon_type":         3,
		"logon_process":      "NtLmSsp",
		"authentication_pkg": strings.ToUpper(authMethod),
		"source_address":     "10.0.0.100",
		"source_port":        49299,
		"workstation_name":   "AEGISCLAW-SRC",
		"logon_id":           "0x00000000000F4240",
		"technique_id":       techniqueID,
		"simulation":         true,
		"aegisclaw_marker":   true,
	}

	logonPath := filepath.Join(tmpDir, "network_logon_event_4624.json")
	logonData, _ := json.MarshalIndent(logonEvent, "", "  ")
	if err := os.WriteFile(logonPath, logonData, 0644); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("writing logon event: %v", err)
		os.RemoveAll(tmpDir)
		return
	}

	result.Status = "completed"
	result.CleanupDone = false
	outputs := map[string]any{
		"action":              step.Action,
		"technique_id":        techniqueID,
		"target_share":        targetShare,
		"auth_method":         authMethod,
		"simulation_method":   "telemetry_artifact_generation",
		"telemetry_generated": true,
		"artifacts":           []string{netPath, sharePath, logonPath},
		"marker_dir":          tmpDir,
		"marker_path":         netPath,
		"created_at":          now.Format(time.RFC3339),
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data

	e.logger.Info("SMB lateral movement simulation completed",
		"technique", techniqueID,
		"target_share", targetShare,
		"artifacts_dir", tmpDir,
	)
}

// ---------------------------------------------------------------------------
// Generic verification actions — technique-specific connector queries
// ---------------------------------------------------------------------------

// executeVerifyCredentialAccessAlert queries EDR for credential access alerts
// matching a specific technique. Uses the same connector pattern as verify_detection
// but with credential-access-specific queries.
func (e *Executor) executeVerifyCredentialAccessAlert(ctx context.Context, step *PlaybookStep, result *StepResult) {
	connectorID, _ := step.Inputs["connector_id"].(string)
	techniqueID, _ := step.Inputs["expected_technique"].(string)
	if techniqueID == "" {
		techniqueID, _ = step.Inputs["technique_id"].(string)
	}
	expectedSeverity, _ := step.Inputs["expected_severity"].(string)
	if expectedSeverity == "" {
		expectedSeverity = "high"
	}

	svc := e.getConnectorSvc()
	if connectorID == "" || svc == nil {
		result.Status = "completed"
		outputs := map[string]any{
			"action":       "verify_credential_access_alert",
			"detected":     false,
			"no_connector": true,
			"technique_id": techniqueID,
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
	query := fmt.Sprintf("credential access alert %s severity:%s", techniqueID, expectedSeverity)

	eventQuery := connectorsdk.EventQuery{
		TimeRange: connectorsdk.TimeRange{
			Start: now.Add(-10 * time.Minute),
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
		result.Error = fmt.Sprintf("verify_credential_access_alert: %v", err)
		outputs := map[string]any{
			"action":       "verify_credential_access_alert",
			"detected":     false,
			"technique_id": techniqueID,
			"error":        err.Error(),
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	detected := eventResult.TotalCount > 0
	result.Status = "completed"
	outputs := map[string]any{
		"action":            "verify_credential_access_alert",
		"detected":          detected,
		"alert_count":       eventResult.TotalCount,
		"latency":           latency.String(),
		"technique_id":      techniqueID,
		"expected_severity": expectedSeverity,
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data
}

// executeVerifySIEMEventCorrelation queries SIEM for correlated events matching
// a specific technique. Uses the same connector pattern as verify_detection but
// with correlation-specific queries and wider time windows.
func (e *Executor) executeVerifySIEMEventCorrelation(ctx context.Context, step *PlaybookStep, result *StepResult) {
	connectorID, _ := step.Inputs["connector_id"].(string)
	techniqueID, _ := step.Inputs["expected_technique"].(string)
	if techniqueID == "" {
		techniqueID, _ = step.Inputs["technique_id"].(string)
	}
	eventCategory, _ := step.Inputs["event_category"].(string)
	if eventCategory == "" {
		eventCategory = "security"
	}
	timeRangeMinutes := 30
	if trm, ok := step.Inputs["time_range_minutes"].(float64); ok && trm > 0 {
		timeRangeMinutes = int(trm)
	}
	minExpectedEvents := 1
	if mee, ok := step.Inputs["min_expected_events"].(float64); ok && mee > 0 {
		minExpectedEvents = int(mee)
	}

	svc := e.getConnectorSvc()
	if connectorID == "" || svc == nil {
		result.Status = "completed"
		outputs := map[string]any{
			"action":           "verify_siem_event_correlation",
			"correlated":       false,
			"no_connector":     true,
			"technique_id":     techniqueID,
			"event_category":   eventCategory,
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
	query := fmt.Sprintf("correlated events category:%s technique:%s", eventCategory, techniqueID)

	eventQuery := connectorsdk.EventQuery{
		TimeRange: connectorsdk.TimeRange{
			Start: now.Add(-time.Duration(timeRangeMinutes) * time.Minute),
			End:   now,
		},
		Query:      query,
		MaxResults: 200,
	}

	startTime := time.Now()
	eventResult, err := svc.QueryEvents(ctx, connUUID, eventQuery)
	latency := time.Since(startTime)

	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("verify_siem_event_correlation: %v", err)
		outputs := map[string]any{
			"action":         "verify_siem_event_correlation",
			"correlated":     false,
			"technique_id":   techniqueID,
			"event_category": eventCategory,
			"error":          err.Error(),
		}
		data, _ := json.Marshal(outputs)
		result.Outputs = data
		return
	}

	correlated := eventResult.TotalCount >= minExpectedEvents
	result.Status = "completed"
	outputs := map[string]any{
		"action":              "verify_siem_event_correlation",
		"correlated":          correlated,
		"event_count":         eventResult.TotalCount,
		"min_expected_events": minExpectedEvents,
		"latency":             latency.String(),
		"technique_id":        techniqueID,
		"event_category":      eventCategory,
		"time_range_minutes":  timeRangeMinutes,
	}
	data, _ := json.Marshal(outputs)
	result.Outputs = data
}
