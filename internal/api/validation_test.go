package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAssetType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid endpoint", "endpoint", false},
		{"valid server", "server", false},
		{"valid application", "application", false},
		{"valid identity", "identity", false},
		{"valid cloud_account", "cloud_account", false},
		{"valid k8s_cluster", "k8s_cluster", false},
		{"invalid empty string", "", true},
		{"invalid unknown type", "database", true},
		{"invalid capitalized", "Endpoint", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAssetType(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid asset type")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSeverity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid critical", "critical", false},
		{"valid high", "high", false},
		{"valid medium", "medium", false},
		{"valid low", "low", false},
		{"valid informational", "informational", false},
		{"invalid empty", "", true},
		{"invalid unknown", "urgent", true},
		{"invalid capitalized", "Critical", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSeverity(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid severity")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFindingStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid observed", "observed", false},
		{"valid needs_review", "needs_review", false},
		{"valid confirmed", "confirmed", false},
		{"valid ticketed", "ticketed", false},
		{"valid fixed", "fixed", false},
		{"valid retested", "retested", false},
		{"valid closed", "closed", false},
		{"valid accepted_risk", "accepted_risk", false},
		{"invalid empty", "", true},
		{"invalid unknown", "open", true},
		{"invalid capitalized", "Confirmed", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFindingStatus(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid finding status")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRunStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid queued", "queued", false},
		{"valid running", "running", false},
		{"valid paused", "paused", false},
		{"valid completed", "completed", false},
		{"valid failed", "failed", false},
		{"valid cancelled", "cancelled", false},
		{"valid killed", "killed", false},
		{"invalid empty", "", true},
		{"invalid unknown", "pending", true},
		{"invalid capitalized", "Queued", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRunStatus(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid run status")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequired(t *testing.T) {
	t.Run("all fields present", func(t *testing.T) {
		err := validateRequired(map[string]string{
			"name":  "test",
			"email": "user@example.com",
		})
		assert.NoError(t, err)
	})

	t.Run("missing single field", func(t *testing.T) {
		err := validateRequired(map[string]string{
			"name":  "",
			"email": "user@example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required fields missing")
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("missing field with whitespace only", func(t *testing.T) {
		err := validateRequired(map[string]string{
			"name":  "   ",
			"email": "user@example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("multiple fields missing", func(t *testing.T) {
		err := validateRequired(map[string]string{
			"name":  "",
			"email": "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required fields missing")
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		err := validateRequired(map[string]string{})
		assert.NoError(t, err)
	})
}
