package finding

import (
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- computeClusterID tests ---

func TestComputeClusterID_DeterministicForSameInputs(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	title := "SQL Injection in login form"
	techniques := []string{"T1190", "T1059.001"}
	assets := []string{"asset-a", "asset-b"}

	id1 := svc.computeClusterID(orgID, title, techniques, assets)
	id2 := svc.computeClusterID(orgID, title, techniques, assets)

	assert.Equal(t, id1, id2, "same inputs must produce the same cluster ID")
}

func TestComputeClusterID_DeterministicRegardlessOfInputOrder(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	title := "SQL Injection in login form"

	// Techniques and assets in different order should produce the same ID
	// because computeClusterID sorts them.
	id1 := svc.computeClusterID(orgID, title, []string{"T1059.001", "T1190"}, []string{"asset-b", "asset-a"})
	id2 := svc.computeClusterID(orgID, title, []string{"T1190", "T1059.001"}, []string{"asset-a", "asset-b"})

	assert.Equal(t, id1, id2, "order of techniques and assets should not affect cluster ID")
}

func TestComputeClusterID_CaseInsensitiveTitle(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	techniques := []string{"T1190"}
	assets := []string{"asset-a"}

	id1 := svc.computeClusterID(orgID, "SQL Injection", techniques, assets)
	id2 := svc.computeClusterID(orgID, "sql injection", techniques, assets)

	assert.Equal(t, id1, id2, "title comparison should be case-insensitive")
}

func TestComputeClusterID_DifferentInputsProduceDifferentIDs(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	tests := []struct {
		name   string
		titleA string
		techA  []string
		titleB string
		techB  []string
	}{
		{
			name:   "different titles",
			titleA: "SQL Injection",
			techA:  []string{"T1190"},
			titleB: "XSS Attack",
			techB:  []string{"T1190"},
		},
		{
			name:   "different techniques",
			titleA: "SQL Injection",
			techA:  []string{"T1190"},
			titleB: "SQL Injection",
			techB:  []string{"T1059.001"},
		},
		{
			name:   "different number of techniques",
			titleA: "SQL Injection",
			techA:  []string{"T1190"},
			titleB: "SQL Injection",
			techB:  []string{"T1190", "T1059.001"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := []string{"asset-a"}
			idA := svc.computeClusterID(orgID, tt.titleA, tt.techA, assets)
			idB := svc.computeClusterID(orgID, tt.titleB, tt.techB, assets)
			assert.NotEqual(t, idA, idB, "different inputs should produce different cluster IDs")
		})
	}
}

func TestComputeClusterID_DifferentOrgsProduceDifferentIDs(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgA := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgB := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	title := "SQL Injection"
	techniques := []string{"T1190"}
	assets := []string{"asset-a"}

	idA := svc.computeClusterID(orgA, title, techniques, assets)
	idB := svc.computeClusterID(orgB, title, techniques, assets)

	assert.NotEqual(t, idA, idB, "different orgs should produce different cluster IDs")
}

func TestComputeClusterID_NilSlicesHandled(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Should not panic with nil slices.
	id := svc.computeClusterID(orgID, "Test Finding", nil, nil)
	assert.NotEqual(t, uuid.Nil, id, "cluster ID should be a valid non-nil UUID")
}

func TestComputeClusterID_EmptyVsNilSlicesProduceSameID(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	title := "Test Finding"

	idNil := svc.computeClusterID(orgID, title, nil, nil)
	idEmpty := svc.computeClusterID(orgID, title, []string{}, []string{})

	assert.Equal(t, idNil, idEmpty, "nil and empty slices should produce the same cluster ID")
}

func TestComputeClusterID_ReturnsValidUUID(t *testing.T) {
	svc := NewService(nil, newTestLogger())
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	id := svc.computeClusterID(orgID, "Finding Title", []string{"T1190"}, []string{"web-server"})

	// UUID string should be 36 characters (8-4-4-4-12 with hyphens).
	assert.Len(t, id.String(), 36, "cluster ID should be a valid UUID string")
	// Should not be the zero UUID.
	assert.NotEqual(t, uuid.Nil, id)
}

// --- TransitionStatus state machine validation ---
//
// TransitionStatus is a method on *Service that calls FindingRepo.GetByID (DB)
// and FindingRepo.UpdateStatus (DB). Since FindingRepo is a concrete struct
// (not an interface), fully mocking it requires mocking pgx internals.
// Instead, we directly validate the validTransitions map which encodes the
// state machine that TransitionStatus enforces.

// isTransitionAllowed checks whether transitioning from -> to is permitted
// according to the validTransitions map, mirroring the logic in TransitionStatus.
func isTransitionAllowed(from, to models.FindingStatus) bool {
	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}
	for _, v := range allowed {
		if v == to {
			return true
		}
	}
	return false
}

