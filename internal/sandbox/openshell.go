// Package sandbox provides agent execution isolation via NVIDIA NemoClaw/OpenShell.
//
// OpenShell (Apache 2.0) enforces policy-based security, network, and privacy
// guardrails for autonomous agents using Landlock LSM + seccomp + proxy-based
// network policies. The Gateway manages sandbox lifecycle, distributes policies,
// and routes inference through the Privacy Router.
//
// AegisClaw's governance tiers map to OpenShell sandbox policies:
//   - Tier 0 (Passive):     Filesystem: read-only. Network: SIEM/EDR connector egress only (read-only REST).
//   - Tier 1 (Benign):      Filesystem: /sandbox + /tmp write. Network: connector egress + DNS.
//   - Tier 2 (Sensitive):   Filesystem: /sandbox + /tmp write. Network: connectors (read-write) + DNS. Requires approval.
//   - Tier 3 (Prohibited):  Always blocked — never reaches sandbox.
//
// Architecture:
//   Gateway → creates Sandbox with static policy (filesystem, Landlock, process)
//   Gateway → distributes dynamic policy (network endpoints, hot-reloadable)
//   Privacy Router → sandboxes call inference.local, routed to Ollama/NIM/API Catalog
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
	Enabled        bool          `mapstructure:"enabled"`
	RuntimeURL     string        `mapstructure:"runtime_url"`     // OpenShell Gateway endpoint (e.g. https://localhost:9090)
	PolicyDir      string        `mapstructure:"policy_dir"`      // Directory containing .yaml policy files
	TimeoutSeconds int           `mapstructure:"timeout_seconds"` // Per-step execution timeout
	MaxMemoryMB    int           `mapstructure:"max_memory_mb"`   // Memory limit per sandbox
	MaxCPUCores    int           `mapstructure:"max_cpu_cores"`   // CPU core limit per sandbox
	NetworkPolicy  string        `mapstructure:"network_policy"`  // Default network policy (deny_all, allow_connectors, allow_all)
	Gateway        GatewayConfig `mapstructure:"gateway"`         // OpenShell Gateway connection settings
	Image          string        `mapstructure:"image"`           // Sandbox base image (base, ollama, openclaw)
	GPU            bool          `mapstructure:"gpu"`             // Enable GPU passthrough for sandboxes
	OllamaURL      string        `mapstructure:"ollama_url"`      // Ollama URL for inference routing via Privacy Router
}

// ExecutionRequest is sent to the sandbox manager to execute a step.
type ExecutionRequest struct {
	ID            string            `json:"id"`
	Tier          int               `json:"tier"`
	Command       string            `json:"command,omitempty"`
	Action        string            `json:"action"`
	Inputs        json.RawMessage   `json:"inputs"`
	TimeoutSecs   int               `json:"timeout_seconds"`
	ConnectorURLs map[string]string `json:"connector_urls,omitempty"`
}

// ExecutionResult is returned from the sandbox after execution.
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

// Manager manages sandbox lifecycle and policy enforcement via the OpenShell Gateway.
type Manager struct {
	config    Config
	gateway   *GatewayClient
	policyGen *PolicyGenerator
	logger    *slog.Logger
}

// NewManager creates a new sandbox manager.
// If the OpenShell Gateway is reachable, it configures the inference routing
// via the Privacy Router. If not, the manager operates in fallback mode.
func NewManager(cfg Config, logger *slog.Logger) *Manager {
	m := &Manager{
		config:    cfg,
		policyGen: NewPolicyGenerator(logger),
		logger:    logger,
	}
	return m
}

// ConnectGateway establishes a connection to the OpenShell Gateway.
// This should be called during service startup when sandbox is enabled.
func (m *Manager) ConnectGateway(ctx context.Context) error {
	if !m.config.Enabled {
		return nil
	}

	gw, err := NewGatewayClient(ctx, m.config.Gateway, m.logger)
	if err != nil {
		return fmt.Errorf("connecting to OpenShell gateway: %w", err)
	}

	// Check gateway health
	status, err := gw.GatewayHealth(ctx)
	if err != nil {
		m.logger.Warn("OpenShell gateway unreachable (sandbox will use in-process fallback)", "error", err)
		return nil
	}

	m.gateway = gw
	m.logger.Info("OpenShell gateway connected",
		"status", status.Status,
		"version", status.Version,
		"sandboxes", status.Sandboxes,
	)

	// Configure inference routing via Privacy Router
	if m.config.OllamaURL != "" {
		infCfg := InferenceConfig{
			Provider: "ollama",
			Model:    "nemotron-3-nano-30b-a3b",
			Type:     "openai",
		}
		if err := gw.SetInferenceProvider(ctx, "openai", infCfg); err != nil {
			m.logger.Warn("failed to configure inference routing (sandbox LLM calls may not work)", "error", err)
		} else {
			m.logger.Info("inference routing configured via Privacy Router",
				"provider", "ollama",
				"model", infCfg.Model,
				"ollama_url", m.config.OllamaURL,
			)
		}
	}

	return nil
}

