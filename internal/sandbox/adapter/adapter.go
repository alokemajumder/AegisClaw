// Package adapter bridges internal/sandbox and internal/playbook without
// creating a circular import. It adapts sandbox.Manager to the
// playbook.SandboxExecutor interface.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alokemajumder/AegisClaw/internal/playbook"
	"github.com/alokemajumder/AegisClaw/internal/sandbox"
)

// SandboxAdapter wraps a sandbox.Manager and implements playbook.SandboxExecutor.
type SandboxAdapter struct {
	mgr *sandbox.Manager
}

// New creates a SandboxAdapter wrapping the given sandbox.Manager.
func New(mgr *sandbox.Manager) *SandboxAdapter {
	return &SandboxAdapter{mgr: mgr}
}

// IsEnabled delegates to the underlying sandbox manager.
func (a *SandboxAdapter) IsEnabled() bool {
	return a.mgr.IsEnabled()
}

// IsGatewayConnected delegates to the underlying sandbox manager.
func (a *SandboxAdapter) IsGatewayConnected() bool {
	return a.mgr.IsGatewayConnected()
}

// ExecutePlaybookStep converts between sandbox and playbook types, routing
// the step through the OpenShell sandbox.
func (a *SandboxAdapter) ExecutePlaybookStep(ctx context.Context, tier int, action string, inputs json.RawMessage, timeoutSecs int) (*playbook.SandboxStepResult, error) {
	reqID := fmt.Sprintf("%s-%d", action, time.Now().UnixNano())

	req := sandbox.ExecutionRequest{
		ID:          reqID,
		Tier:        tier,
		Action:      action,
		Inputs:      inputs,
		TimeoutSecs: timeoutSecs,
	}

	execResult, err := a.mgr.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	return &playbook.SandboxStepResult{
		Status:   execResult.Status,
		ExitCode: execResult.ExitCode,
		Outputs:  execResult.Outputs,
		Duration: execResult.Duration,
		Error:    execResult.Stderr,
	}, nil
}
