package nats

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeEnvelope_ValidJSON(t *testing.T) {
	tests := []struct {
		name    string
		orgID   uuid.UUID
		msgType string
		payload RunTriggerMsg
	}{
		{
			name:    "scheduler trigger",
			orgID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			msgType: SubjectRunTrigger,
			payload: RunTriggerMsg{
				EngagementID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				OrgID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
				TriggeredBy:  "scheduler",
			},
		},
		{
			name:    "user trigger",
			orgID:   uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			msgType: SubjectRunTrigger,
			payload: RunTriggerMsg{
				EngagementID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
				OrgID:        uuid.MustParse("33333333-3333-3333-3333-333333333333"),
				TriggeredBy:  "user-abc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := NewEnvelope(tt.orgID, tt.msgType, tt.payload)
			data, err := json.Marshal(env)
			require.NoError(t, err)

			decoded, err := DecodeEnvelope[RunTriggerMsg](data)

			require.NoError(t, err)
			require.NotNil(t, decoded)
			assert.Equal(t, env.TraceID, decoded.TraceID)
			assert.Equal(t, tt.orgID, decoded.OrgID)
			assert.Equal(t, tt.msgType, decoded.Type)
			assert.Equal(t, tt.payload.EngagementID, decoded.Payload.EngagementID)
			assert.Equal(t, tt.payload.OrgID, decoded.Payload.OrgID)
			assert.Equal(t, tt.payload.TriggeredBy, decoded.Payload.TriggeredBy)
			assert.False(t, decoded.Timestamp.IsZero())
		})
	}
}

func TestDecodeEnvelope_InvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty bytes",
			data: []byte{},
		},
		{
			name: "garbage data",
			data: []byte("not-json-at-all"),
		},
		{
			name: "truncated JSON",
			data: []byte(`{"trace_id": "abc", "org_id": "`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := DecodeEnvelope[RunTriggerMsg](tt.data)

			require.Error(t, err)
			assert.Nil(t, decoded)
			assert.Contains(t, err.Error(), "decoding envelope")
		})
	}
}

func TestDecodeEnvelope_KillSwitchPayload(t *testing.T) {
	orgID := uuid.New()
	payload := KillSwitchMsg{
		Engaged: true,
		Reason:  "emergency",
		ActorID: "admin-42",
	}

	env := NewEnvelope(orgID, SubjectKillSwitch, payload)
	data, err := json.Marshal(env)
	require.NoError(t, err)

	decoded, err := DecodeEnvelope[KillSwitchMsg](data)

	require.NoError(t, err)
	require.NotNil(t, decoded)
	assert.True(t, decoded.Payload.Engaged)
	assert.Equal(t, "emergency", decoded.Payload.Reason)
	assert.Equal(t, "admin-42", decoded.Payload.ActorID)
}