func TestTransitionStatus_ValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from models.FindingStatus
		to   models.FindingStatus
	}{
		{"observed -> needs_review", models.FindingObserved, models.FindingNeedsReview},
		{"observed -> confirmed", models.FindingObserved, models.FindingConfirmed},
		{"observed -> accepted_risk", models.FindingObserved, models.FindingAcceptedRisk},
		{"needs_review -> confirmed", models.FindingNeedsReview, models.FindingConfirmed},
		{"needs_review -> accepted_risk", models.FindingNeedsReview, models.FindingAcceptedRisk},
		{"confirmed -> ticketed", models.FindingConfirmed, models.FindingTicketed},
		{"confirmed -> accepted_risk", models.FindingConfirmed, models.FindingAcceptedRisk},
		{"ticketed -> fixed", models.FindingTicketed, models.FindingFixed},
		{"fixed -> retested", models.FindingFixed, models.FindingRetested},
		{"retested -> closed", models.FindingRetested, models.FindingClosed},
		{"retested -> confirmed (regression)", models.FindingRetested, models.FindingConfirmed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := isTransitionAllowed(tt.from, tt.to)
			require.True(t, allowed, "transition from %s to %s should be valid", tt.from, tt.to)
		})
	}
}

func TestTransitionStatus_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from models.FindingStatus
		to   models.FindingStatus
	}{
		{"observed -> closed", models.FindingObserved, models.FindingClosed},
		{"observed -> ticketed", models.FindingObserved, models.FindingTicketed},
		{"observed -> fixed", models.FindingObserved, models.FindingFixed},
		{"observed -> retested", models.FindingObserved, models.FindingRetested},
		{"needs_review -> closed", models.FindingNeedsReview, models.FindingClosed},
		{"needs_review -> ticketed", models.FindingNeedsReview, models.FindingTicketed},
		{"needs_review -> fixed", models.FindingNeedsReview, models.FindingFixed},
		{"confirmed -> closed", models.FindingConfirmed, models.FindingClosed},
		{"confirmed -> fixed", models.FindingConfirmed, models.FindingFixed},
		{"confirmed -> retested", models.FindingConfirmed, models.FindingRetested},
		{"ticketed -> closed", models.FindingTicketed, models.FindingClosed},
		{"ticketed -> confirmed", models.FindingTicketed, models.FindingConfirmed},
		{"ticketed -> needs_review", models.FindingTicketed, models.FindingNeedsReview},
		{"fixed -> closed", models.FindingFixed, models.FindingClosed},
		{"fixed -> confirmed", models.FindingFixed, models.FindingConfirmed},
		{"retested -> ticketed", models.FindingRetested, models.FindingTicketed},
		{"retested -> fixed", models.FindingRetested, models.FindingFixed},
		{"closed -> observed (terminal)", models.FindingClosed, models.FindingObserved},
		{"closed -> confirmed (terminal)", models.FindingClosed, models.FindingConfirmed},
		{"accepted_risk -> observed (terminal)", models.FindingAcceptedRisk, models.FindingObserved},
		{"accepted_risk -> confirmed (terminal)", models.FindingAcceptedRisk, models.FindingConfirmed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed := isTransitionAllowed(tt.from, tt.to)
			require.False(t, allowed, "transition from %s to %s should be invalid", tt.from, tt.to)
		})
	}
}

func TestTransitionStatus_SameStatusIsInvalid(t *testing.T) {
	allStatuses := []models.FindingStatus{
		models.FindingObserved,
		models.FindingNeedsReview,
		models.FindingConfirmed,
		models.FindingTicketed,
		models.FindingFixed,
		models.FindingRetested,
		models.FindingClosed,
		models.FindingAcceptedRisk,
	}

	for _, status := range allStatuses {
		t.Run(fmt.Sprintf("%s -> %s", status, status), func(t *testing.T) {
			allowed := isTransitionAllowed(status, status)
			require.False(t, allowed, "transitioning to the same status should be invalid")
		})
	}
}

func TestTransitionStatus_TerminalStatesHaveNoOutboundTransitions(t *testing.T) {
	terminalStatuses := []models.FindingStatus{
		models.FindingClosed,
		models.FindingAcceptedRisk,
	}

	allStatuses := []models.FindingStatus{
		models.FindingObserved,
		models.FindingNeedsReview,
		models.FindingConfirmed,
		models.FindingTicketed,
		models.FindingFixed,
		models.FindingRetested,
		models.FindingClosed,
		models.FindingAcceptedRisk,
	}

	for _, terminal := range terminalStatuses {
		t.Run(string(terminal), func(t *testing.T) {
			for _, target := range allStatuses {
				allowed := isTransitionAllowed(terminal, target)
				assert.False(t, allowed,
					"%s should be a terminal state with no transitions, but %s -> %s was allowed",
					terminal, terminal, target)
			}
		})
	}
}

func TestValidTransitionsMap_AllSourceStatesHaveTargets(t *testing.T) {
	// Every non-terminal status that appears as a key in validTransitions
	// should have at least one valid target state.
	for from, targets := range validTransitions {
		assert.NotEmpty(t, targets, "status %s has no valid transitions defined", from)
	}
}

func TestValidTransitionsMap_AllTargetsAreValidStatuses(t *testing.T) {
	knownStatuses := map[models.FindingStatus]bool{
		models.FindingObserved:    true,
		models.FindingNeedsReview: true,
		models.FindingConfirmed:   true,
		models.FindingTicketed:    true,
		models.FindingFixed:       true,
		models.FindingRetested:    true,
		models.FindingClosed:      true,
		models.FindingAcceptedRisk: true,
	}

	for from, targets := range validTransitions {
		for _, to := range targets {
			assert.True(t, knownStatuses[to],
				"transition %s -> %s references unknown status %s", from, to, to)
		}
	}
}
