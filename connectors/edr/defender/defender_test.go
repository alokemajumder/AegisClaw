package defender

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefenderConnector_Type(t *testing.T) {
	c := New()
	assert.Equal(t, "defender", c.Type())
}

func TestDefenderConnector_Category(t *testing.T) {
	c := New()
	assert.Equal(t, connectorsdk.CategoryEDR, c.Category())
}

func TestDefenderConnector_ConfigSchema(t *testing.T) {
	c := New()
	schema := c.ConfigSchema()

	require.NotNil(t, schema)
	assert.True(t, json.Valid([]byte(schema)), "ConfigSchema must return valid JSON")

	var parsed map[string]interface{}
	err := json.Unmarshal(schema, &parsed)
	require.NoError(t, err)
	assert.Equal(t, "object", parsed["type"])

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")
	assert.Contains(t, props, "tenant_id")
	assert.Contains(t, props, "client_id")
	assert.Contains(t, props, "client_secret")
	assert.Contains(t, props, "api_url")

	// Required fields.
	required, ok := parsed["required"].([]interface{})
	require.True(t, ok, "schema should have required array")
	requiredStrs := make([]string, len(required))
	for i, r := range required {
		requiredStrs[i] = r.(string)
	}
	assert.Contains(t, requiredStrs, "tenant_id")
	assert.Contains(t, requiredStrs, "client_id")
	assert.Contains(t, requiredStrs, "client_secret")
}

func TestDefenderConnector_Initialize_MissingConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]string
		errSubstr string
	}{
		{
			name:      "empty config",
			config:    map[string]string{},
			errSubstr: "tenant_id",
		},
		{
			name: "missing client_id",
			config: map[string]string{
				"tenant_id":     "tid-123",
				"client_secret": "csecret",
			},
			errSubstr: "client_id",
		},
		{
			name: "missing client_secret",
			config: map[string]string{
				"tenant_id": "tid-123",
				"client_id": "cid-456",
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

func TestDefenderConnector_Initialize_Valid(t *testing.T) {
	c := New()
	cfg := map[string]string{
		"tenant_id":     "tid-123",
		"client_id":     "cid-456",
		"client_secret": "secret-789",
	}
	cfgJSON, err := json.Marshal(cfg)
	require.NoError(t, err)

	err = c.Init(context.Background(), connectorsdk.ConnectorConfig{
		Config: cfgJSON,
	})

	assert.NoError(t, err)
}

func TestDefenderConnector_Initialize_DefaultAPIURL(t *testing.T) {
	c := New()
	cfg := map[string]string{
		"tenant_id":     "tid-123",
		"client_id":     "cid-456",
		"client_secret": "secret-789",
	}
	cfgJSON, err := json.Marshal(cfg)
	require.NoError(t, err)

	err = c.Init(context.Background(), connectorsdk.ConnectorConfig{
		Config: cfgJSON,
	})

	require.NoError(t, err)
	assert.Equal(t, "https://api.securitycenter.microsoft.com", c.cfg.APIURL)
}
