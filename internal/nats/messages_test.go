package nats

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEnvelope_FieldsSetCorrectly(t *testing.T) {
	orgID := uuid.New()
	payload := RunTriggerMsg{
		EngagementID: uuid.New(),
		OrgID:        orgID,
		TriggeredBy:  "scheduler",
	}

	env := NewEnvelope(orgID, SubjectRunTrigger, payload)

	require.NotNil(t, env)
	assert.Equal(t, orgID, env.OrgID)
	assert.Equal(t, SubjectRunTrigger, env.Type)
	assert.Equal(t, payload, env.Payload)
}

func TestNewEnvelope_GeneratesNonEmptyTraceID(t *testing.T) {
	orgID := uuid.New()
	payload := KillSwitchMsg{Engaged: true, Reason: "test", ActorID: "admin"}

	env := NewEnvelope(orgID, SubjectKillSwitch, payload)

	require.NotNil(t, env)
	assert.NotEmpty(t, env.TraceID)

	// TraceID must be a valid UUID string.
	_, err := uuid.Parse(env.TraceID)
	assert.NoError(t, err, "TraceID should be a valid UUID")
}

func TestNewEnvelope_SetsTimestamp(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	orgID := uuid.New()
	payload := RunStatusMsg{
		RunID:        uuid.New(),
		EngagementID: uuid.New(),
		Status:       "running",
	}

	env := NewEnvelope(orgID, SubjectRunStatus, payload)
	after := time.Now().UTC().Add(time.Second)

	require.NotNil(t, env)
	assert.False(t, env.Timestamp.IsZero(), "Timestamp should not be zero")
	assert.True(t, env.Timestamp.After(before), "Timestamp should be after the test start time")
	assert.True(t, env.Timestamp.Before(after), "Timestamp should be before the test end time")
}

func TestNewEnvelope_UniqueTraceIDs(t *testing.T) {
	orgID := uuid.New()
	payload := KillSwitchMsg{Engaged: false, Reason: "clear", ActorID: "admin"}

	env1 := NewEnvelope(orgID, SubjectKillSwitch, payload)
	env2 := NewEnvelope(orgID, SubjectKillSwitch, payload)

	assert.NotEqual(t, env1.TraceID, env2.TraceID, "each envelope should receive a unique TraceID")
}
