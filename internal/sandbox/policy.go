package sandbox

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// PolicyGenerator
// ---------------------------------------------------------------------------

// PolicyGenerator produces OpenShell v1 policy structures from AegisClaw
// governance tiers. Each tier maps to progressively wider filesystem and
// network permissions; Tier 3 is always rejected.
type PolicyGenerator struct {
	logger *slog.Logger
}

// NewPolicyGenerator creates a PolicyGenerator with the given logger.
func NewPolicyGenerator(logger *slog.Logger) *PolicyGenerator {
	return &PolicyGenerator{logger: logger}
}

// GeneratePolicy maps an AegisClaw governance tier (0-2) to a fully populated
// OpenShell v1 policy. connectorURLs is a map of connector-name to base URL
// (e.g. {"sentinel": "sentinel.contoso.com"}).
//
// Tier 3 always returns an error — those actions are prohibited and must never
// reach the sandbox.
func (g *PolicyGenerator) GeneratePolicy(tier int, connectorURLs map[string]string) (*OpenShellPolicy, error) {
	switch tier {
	case 0:
		return g.passivePolicy(connectorURLs), nil
	case 1:
		return g.benignPolicy(connectorURLs), nil
	case 2:
		return g.sensitivePolicy(connectorURLs), nil
	case 3:
		return nil, fmt.Errorf("generate policy: tier 3 actions are prohibited and must not be sandboxed")
	default:
		return nil, fmt.Errorf("generate policy: unknown governance tier %d", tier)
	}
}

// passivePolicy builds a Tier 0 policy: read-only filesystem, connector
// endpoints only (read-only, REST, TLS terminate), hard Landlock, curl only.
func (g *PolicyGenerator) passivePolicy(connectorURLs map[string]string) *OpenShellPolicy {
	g.logger.Debug("generating tier 0 (passive) OpenShell policy", "connectors", len(connectorURLs))

	policies := make(map[string]NetworkPolicy)

	// One network policy per connector — read-only, REST protocol, TLS terminate.
	for name, rawURL := range connectorURLs {
		host := extractHost(rawURL)
		policyName := fmt.Sprintf("connector-%s", name)
		policies[policyName] = NetworkPolicy{
			Name: policyName,
			Endpoints: []NetworkEndpoint{
				{
					Host:        host,
					Port:        443,
					Protocol:    "rest",
					TLS:         "terminate",
					Enforcement: "enforce",
					Access:      "read-only",
				},
			},
			Binaries: []BinaryRule{
				{Path: "/usr/bin/curl"},
			},
		}
	}

	return &OpenShellPolicy{
		Version: 1,
		FilesystemPolicy: FilesystemPolicy{
			IncludeWorkdir: false,
			ReadOnly:       []string{"/usr", "/lib", "/etc", "/sandbox"},
		},
		Landlock:        LandlockPolicy{Compatibility: "hard_requirement"},
		Process:         ProcessPolicy{RunAsUser: "sandbox", RunAsGroup: "sandbox"},
		NetworkPolicies: policies,
	}
}

// benignPolicy builds a Tier 1 policy: read-only system paths, read-write
// /sandbox + /tmp, connector endpoints + DNS, curl + python3.
func (g *PolicyGenerator) benignPolicy(connectorURLs map[string]string) *OpenShellPolicy {
	g.logger.Debug("generating tier 1 (benign) OpenShell policy", "connectors", len(connectorURLs))

	policies := make(map[string]NetworkPolicy)

	// DNS resolution policy.
	policies["dns"] = NetworkPolicy{
		Name: "dns-resolution",
		Endpoints: []NetworkEndpoint{
			{
				Host:        "0.0.0.0/0",
				Port:        53,
				Protocol:    "udp",
				TLS:         "passthrough",
				Enforcement: "enforce",
				Access:      "full",
			},
		},
		Binaries: []BinaryRule{
			{Path: "/usr/bin/curl"},
			{Path: "/usr/bin/python3"},
		},
	}

	// Connector policies — read-only access.
	for name, rawURL := range connectorURLs {
		host := extractHost(rawURL)
		policyName := fmt.Sprintf("connector-%s", name)
		policies[policyName] = NetworkPolicy{
			Name: policyName,
			Endpoints: []NetworkEndpoint{
				{
					Host:        host,
					Port:        443,
					Protocol:    "rest",
					TLS:         "terminate",
					Enforcement: "enforce",
					Access:      "read-only",
				},
			},
			Binaries: []BinaryRule{
				{Path: "/usr/bin/curl"},
				{Path: "/usr/bin/python3"},
			},
		}
	}

	return &OpenShellPolicy{
		Version: 1,
		FilesystemPolicy: FilesystemPolicy{
			IncludeWorkdir: true,
			ReadOnly:       []string{"/usr", "/lib", "/etc"},
			ReadWrite:      []string{"/sandbox", "/tmp"},
		},
		Landlock:        LandlockPolicy{Compatibility: "hard_requirement"},
		Process:         ProcessPolicy{RunAsUser: "sandbox", RunAsGroup: "sandbox"},
		NetworkPolicies: policies,
	}
}

