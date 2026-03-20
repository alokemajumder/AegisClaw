package sandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"enabled", true},
		{"disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(Config{Enabled: tt.enabled}, newTestLogger())

			assert.Equal(t, tt.enabled, m.IsEnabled())
		})
	}
}

func TestManager_Disabled_Execute(t *testing.T) {
	m := NewManager(Config{Enabled: false}, newTestLogger())

	result, err := m.Execute(t.Context(), ExecutionRequest{
		ID:     "test-step-1",
		Tier:   0,
		Action: "query-siem",
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not enabled")
}

func TestManager_GeneratePolicyForTier(t *testing.T) {
	connectors := map[string]string{
		"sentinel": "https://sentinel.contoso.com",
	}

	tests := []struct {
		name string
		tier int
	}{
		{"tier 0", 0},
		{"tier 1", 1},
		{"tier 2", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(Config{
				Enabled:   true,
				OllamaURL: "http://localhost:11434",
			}, newTestLogger())

			policy, err := m.GeneratePolicyForTier(tt.tier, connectors)

			require.NoError(t, err)
			require.NotNil(t, policy)
			assert.Equal(t, 1, policy.Version)

			// Inference policy should be injected when OllamaURL is set.
			assert.Contains(t, policy.NetworkPolicies, "inference")

			// Policy should pass validation.
			assert.NoError(t, ValidatePolicy(policy))
		})
	}
}

func TestManager_GeneratePolicyForTier3(t *testing.T) {
	m := NewManager(Config{Enabled: true}, newTestLogger())

	policy, err := m.GeneratePolicyForTier(3, nil)

	require.Error(t, err)
	assert.Nil(t, policy)
	assert.Contains(t, err.Error(), "prohibited")
}
