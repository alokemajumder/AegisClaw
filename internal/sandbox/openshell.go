// Package sandbox provides agent execution isolation via NVIDIA NemoClaw/OpenShell.
//
// OpenShell enforces policy-based security, network, and privacy guardrails for
// autonomous agents using Landlock + seccomp + network namespaces.
//
// AegisClaw's governance tiers map to OpenShell sandbox policies:
//   - Tier 0 (Passive):     Network: allow outbound to SIEM/EDR. Filesystem: read-only.
//   - Tier 1 (Benign):      Network: allow SIEM/EDR + temp file creation. Filesystem: /tmp write.
//   - Tier 2 (Sensitive):   Network: restricted. Filesystem: /sandbox + /tmp. Requires approval.
//   - Tier 3 (Prohibited):  Always blocked — never reaches sandbox.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// Config holds NemoClaw/OpenShell sandbox configuration.
type Config struct {
	Enabled        bool   `mapstructure:"enabled"`
	RuntimeURL     string `mapstructure:"runtime_url"`     // OpenShell runtime endpoint (e.g. http://localhost:8765)
	PolicyDir      string `mapstructure:"policy_dir"`      // Directory containing .yaml policy files
	TimeoutSeconds int    `mapstructure:"timeout_seconds"` // Per-step execution timeout
	MaxMemoryMB    int    `mapstructure:"max_memory_mb"`   // Memory limit per sandbox
	MaxCPUCores    int    `mapstructure:"max_cpu_cores"`   // CPU core limit per sandbox
	NetworkPolicy  string `mapstructure:"network_policy"`  // Default network policy (deny_all, allow_connectors, allow_all)
}

// SandboxPolicy maps AegisClaw governance tiers to OpenShell isolation policies.
type SandboxPolicy struct {
	Tier           int               `json:"tier"`
	NetworkRules   []NetworkRule     `json:"network_rules"`
	FilesystemMode string            `json:"filesystem_mode"` // read_only, sandbox_write, full_write
	AllowedPaths   []string          `json:"allowed_paths"`
	DeniedSyscalls []string          `json:"denied_syscalls"`
	MaxMemoryMB    int               `json:"max_memory_mb"`
	MaxCPUCores    int               `json:"max_cpu_cores"`
	Labels         map[string]string `json:"labels,omitempty"`
}

// NetworkRule defines an allowed network connection from within the sandbox.
type NetworkRule struct {
	Direction string `json:"direction"` // egress, ingress
	Protocol  string `json:"protocol"`  // tcp, udp
	Host      string `json:"host"`      // hostname or CIDR
	Port      int    `json:"port"`
	Allow     bool   `json:"allow"`
}

// ExecutionRequest is sent to the OpenShell runtime to execute a step.
type ExecutionRequest struct {
	ID            string          `json:"id"`
	Policy        SandboxPolicy   `json:"policy"`
	Command       string          `json:"command,omitempty"`
	Action        string          `json:"action"`
	Inputs        json.RawMessage `json:"inputs"`
	TimeoutSecs   int             `json:"timeout_seconds"`
	InferenceGW   string          `json:"inference_gateway,omitempty"` // NIM endpoint for LLM calls within sandbox
	ConnectorURLs map[string]string `json:"connector_urls,omitempty"`  // Allowed external endpoints
}

// ExecutionResult is returned from the OpenShell runtime after execution.
type ExecutionResult struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"` // completed, failed, timeout, policy_violation
	ExitCode    int             `json:"exit_code"`
	Stdout      string          `json:"stdout,omitempty"`
	Stderr      string          `json:"stderr,omitempty"`
	Outputs     json.RawMessage `json:"outputs,omitempty"`
	Artifacts   []string        `json:"artifacts,omitempty"`
	Duration    time.Duration   `json:"duration"`
	PolicyEvent string          `json:"policy_event,omitempty"` // If sandbox policy was triggered
}

// Manager manages sandbox lifecycle and policy enforcement.
type Manager struct {
	config Config
	logger *slog.Logger
}

// NewManager creates a new sandbox manager.
func NewManager(cfg Config, logger *slog.Logger) *Manager {
	return &Manager{
		config: cfg,
		logger: logger,
	}
}

// IsEnabled returns whether sandbox execution is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// PolicyForTier returns the appropriate sandbox policy for a governance tier.
// Tier 3 is never sandboxed — it must be blocked by PolicyEnforcer before reaching here.
func (m *Manager) PolicyForTier(tier int, connectorURLs map[string]string) (*SandboxPolicy, error) {
	switch tier {
	case 0:
		return m.passivePolicy(connectorURLs), nil
	case 1:
		return m.benignPolicy(connectorURLs), nil
	case 2:
		return m.sensitivePolicy(connectorURLs), nil
	case 3:
		return nil, fmt.Errorf("tier 3 actions are prohibited and must not reach sandbox")
	default:
		return nil, fmt.Errorf("unknown governance tier: %d", tier)
	}
}