// sensitivePolicy builds a Tier 2 policy: read-only system paths, read-write
// /sandbox + /tmp, connector endpoints with read-write access + DNS,
// enforcement=enforce, curl + python3 + bash.
func (g *PolicyGenerator) sensitivePolicy(connectorURLs map[string]string) *OpenShellPolicy {
	g.logger.Debug("generating tier 2 (sensitive) OpenShell policy", "connectors", len(connectorURLs))

	policies := make(map[string]NetworkPolicy)

	// DNS resolution policy.
	policies["dns"] = NetworkPolicy{
		Name: "dns-resolution",
		Endpoints: []NetworkEndpoint{
			{
				Host:        "0.0.0.0/0",
				Port:        53,
				Protocol:    "udp",
				TLS:         "passthrough",
				Enforcement: "enforce",
				Access:      "full",
			},
		},
		Binaries: []BinaryRule{
			{Path: "/usr/bin/curl"},
			{Path: "/usr/bin/python3"},
			{Path: "/usr/bin/bash"},
		},
	}

	// Connector policies — read-write access, enforced.
	for name, rawURL := range connectorURLs {
		host := extractHost(rawURL)
		policyName := fmt.Sprintf("connector-%s", name)
		policies[policyName] = NetworkPolicy{
			Name: policyName,
			Endpoints: []NetworkEndpoint{
				{
					Host:        host,
					Port:        443,
					Protocol:    "rest",
					TLS:         "terminate",
					Enforcement: "enforce",
					Access:      "read-write",
				},
			},
			Binaries: []BinaryRule{
				{Path: "/usr/bin/curl"},
				{Path: "/usr/bin/python3"},
				{Path: "/usr/bin/bash"},
			},
		}
	}

	return &OpenShellPolicy{
		Version: 1,
		FilesystemPolicy: FilesystemPolicy{
			IncludeWorkdir: true,
			ReadOnly:       []string{"/usr", "/lib", "/etc"},
			ReadWrite:      []string{"/sandbox", "/tmp"},
		},
		Landlock:        LandlockPolicy{Compatibility: "hard_requirement"},
		Process:         ProcessPolicy{RunAsUser: "sandbox", RunAsGroup: "sandbox"},
		NetworkPolicies: policies,
	}
}

// ---------------------------------------------------------------------------
// Serialisation
// ---------------------------------------------------------------------------

// MarshalPolicyYAML serialises an OpenShellPolicy to JSON bytes.
//
// NOTE: For actual .yaml file output, replace encoding/json with gopkg.in/yaml.v3
// and switch the struct tags accordingly. JSON is used here because the types
// carry json tags and yaml.v3 is not yet in the dependency tree.
func MarshalPolicyYAML(policy *OpenShellPolicy) ([]byte, error) {
	if policy == nil {
		return nil, fmt.Errorf("marshal policy: policy is nil")
	}
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal policy: %w", err)
	}
	return data, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// maxPaths is the OpenShell limit on combined filesystem paths.
const maxPaths = 256

// maxPathLen is the OpenShell per-path character limit.
const maxPathLen = 4096

// ValidatePolicy checks that an OpenShellPolicy conforms to OpenShell v1
// constraints: version must be 1, all paths must be absolute with no ".."
// components, combined path count must not exceed 256, and the process must
// not run as root.
func ValidatePolicy(policy *OpenShellPolicy) error {
	if policy == nil {
		return fmt.Errorf("validate policy: policy is nil")
	}

	// Version must be 1.
	if policy.Version != 1 {
		return fmt.Errorf("validate policy: unsupported version %d, expected 1", policy.Version)
	}

	// Process must not run as root.
	if policy.Process.RunAsUser == "root" || policy.Process.RunAsUser == "0" {
		return fmt.Errorf("validate policy: run_as_user cannot be root or 0")
	}
	if policy.Process.RunAsGroup == "root" || policy.Process.RunAsGroup == "0" {
		return fmt.Errorf("validate policy: run_as_group cannot be root or 0")
	}

	// Collect and validate all filesystem paths.
	var allPaths []string
	allPaths = append(allPaths, policy.FilesystemPolicy.ReadOnly...)
	allPaths = append(allPaths, policy.FilesystemPolicy.ReadWrite...)

	// Also count binary paths from network policies.
	for _, np := range policy.NetworkPolicies {
		for _, b := range np.Binaries {
			allPaths = append(allPaths, b.Path)
		}
	}

	if len(allPaths) > maxPaths {
		return fmt.Errorf("validate policy: %d paths exceed maximum of %d", len(allPaths), maxPaths)
	}

	for _, p := range allPaths {
		if err := validatePath(p); err != nil {
			return fmt.Errorf("validate policy: %w", err)
		}
	}

	return nil
}

// validatePath checks that a single path is absolute, contains no ".."
// traversal, and does not exceed the maximum length.
func validatePath(p string) error {
	if len(p) > maxPathLen {
		return fmt.Errorf("path %q exceeds max length %d", p, maxPathLen)
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("path %q is not absolute", p)
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("path %q contains '..' traversal", p)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Inference policy
// ---------------------------------------------------------------------------

// GenerateInferencePolicy creates a NetworkPolicy that allows sandbox
// processes to reach the Ollama LLM inference endpoint via the OpenShell
// privacy router (host.openshell.internal). This enables agents inside the
// sandbox to call LLM APIs without direct network access to the host.
func GenerateInferencePolicy(ollamaURL string) *NetworkPolicy {
	port := 11434 // Default Ollama port.
	if u, err := url.Parse(ollamaURL); err == nil && u.Port() != "" {
		fmt.Sscanf(u.Port(), "%d", &port)
	}

	return &NetworkPolicy{
		Name: "inference-ollama",
		Endpoints: []NetworkEndpoint{
			{
				Host:        "host.openshell.internal",
				Port:        port,
				Protocol:    "rest",
				TLS:         "terminate",
				Enforcement: "enforce",
				Access:      "full",
			},
		},
		Binaries: []BinaryRule{
			{Path: "/usr/bin/curl"},
			{Path: "/usr/bin/python3"},
		},
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractHost pulls the hostname from a URL string. If the string is not a
// valid URL it is returned as-is (it may already be a bare hostname).
func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	host := u.Hostname()
	if host == "" {
		return rawURL
	}
	return host
}
