package sentinel

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentinelConnector_Type(t *testing.T) {
	c := New()
	assert.Equal(t, "sentinel", c.Type())
}

func TestSentinelConnector_Category(t *testing.T) {
	c := New()
	assert.Equal(t, connectorsdk.CategorySIEM, c.Category())
}

func TestSentinelConnector_ConfigSchema(t *testing.T) {
	c := New()
	schema := c.ConfigSchema()

	require.NotNil(t, schema)
	assert.True(t, json.Valid([]byte(schema)), "ConfigSchema must return valid JSON")

	// Verify it declares an object with expected properties.
	var parsed map[string]interface{}
	err := json.Unmarshal(schema, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "object", parsed["type"])

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")
	assert.Contains(t, props, "workspace_id")
	assert.Contains(t, props, "tenant_id")
	assert.Contains(t, props, "client_id")
	assert.Contains(t, props, "client_secret")

	// Required fields.
	required, ok := parsed["required"].([]interface{})
	require.True(t, ok, "schema should have required array")
	requiredStrs := make([]string, len(required))
	for i, r := range required {
		requiredStrs[i] = r.(string)
	}
	assert.Contains(t, requiredStrs, "workspace_id")
	assert.Contains(t, requiredStrs, "tenant_id")
	assert.Contains(t, requiredStrs, "client_id")
	assert.Contains(t, requiredStrs, "client_secret")
}

func TestSentinelConnector_Initialize_MissingConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]string
		errSubstr string
	}{
		{
			name:      "empty config",
			config:    map[string]string{},
			errSubstr: "workspace_id",
		},
		{
			name: "missing tenant_id",
			config: map[string]string{
				"workspace_id":  "ws-123",
				"client_id":     "cid",
				"client_secret": "csecret",
			},
			errSubstr: "tenant_id",
		},
		{
			name: "missing client_id",
			config: map[string]string{
				"workspace_id":  "ws-123",
				"tenant_id":     "tid-456",
				"client_secret": "csecret",
			},
			errSubstr: "client_id",
		},
		{
			name: "missing client_secret",
			config: map[string]string{
				"workspace_id": "ws-123",
				"tenant_id":    "tid-456",
				"client_id":    "cid",
			},
			errSubstr: "client_secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			cfgJSON, err := json.Marshal(tt.config)
			require.NoError(t, err)

			err = c.Init(context.Background(), connectorsdk.ConnectorConfig{
				Config: cfgJSON,
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errSubstr)
		})
	}
}

func TestSentinelConnector_Initialize_Valid(t *testing.T) {
	c := New()
	cfg := map[string]string{
		"workspace_id":  "ws-123",
		"tenant_id":     "tid-456",
		"client_id":     "cid-789",
		"client_secret": "secret-abc",
	}
	cfgJSON, err := json.Marshal(cfg)
	require.NoError(t, err)

	err = c.Init(context.Background(), connectorsdk.ConnectorConfig{
		Config: cfgJSON,
	})

	assert.NoError(t, err)
}