// passivePolicy: read-only filesystem, outbound to SIEM/EDR only.
func (m *Manager) passivePolicy(connectorURLs map[string]string) *SandboxPolicy {
	rules := m.connectorNetworkRules(connectorURLs)
	return &SandboxPolicy{
		Tier:           0,
		NetworkRules:   rules,
		FilesystemMode: "read_only",
		AllowedPaths:   []string{"/sandbox"},
		DeniedSyscalls: []string{"ptrace", "mount", "chroot", "reboot", "kexec_load"},
		MaxMemoryMB:    min(m.config.MaxMemoryMB, 512),
		MaxCPUCores:    min(m.config.MaxCPUCores, 1),
		Labels:         map[string]string{"tier": "0", "scope": "passive"},
	}
}

// benignPolicy: /tmp write access, outbound to connectors, temp file creation allowed.
func (m *Manager) benignPolicy(connectorURLs map[string]string) *SandboxPolicy {
	rules := m.connectorNetworkRules(connectorURLs)
	return &SandboxPolicy{
		Tier:           1,
		NetworkRules:   rules,
		FilesystemMode: "sandbox_write",
		AllowedPaths:   []string{"/sandbox", "/tmp"},
		DeniedSyscalls: []string{"ptrace", "mount", "chroot", "reboot", "kexec_load"},
		MaxMemoryMB:    min(m.config.MaxMemoryMB, 1024),
		MaxCPUCores:    min(m.config.MaxCPUCores, 2),
		Labels:         map[string]string{"tier": "1", "scope": "benign"},
	}
}

// sensitivePolicy: restricted network, /sandbox + /tmp write, requires human approval upstream.
func (m *Manager) sensitivePolicy(connectorURLs map[string]string) *SandboxPolicy {
	rules := m.connectorNetworkRules(connectorURLs)
	return &SandboxPolicy{
		Tier:           2,
		NetworkRules:   rules,
		FilesystemMode: "sandbox_write",
		AllowedPaths:   []string{"/sandbox", "/tmp"},
		DeniedSyscalls: []string{"ptrace", "mount", "chroot", "reboot", "kexec_load", "init_module", "finit_module"},
		MaxMemoryMB:    min(m.config.MaxMemoryMB, 2048),
		MaxCPUCores:    min(m.config.MaxCPUCores, 2),
		Labels:         map[string]string{"tier": "2", "scope": "sensitive"},
	}
}

// connectorNetworkRules creates egress rules for configured connector endpoints.
func (m *Manager) connectorNetworkRules(connectorURLs map[string]string) []NetworkRule {
	var rules []NetworkRule

	// Deny all by default
	rules = append(rules, NetworkRule{
		Direction: "egress",
		Protocol:  "tcp",
		Host:      "0.0.0.0/0",
		Port:      0,
		Allow:     false,
	})

	// Allow DNS resolution
	rules = append(rules, NetworkRule{
		Direction: "egress",
		Protocol:  "udp",
		Host:      "0.0.0.0/0",
		Port:      53,
		Allow:     true,
	})

	// Allow connector endpoints
	for name, url := range connectorURLs {
		rules = append(rules, NetworkRule{
			Direction: "egress",
			Protocol:  "tcp",
			Host:      url,
			Port:      443,
			Allow:     true,
		})
		m.logger.Debug("sandbox: allowing egress to connector", "connector", name, "url", url)
	}

	return rules
}

// Execute runs an action within a sandboxed environment.
// If OpenShell is not available, falls back to in-process execution with a warning.
func (m *Manager) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("sandbox execution is not enabled")
	}

	m.logger.Info("executing in sandbox",
		"id", req.ID,
		"action", req.Action,
		"tier", req.Policy.Tier,
		"timeout", req.TimeoutSecs,
	)

	// Apply execution timeout
	timeout := time.Duration(req.TimeoutSecs) * time.Second
	if timeout == 0 {
		timeout = time.Duration(m.config.TimeoutSeconds) * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_ = execCtx // OpenShell HTTP client would use this context

	// The actual OpenShell API call would be:
	//   POST {runtime_url}/v1/execute
	//   Body: ExecutionRequest (with policy, command, inputs)
	//   Response: ExecutionResult
	//
	// OpenShell creates a Landlock+seccomp+netns sandbox matching the policy,
	// executes the action, captures outputs and artifacts, then tears down.
	//
	// For now, return a placeholder indicating the sandbox interface is ready
	// for integration with the NemoClaw runtime when deployed.
	return nil, fmt.Errorf("OpenShell runtime not connected at %s — use in-process fallback", m.config.RuntimeURL)
}

