package sandbox

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------------------------------------------------------------------
// GeneratePolicy tests
// ---------------------------------------------------------------------------

func TestGeneratePolicy_Tier0(t *testing.T) {
	gen := NewPolicyGenerator(newTestLogger())
	connectors := map[string]string{"sentinel": "https://sentinel.contoso.com"}

	policy, err := gen.GeneratePolicy(0, connectors)

	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.Equal(t, 1, policy.Version)

	// Filesystem: read-only, no read-write paths, workdir excluded.
	assert.False(t, policy.FilesystemPolicy.IncludeWorkdir)
	assert.NotEmpty(t, policy.FilesystemPolicy.ReadOnly)
	assert.Empty(t, policy.FilesystemPolicy.ReadWrite)

	// Landlock: hard requirement.
	assert.Equal(t, "hard_requirement", policy.Landlock.Compatibility)

	// Process: sandbox user.
	assert.Equal(t, "sandbox", policy.Process.RunAsUser)

	// Network: connector endpoints only, port 443.
	require.Contains(t, policy.NetworkPolicies, "connector-sentinel")
	np := policy.NetworkPolicies["connector-sentinel"]
	require.Len(t, np.Endpoints, 1)
	assert.Equal(t, 443, np.Endpoints[0].Port)
	assert.Equal(t, "rest", np.Endpoints[0].Protocol)
	assert.Equal(t, "read-only", np.Endpoints[0].Access)

	// Binaries: curl only.
	assert.Len(t, np.Binaries, 1)
	assert.Equal(t, "/usr/bin/curl", np.Binaries[0].Path)
}

func TestGeneratePolicy_Tier1(t *testing.T) {
	gen := NewPolicyGenerator(newTestLogger())
	connectors := map[string]string{"sentinel": "https://sentinel.contoso.com"}

	policy, err := gen.GeneratePolicy(1, connectors)

	require.NoError(t, err)
	require.NotNil(t, policy)

	// Filesystem: /sandbox + /tmp writable.
	assert.True(t, policy.FilesystemPolicy.IncludeWorkdir)
	assert.Contains(t, policy.FilesystemPolicy.ReadWrite, "/sandbox")
	assert.Contains(t, policy.FilesystemPolicy.ReadWrite, "/tmp")

	// Network: DNS policy present.
	require.Contains(t, policy.NetworkPolicies, "dns")
	dns := policy.NetworkPolicies["dns"]
	require.Len(t, dns.Endpoints, 1)
	assert.Equal(t, 53, dns.Endpoints[0].Port)

	// Connector policy present.
	require.Contains(t, policy.NetworkPolicies, "connector-sentinel")
	np := policy.NetworkPolicies["connector-sentinel"]
	assert.Equal(t, "read-only", np.Endpoints[0].Access)

	// Binaries: curl + python3.
	binPaths := binaryPaths(np.Binaries)
	assert.Contains(t, binPaths, "/usr/bin/curl")
	assert.Contains(t, binPaths, "/usr/bin/python3")
}

func TestGeneratePolicy_Tier2(t *testing.T) {
	gen := NewPolicyGenerator(newTestLogger())
	connectors := map[string]string{"sentinel": "https://sentinel.contoso.com"}

	policy, err := gen.GeneratePolicy(2, connectors)

	require.NoError(t, err)
	require.NotNil(t, policy)

	// Connector access is read-write.
	require.Contains(t, policy.NetworkPolicies, "connector-sentinel")
	np := policy.NetworkPolicies["connector-sentinel"]
	assert.Equal(t, "read-write", np.Endpoints[0].Access)

	// Binaries: curl + python3 + bash.
	binPaths := binaryPaths(np.Binaries)
	assert.Contains(t, binPaths, "/usr/bin/curl")
	assert.Contains(t, binPaths, "/usr/bin/python3")
	assert.Contains(t, binPaths, "/usr/bin/bash")
}

func TestGeneratePolicy_Tier3(t *testing.T) {
	gen := NewPolicyGenerator(newTestLogger())

	policy, err := gen.GeneratePolicy(3, nil)

	require.Error(t, err)
	assert.Nil(t, policy)
	assert.Contains(t, err.Error(), "prohibited")
}

