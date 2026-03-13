package nats

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Subject constants for NATS messaging.
const (
	SubjectRunTrigger = "runs.trigger"
	SubjectRunStatus  = "runs.status"
	SubjectRunKill    = "runs.kill"

	SubjectAgentTask   = "agents.task"
	SubjectAgentResult = "agents.result"

	SubjectEvidenceStore = "evidence.store"

	SubjectConnectorExec   = "connectors.exec"
	SubjectConnectorResult = "connectors.result"

	SubjectApprovalRequest  = "approvals.request"
	SubjectApprovalDecision = "approvals.decision"

	SubjectKillSwitch = "runs.killswitch"
)

// Envelope wraps all NATS messages with tracing and routing metadata.
type Envelope[T any] struct {
	TraceID   string    `json:"trace_id"`
	OrgID     uuid.UUID `json:"org_id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   T         `json:"payload"`
}

// NewEnvelope creates a new message envelope.
func NewEnvelope[T any](orgID uuid.UUID, msgType string, payload T) *Envelope[T] {
	return &Envelope[T]{
		TraceID:   uuid.New().String(),
		OrgID:     orgID,
		Type:      msgType,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}

// RunTriggerMsg is published when a run should be started.
type RunTriggerMsg struct {
	EngagementID uuid.UUID `json:"engagement_id"`
	OrgID        uuid.UUID `json:"org_id"`
	TriggeredBy  string    `json:"triggered_by"` // user ID or "scheduler"
}

// RunStatusMsg is published when a run's status changes.
type RunStatusMsg struct {
	RunID        uuid.UUID `json:"run_id"`
	EngagementID uuid.UUID `json:"engagement_id"`
	Status       string    `json:"status"`
	Message      string    `json:"message,omitempty"`
}

// AgentTaskMsg dispatches a task to an agent.
type AgentTaskMsg struct {
	TaskID       string          `json:"task_id"`
	RunID        uuid.UUID       `json:"run_id"`
	EngagementID uuid.UUID       `json:"engagement_id"`
	StepNumber   int             `json:"step_number"`
	AgentType    string          `json:"agent_type"`
	Action       string          `json:"action"`
	Tier         int             `json:"tier"`
	Inputs       json.RawMessage `json:"inputs"`
}

// AgentResultMsg is the result of an agent task.
type AgentResultMsg struct {
	TaskID      string          `json:"task_id"`
	RunID       uuid.UUID       `json:"run_id"`
	StepNumber  int             `json:"step_number"`
	AgentType   string          `json:"agent_type"`
	Status      string          `json:"status"`
	Outputs     json.RawMessage `json:"outputs,omitempty"`
	EvidenceIDs []string        `json:"evidence_ids,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// KillSwitchMsg is published when the kill switch is engaged or disengaged.
type KillSwitchMsg struct {
	Engaged bool   `json:"engaged"`
	Reason  string `json:"reason"`
	ActorID string `json:"actor_id"`
}
