package connectorsdk

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnector is a minimal Connector implementation used for testing.
type mockConnector struct {
	connType string
}

func (m *mockConnector) Type() string                                         { return m.connType }
func (m *mockConnector) Category() Category                                   { return CategorySIEM }
func (m *mockConnector) Capabilities() []Capability                           { return []Capability{CapQueryEvents} }
func (m *mockConnector) Version() string                                      { return "0.1.0" }
func (m *mockConnector) Init(_ context.Context, _ ConnectorConfig) error      { return nil }
func (m *mockConnector) Close() error                                         { return nil }
func (m *mockConnector) HealthCheck(_ context.Context) (*HealthStatus, error) { return nil, nil }
func (m *mockConnector) ValidateCredentials(_ context.Context) error          { return nil }
func (m *mockConnector) ConfigSchema() json.RawMessage                        { return json.RawMessage(`{}`) }

func newMockFactory(connType string) Factory {
	return func() Connector {
		return &mockConnector{connType: connType}
	}
}

func TestRegister_NewType(t *testing.T) {
	r := NewRegistry()

	err := r.Register("sentinel", newMockFactory("sentinel"))

	require.NoError(t, err)
	assert.True(t, r.Has("sentinel"))
}

func TestRegister_DuplicateType(t *testing.T) {
	r := NewRegistry()
	err := r.Register("sentinel", newMockFactory("sentinel"))
	require.NoError(t, err)

	err = r.Register("sentinel", newMockFactory("sentinel"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestCreate_RegisteredType(t *testing.T) {
	r := NewRegistry()
	err := r.Register("defender", newMockFactory("defender"))
	require.NoError(t, err)

	conn, err := r.Create("defender")

	require.NoError(t, err)
	require.NotNil(t, conn)
	assert.Equal(t, "defender", conn.Type())
}

func TestCreate_UnknownType(t *testing.T) {
	r := NewRegistry()

	conn, err := r.Create("nonexistent")

	require.Error(t, err)
	assert.Nil(t, conn)
	assert.Contains(t, err.Error(), "not registered")
}

func TestListTypes(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register("sentinel", newMockFactory("sentinel")))
	require.NoError(t, r.Register("defender", newMockFactory("defender")))
	require.NoError(t, r.Register("entra", newMockFactory("entra")))

	types := r.ListTypes()

	sort.Strings(types)
	assert.Equal(t, []string{"defender", "entra", "sentinel"}, types)
}

func TestListTypes_EmptyRegistry(t *testing.T) {
	r := NewRegistry()

	types := r.ListTypes()

	assert.Empty(t, types)
}

func TestHas(t *testing.T) {
	tests := []struct {
		name          string
		registered    []string
		lookup        string
		expectPresent bool
	}{
		{
			name:          "registered type returns true",
			registered:    []string{"sentinel", "defender"},
			lookup:        "sentinel",
			expectPresent: true,
		},
		{
			name:          "unregistered type returns false",
			registered:    []string{"sentinel"},
			lookup:        "unknown",
			expectPresent: false,
		},
		{
			name:          "empty registry returns false",
			registered:    nil,
			lookup:        "sentinel",
			expectPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			for _, ct := range tt.registered {
				require.NoError(t, r.Register(ct, newMockFactory(ct)))
			}

			got := r.Has(tt.lookup)

			assert.Equal(t, tt.expectPresent, got)
		})
	}
}