func TestGeneratePolicy_WithConnectorURLs(t *testing.T) {
	gen := NewPolicyGenerator(newTestLogger())
	connectors := map[string]string{
		"sentinel": "https://sentinel.contoso.com",
		"defender": "https://api.securitycenter.microsoft.com",
	}

	policy, err := gen.GeneratePolicy(1, connectors)

	require.NoError(t, err)
	require.NotNil(t, policy)

	// Both connector policies should exist.
	require.Contains(t, policy.NetworkPolicies, "connector-sentinel")
	require.Contains(t, policy.NetworkPolicies, "connector-defender")

	// Verify extracted hosts appear in endpoints.
	sentinelEP := policy.NetworkPolicies["connector-sentinel"].Endpoints[0]
	assert.Equal(t, "sentinel.contoso.com", sentinelEP.Host)

	defenderEP := policy.NetworkPolicies["connector-defender"].Endpoints[0]
	assert.Equal(t, "api.securitycenter.microsoft.com", defenderEP.Host)
}

// ---------------------------------------------------------------------------
// ValidatePolicy tests
// ---------------------------------------------------------------------------

func TestValidatePolicy_Valid(t *testing.T) {
	gen := NewPolicyGenerator(newTestLogger())
	policy, err := gen.GeneratePolicy(0, map[string]string{"s": "https://s.example.com"})
	require.NoError(t, err)

	err = ValidatePolicy(policy)

	assert.NoError(t, err)
}

func TestValidatePolicy_PathTraversal(t *testing.T) {
	policy := &OpenShellPolicy{
		Version: 1,
		FilesystemPolicy: FilesystemPolicy{
			ReadOnly: []string{"/usr", "/lib/../etc"},
		},
		Process: ProcessPolicy{RunAsUser: "sandbox", RunAsGroup: "sandbox"},
	}

	err := ValidatePolicy(policy)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "..")
}

func TestValidatePolicy_RootUser(t *testing.T) {
	tests := []struct {
		name  string
		user  string
		group string
	}{
		{"root user", "root", "sandbox"},
		{"uid 0", "0", "sandbox"},
		{"root group", "sandbox", "root"},
		{"gid 0", "sandbox", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &OpenShellPolicy{
				Version: 1,
				Process: ProcessPolicy{RunAsUser: tt.user, RunAsGroup: tt.group},
			}

			err := ValidatePolicy(policy)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "root")
		})
	}
}

func TestValidatePolicy_TooManyPaths(t *testing.T) {
	paths := make([]string, maxPaths+1)
	for i := range paths {
		paths[i] = fmt.Sprintf("/path/%d", i)
	}

	policy := &OpenShellPolicy{
		Version: 1,
		FilesystemPolicy: FilesystemPolicy{
			ReadOnly: paths,
		},
		Process: ProcessPolicy{RunAsUser: "sandbox", RunAsGroup: "sandbox"},
	}

	err := ValidatePolicy(policy)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceed maximum")
}

// ---------------------------------------------------------------------------
// Inference policy
// ---------------------------------------------------------------------------

func TestGenerateInferencePolicy(t *testing.T) {
	np := GenerateInferencePolicy("http://localhost:11434")

	require.NotNil(t, np)
	assert.Equal(t, "inference-ollama", np.Name)
	require.Len(t, np.Endpoints, 1)
	assert.Equal(t, "host.openshell.internal", np.Endpoints[0].Host)
	assert.Equal(t, 11434, np.Endpoints[0].Port)
	assert.Equal(t, "full", np.Endpoints[0].Access)
}

func TestGenerateInferencePolicy_CustomPort(t *testing.T) {
	np := GenerateInferencePolicy("http://localhost:8888")

	require.NotNil(t, np)
	assert.Equal(t, 8888, np.Endpoints[0].Port)
}

// ---------------------------------------------------------------------------
// MarshalPolicyYAML (actually JSON)
// ---------------------------------------------------------------------------

func TestMarshalPolicyYAML(t *testing.T) {
	gen := NewPolicyGenerator(newTestLogger())
	policy, err := gen.GeneratePolicy(0, map[string]string{"s": "https://s.example.com"})
	require.NoError(t, err)

	data, err := MarshalPolicyYAML(policy)

	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Must be valid JSON.
	assert.True(t, json.Valid(data), "output should be valid JSON")

	// Round-trip: unmarshal back into an OpenShellPolicy.
	var roundTripped OpenShellPolicy
	err = json.Unmarshal(data, &roundTripped)
	require.NoError(t, err)
	assert.Equal(t, 1, roundTripped.Version)
}

func TestMarshalPolicyYAML_Nil(t *testing.T) {
	_, err := MarshalPolicyYAML(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func binaryPaths(rules []BinaryRule) []string {
	paths := make([]string, len(rules))
	for i, r := range rules {
		paths[i] = r.Path
	}
	return paths
}

// Suppress unused import warning for strings package.
var _ = strings.Contains