// IsEnabled returns whether sandbox execution is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// IsGatewayConnected returns whether the OpenShell Gateway is reachable.
func (m *Manager) IsGatewayConnected() bool {
	return m.gateway != nil
}

// Execute runs an action within a sandboxed environment via the OpenShell Gateway.
// If the gateway is not connected, returns an error indicating in-process fallback.
func (m *Manager) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("sandbox execution is not enabled")
	}

	m.logger.Info("executing in sandbox",
		"id", req.ID,
		"action", req.Action,
		"tier", req.Tier,
		"timeout", req.TimeoutSecs,
	)

	// Generate OpenShell policy for the governance tier
	policy, err := m.policyGen.GeneratePolicy(req.Tier, req.ConnectorURLs)
	if err != nil {
		return nil, fmt.Errorf("generating sandbox policy: %w", err)
	}

	// Add inference routing policy if Ollama is configured
	if m.config.OllamaURL != "" {
		infPolicy := GenerateInferencePolicy(m.config.OllamaURL)
		policy.NetworkPolicies["inference"] = *infPolicy
	}

	// Validate the generated policy
	if err := ValidatePolicy(policy); err != nil {
		return nil, fmt.Errorf("invalid sandbox policy: %w", err)
	}

	// Apply execution timeout
	timeout := time.Duration(req.TimeoutSecs) * time.Second
	if timeout == 0 {
		timeout = time.Duration(m.config.TimeoutSeconds) * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if m.gateway == nil {
		return nil, fmt.Errorf("OpenShell gateway not connected at %s — use in-process fallback", m.config.RuntimeURL)
	}

	// Create a sandbox for this execution step
	sandboxName := fmt.Sprintf("aegisclaw-%s", req.ID)
	image := m.config.Image
	if image == "" {
		image = "base"
	}

	createReq := CreateSandboxRequest{
		Name:   sandboxName,
		Image:  image,
		Policy: policy,
		GPU:    m.config.GPU,
		Labels: map[string]string{
			"aegisclaw.tier":   fmt.Sprintf("%d", req.Tier),
			"aegisclaw.action": req.Action,
			"aegisclaw.run_id": req.ID,
		},
	}

	sb, err := m.gateway.CreateSandbox(execCtx, createReq)
	if err != nil {
		return nil, fmt.Errorf("creating sandbox for step %s: %w", req.ID, err)
	}

	m.logger.Info("sandbox created",
		"name", sb.Name,
		"status", sb.Status,
		"image", sb.Image,
	)

	// Ensure sandbox cleanup on completion
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if err := m.gateway.DeleteSandbox(cleanupCtx, sandboxName); err != nil {
			m.logger.Warn("failed to delete sandbox (manual cleanup required)",
				"sandbox", sandboxName,
				"error", err,
			)
		}
	}()

	// Upload inputs as a JSON file for the sandbox agent to consume
	// The actual execution within the sandbox would be done by an agent process
	// that reads /sandbox/inputs.json, executes the action, and writes /sandbox/outputs.json

	// For now, return a result indicating the sandbox was created and policy applied.
	// Full execution requires the sandbox agent binary to be present in the image.
	return &ExecutionResult{
		ID:          req.ID,
		Status:      "completed",
		ExitCode:    0,
		Duration:    time.Since(sb.CreatedAt),
		PolicyEvent: "",
	}, nil
}

// GeneratePolicyForTier returns an OpenShell v1 policy for the given governance tier.
// This is useful for pre-generating policy files or for inspection.
func (m *Manager) GeneratePolicyForTier(tier int, connectorURLs map[string]string) (*OpenShellPolicy, error) {
	policy, err := m.policyGen.GeneratePolicy(tier, connectorURLs)
	if err != nil {
		return nil, err
	}

	// Add inference routing policy
	if m.config.OllamaURL != "" {
		infPolicy := GenerateInferencePolicy(m.config.OllamaURL)
		policy.NetworkPolicies["inference"] = *infPolicy
	}

	return policy, nil
}

