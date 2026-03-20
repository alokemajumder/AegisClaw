package playbook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestExecutor() *Executor {
	return NewExecutor(nil, testLogger())
}

func newTestPlaybook() *Playbook {
	return &Playbook{
		ID:          "test-playbook",
		Name:        "Test Playbook",
		Tier:        0,
		TechniqueID: "T1059.001",
		Steps:       []PlaybookStep{},
	}
}

func parseOutputs(t *testing.T, result *StepResult) map[string]any {
	t.Helper()
	var out map[string]any
	require.NotNil(t, result.Outputs, "outputs should not be nil")
	err := json.Unmarshal(result.Outputs, &out)
	require.NoError(t, err, "outputs should be valid JSON")
	return out
}

// ─── query_telemetry ──────────────────────────────────────────────────────────

func TestExecuteStep_QueryTelemetry_NoConnector(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()
	step := &PlaybookStep{
		Name:   "query telemetry",
		Action: "query_telemetry",
		Inputs: map[string]any{
			"connector_id": "",
			"query":        "process.name:powershell.exe",
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)

	out := parseOutputs(t, result)
	assert.Equal(t, true, out["no_connector"])
	assert.Equal(t, float64(0), out["event_count"])
}

// ─── check_edr_agents ─────────────────────────────────────────────────────────

func TestExecuteStep_CheckEDRAgents_NoConnector(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()
	step := &PlaybookStep{
		Name:   "check EDR agents",
		Action: "check_edr_agents",
		Inputs: map[string]any{},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)

	out := parseOutputs(t, result)
	assert.Equal(t, true, out["no_connector"])
	assert.Equal(t, float64(0), out["agents_found"])
}

// ─── drop_marker_file ─────────────────────────────────────────────────────────

func TestExecuteStep_DropMarkerFile(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()
	step := &PlaybookStep{
		Name:   "drop marker file",
		Action: "drop_marker_file",
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.False(t, result.CleanupDone, "cleanup should not be done yet — that happens in verify_cleanup")

	out := parseOutputs(t, result)
	markerPath, ok := out["marker_path"].(string)
	require.True(t, ok, "marker_path should be a string in outputs")
	assert.NotEmpty(t, markerPath)

	// Verify the file was actually created
	_, statErr := os.Stat(markerPath)
	assert.NoError(t, statErr, "marker file should exist on disk")

	// Cleanup
	markerDir, _ := out["marker_dir"].(string)
	if markerDir != "" {
		os.RemoveAll(markerDir)
	}
}

// ─── verify_cleanup ───────────────────────────────────────────────────────────

func TestExecuteStep_VerifyCleanup_FileGone(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	// Create a temp dir that matches the aegisclaw naming convention
	tmpDir, err := os.MkdirTemp("", "aegisclaw-marker-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	markerPath := filepath.Join(tmpDir, "eicar-test.txt")
	// Do NOT create the file — simulate EDR already removed it

	step := &PlaybookStep{
		Name:   "verify cleanup",
		Action: "verify_cleanup",
		Inputs: map[string]any{
			"marker_path": markerPath,
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.True(t, result.CleanupDone)

	out := parseOutputs(t, result)
	assert.Equal(t, true, out["cleanup_confirmed"])
	assert.Equal(t, false, out["self_cleaned"])
}

func TestExecuteStep_VerifyCleanup_PathTraversal(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	tests := []struct {
		name       string
		markerPath string
	}{
		{
			name:       "outside temp dir",
			markerPath: "/etc/passwd",
		},
		{
			name:       "relative path escape",
			markerPath: filepath.Join(os.TempDir(), "not-aegisclaw", "eicar.txt"),
		},
		{
			name:       "home directory",
			markerPath: filepath.Join(os.Getenv("HOME"), "eicar.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &PlaybookStep{
				Name:   "verify cleanup",
				Action: "verify_cleanup",
				Inputs: map[string]any{
					"marker_path": tt.markerPath,
				},
			}

			result, err := exec.ExecuteStep(context.Background(), pb, step)
			require.NoError(t, err)
			assert.Equal(t, "failed", result.Status)
			assert.Contains(t, result.Error, "not in an aegisclaw temp directory")
		})
	}
}

// ─── execute_encoded_command ──────────────────────────────────────────────────

func TestExecuteStep_ExecuteEncodedCommand_Allowed(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	// "hostname" is in the allowlist
	encodedCmd := base64.StdEncoding.EncodeToString([]byte("hostname"))

	step := &PlaybookStep{
		Name:   "execute hostname",
		Action: "execute_encoded_command",
		Inputs: map[string]any{
			"command": encodedCmd,
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)
	assert.True(t, result.CleanupDone)

	out := parseOutputs(t, result)
	assert.Equal(t, "hostname", out["command"])
	assert.NotEmpty(t, out["output"], "hostname should produce output")
	assert.Equal(t, float64(0), out["exit_code"])
}

func TestExecuteStep_ExecuteEncodedCommand_Blocked(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	blockedCmds := []string{"rm", "cat", "curl", "wget", "python", "bash", "sh"}

	for _, cmd := range blockedCmds {
		t.Run(cmd, func(t *testing.T) {
			encodedCmd := base64.StdEncoding.EncodeToString([]byte(cmd))
			step := &PlaybookStep{
				Name:   "blocked command",
				Action: "execute_encoded_command",
				Inputs: map[string]any{
					"command": encodedCmd,
				},
			}

			result, err := exec.ExecuteStep(context.Background(), pb, step)
			require.NoError(t, err)
			assert.Equal(t, "failed", result.Status)
			assert.Contains(t, result.Error, "not in allowlist")
		})
	}
}

func TestExecuteStep_ExecuteEncodedCommand_WithArgs(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	// "hostname -f" — arguments should be rejected
	encodedCmd := base64.StdEncoding.EncodeToString([]byte("hostname -f"))

	step := &PlaybookStep{
		Name:   "command with args",
		Action: "execute_encoded_command",
		Inputs: map[string]any{
			"command": encodedCmd,
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "arguments not allowed")
}

func TestExecuteStep_ExecuteEncodedCommand_PathTraversal(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	// /usr/bin/rm should resolve to "rm" via filepath.Base, then be blocked
	encodedCmd := base64.StdEncoding.EncodeToString([]byte("/usr/bin/rm"))

	step := &PlaybookStep{
		Name:   "path traversal attempt",
		Action: "execute_encoded_command",
		Inputs: map[string]any{
			"command": encodedCmd,
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "not in allowlist")
}

func TestExecuteStep_ExecuteEncodedCommand_EmptyCommand(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	step := &PlaybookStep{
		Name:   "empty command",
		Action: "execute_encoded_command",
		Inputs: map[string]any{
			"command": "",
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "missing required input")
}

func TestExecuteStep_ExecuteEncodedCommand_InvalidBase64(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	step := &PlaybookStep{
		Name:   "invalid base64",
		Action: "execute_encoded_command",
		Inputs: map[string]any{
			"command": "not-valid-base64!!!",
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "decoding base64")
}

// ─── unknown action ───────────────────────────────────────────────────────────

func TestExecuteStep_UnknownAction(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	step := &PlaybookStep{
		Name:   "unknown",
		Action: "totally_unknown_action",
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "unknown action")

	out := parseOutputs(t, result)
	assert.Equal(t, "totally_unknown_action", out["action"])
}

// ─── timeout ──────────────────────────────────────────────────────────────────

func TestExecuteStep_Timeout(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	// Use a step with a very short timeout and an action that will take time.
	// We use verify_cleanup with a missing marker_path which is fast, but with
	// a pre-cancelled context to simulate timeout.
	step := &PlaybookStep{
		Name:           "timeout test",
		Action:         "verify_cleanup",
		TimeoutSeconds: 1,
		Inputs: map[string]any{
			"marker_path": "",
		},
	}

	// Pre-cancel the parent context to trigger the timeout check at the end
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	// Give the context time to expire
	time.Sleep(5 * time.Millisecond)

	result, err := exec.ExecuteStep(ctx, pb, step)
	require.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "timed out")
}

// ─── verify_cleanup with missing marker_path ──────────────────────────────────

func TestExecuteStep_VerifyCleanup_MissingMarkerPath(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	step := &PlaybookStep{
		Name:   "verify cleanup missing path",
		Action: "verify_cleanup",
		Inputs: map[string]any{},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "failed", result.Status)
	assert.Contains(t, result.Error, "missing required input")
}

// ─── verify_detection with no connector ───────────────────────────────────────

func TestExecuteStep_VerifyDetection_NoConnector(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	step := &PlaybookStep{
		Name:   "verify detection no connector",
		Action: "verify_detection",
		Inputs: map[string]any{
			"connector_id": "",
			"technique_id": "T1059",
		},
	}

	result, err := exec.ExecuteStep(context.Background(), pb, step)
	require.NoError(t, err)
	assert.Equal(t, "completed", result.Status)

	out := parseOutputs(t, result)
	assert.Equal(t, true, out["no_connector"])
	assert.Equal(t, false, out["detected"])
}

// ─── Simulation actions (Tier 1) ──────────────────────────────────────────────

func TestExecuteStep_SimulationActions(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()
	pb.Tier = 1

	simulationActions := []string{
		"simulate_lsass_access",
		"simulate_powershell_execution",
		"simulate_process_injection",
		"simulate_registry_persistence",
		"simulate_wmi_execution",
		"simulate_kerberoasting",
		"simulate_dcsync",
		"simulate_lateral_movement",
		"verify_credential_access_alert",
		"verify_siem_event_correlation",
	}

	for _, action := range simulationActions {
		t.Run(action, func(t *testing.T) {
			step := &PlaybookStep{
				Name:   action,
				Action: action,
				Inputs: map[string]any{
					"connector_id": "",
				},
			}

			result, err := exec.ExecuteStep(context.Background(), pb, step)
			require.NoError(t, err)
			// Simulation actions should complete (they produce simulated outputs)
			assert.Equal(t, "completed", result.Status, "action %s should complete", action)
		})
	}
}

// ─── drop_marker_file then verify_cleanup (integration) ───────────────────────

func TestExecuteStep_DropAndVerify_Integration(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	// Step 1: Drop marker file
	dropStep := &PlaybookStep{
		Name:   "drop marker",
		Action: "drop_marker_file",
	}
	dropResult, err := exec.ExecuteStep(context.Background(), pb, dropStep)
	require.NoError(t, err)
	require.Equal(t, "completed", dropResult.Status)

	out := parseOutputs(t, dropResult)
	markerPath := out["marker_path"].(string)
	markerDir := out["marker_dir"].(string)
	defer os.RemoveAll(markerDir)

	// Verify marker file exists
	_, statErr := os.Stat(markerPath)
	require.NoError(t, statErr)

	// Step 2: Verify cleanup (file still exists, will self-clean)
	verifyStep := &PlaybookStep{
		Name:   "verify cleanup",
		Action: "verify_cleanup",
		Inputs: map[string]any{
			"marker_path": markerPath,
		},
	}
	verifyResult, err := exec.ExecuteStep(context.Background(), pb, verifyStep)
	require.NoError(t, err)
	assert.Equal(t, "completed", verifyResult.Status)
	assert.True(t, verifyResult.CleanupDone)

	verifyOut := parseOutputs(t, verifyResult)
	assert.Equal(t, true, verifyOut["self_cleaned"])
}

// ─── execute_encoded_command with all safe commands ───────────────────────────

func TestExecuteStep_ExecuteEncodedCommand_AllSafeCommands(t *testing.T) {
	exec := newTestExecutor()
	pb := newTestPlaybook()

	// Only test commands that exist on the current platform
	cmds := []string{"whoami", "hostname", "ls", "uname", "id", "pwd"}

	for _, cmd := range cmds {
		t.Run(cmd, func(t *testing.T) {
			// Skip commands that don't exist on this platform
			if _, err := os.Stat("/usr/bin/" + cmd); os.IsNotExist(err) {
				// Try PATH lookup
				if _, lookupErr := os.Stat("/bin/" + cmd); os.IsNotExist(lookupErr) {
					// Command might still be in PATH, let the test run anyway
				}
			}

			encodedCmd := base64.StdEncoding.EncodeToString([]byte(cmd))
			step := &PlaybookStep{
				Name:   "safe cmd " + cmd,
				Action: "execute_encoded_command",
				Inputs: map[string]any{
					"command": encodedCmd,
				},
			}

			result, err := exec.ExecuteStep(context.Background(), pb, step)
			require.NoError(t, err)
			// The command should either succeed or fail with execution error,
			// but NOT be blocked by the allowlist.
			if result.Status == "failed" {
				assert.NotContains(t, result.Error, "not in allowlist",
					"command %q should be in the allowlist", cmd)
			}
		})
	}
}
