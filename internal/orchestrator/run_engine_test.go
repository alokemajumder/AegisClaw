package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/internal/receipt"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// ─── Mock Agent ───────────────────────────────────────────────────────────────

type mockAgent struct {
	name    agentsdk.AgentType
	squad   agentsdk.Squad
	handler func(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error)
}

func (m *mockAgent) Name() agentsdk.AgentType                                  { return m.name }
func (m *mockAgent) Squad() agentsdk.Squad                                     { return m.squad }
func (m *mockAgent) Init(_ context.Context, _ agentsdk.AgentDeps) error        { return nil }
func (m *mockAgent) Shutdown(_ context.Context) error                          { return nil }
func (m *mockAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	if m.handler != nil {
		return m.handler(ctx, task)
	}
	return &agentsdk.Result{TaskID: task.ID, Status: agentsdk.StatusCompleted}, nil
}

// ─── Mock Querier ─────────────────────────────────────────────────────────────

// mockRow implements pgx.Row for QueryRow calls. It always succeeds with
// zero-value scans so that repo methods don't panic on Scan().
type mockRow struct{}

func (r *mockRow) Scan(_ ...any) error { return nil }

// mockRows implements pgx.Rows with zero results.
type mockRows struct{ closed bool }

func (r *mockRows) Close()                                         { r.closed = true }
func (r *mockRows) Err() error                                     { return nil }
func (r *mockRows) CommandTag() pgconn.CommandTag                  { return pgconn.NewCommandTag("SELECT 0") }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription   { return nil }
func (r *mockRows) Next() bool                                     { return false }
func (r *mockRows) Scan(_ ...any) error                            { return nil }
func (r *mockRows) Values() ([]any, error)                         { return nil, nil }
func (r *mockRows) RawValues() [][]byte                            { return nil }
func (r *mockRows) Conn() *pgx.Conn                                { return nil }

type mockQuerier struct{}

func (q *mockQuerier) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return &mockRows{}, nil
}

func (q *mockQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &mockRow{}
}

func (q *mockQuerier) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

// ─── Mock Engagement Querier ──────────────────────────────────────────────────
// A special querier that returns a valid engagement on GetByID.

type engagementQuerier struct {
	mockQuerier
	eng *models.Engagement
}

func (q *engagementQuerier) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	return &engagementRow{eng: q.eng}
}

type engagementRow struct {
	eng *models.Engagement
}

func (r *engagementRow) Scan(dest ...any) error {
	if len(dest) < 19 || r.eng == nil {
		return nil
	}
	// Engagement.GetByID scans 19 fields in order:
	// id, org_id, name, description, status, target_allowlist, target_exclusions,
	// allowed_tiers, allowed_techniques, schedule_cron, run_window_start, run_window_end,
	// blackout_periods, rate_limit, concurrency_cap, connector_ids, created_by, created_at, updated_at
	*dest[0].(*uuid.UUID) = r.eng.ID
	*dest[1].(*uuid.UUID) = r.eng.OrgID
	*dest[2].(*string) = r.eng.Name
	if dp, ok := dest[3].(**string); ok {
		*dp = r.eng.Description
	}
	*dest[4].(*models.EngagementStatus) = r.eng.Status
	if dp, ok := dest[5].(*[]uuid.UUID); ok {
		*dp = r.eng.TargetAllowlist
	}
	if dp, ok := dest[6].(*[]uuid.UUID); ok {
		*dp = r.eng.TargetExclusions
	}
	if dp, ok := dest[7].(*[]int); ok {
		*dp = r.eng.AllowedTiers
	}
	if dp, ok := dest[8].(*[]string); ok {
		*dp = r.eng.AllowedTechniques
	}
	// schedule_cron, run_window_start, run_window_end — nullable, leave zero
	// blackout_periods
	if dp, ok := dest[12].(*json.RawMessage); ok {
		*dp = r.eng.BlackoutPeriods
	}
	if dp, ok := dest[13].(*int); ok {
		*dp = r.eng.RateLimit
	}
	if dp, ok := dest[14].(*int); ok {
		*dp = r.eng.ConcurrencyCap
	}
	if dp, ok := dest[15].(*[]uuid.UUID); ok {
		*dp = r.eng.ConnectorIDs
	}
	// created_by, created_at, updated_at — leave zero
	return nil
}

// ─── Test Helpers ─────────────────────────────────────────────────────────────

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestAgentRegistry(agents map[agentsdk.AgentType]agentsdk.Agent) *AgentRegistry {
	return &AgentRegistry{
		agents: agents,
		logger: testLogger(),
	}
}

func newTestEngagement() *models.Engagement {
	return &models.Engagement{
		ID:                uuid.New(),
		OrgID:             uuid.New(),
		Name:              "Test Engagement",
		Status:            models.EngagementActive,
		AllowedTiers:      []int{0, 1},
		TargetAllowlist:   []uuid.UUID{},
		TargetExclusions:  []uuid.UUID{},
		AllowedTechniques: []string{"T1059"},
		BlackoutPeriods:   json.RawMessage(`[]`),
		ConnectorIDs:      []uuid.UUID{},
	}
}

func newTestRun(engID uuid.UUID) *models.Run {
	return &models.Run{
		ID:           uuid.New(),
		EngagementID: engID,
		OrgID:        uuid.New(),
		Status:       models.RunQueued,
		Tier:         1,
		Metadata:     json.RawMessage(`{}`),
	}
}

// newTestRunEngine creates a RunEngine with mock repos and the given agent registry.
// The engagement querier returns the provided engagement on GetByID.
func newTestRunEngine(reg *AgentRegistry, eng *models.Engagement) *RunEngine {
	mq := &mockQuerier{}
	eq := &engagementQuerier{eng: eng}

	return NewRunEngine(
		reg,
		repository.NewRunRepo(mq),
		repository.NewRunStepRepo(mq),
		repository.NewFindingRepo(mq),
		repository.NewEngagementRepo(eq),
		nil, // connectorSvc
		nil, // coverageRepo
		NewKillSwitch(),
		testLogger(),
	)
}

// completedResult returns a simple completed result.
func completedResult(taskID string) *agentsdk.Result {
	return &agentsdk.Result{
		TaskID:      taskID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     json.RawMessage(`{"ok":true}`),
		CompletedAt: time.Now().UTC(),
	}
}

// ─── buildStepRecords Tests (pure function) ───────────────────────────────────

func TestBuildStepRecords(t *testing.T) {
	e := &RunEngine{logger: testLogger()}

	now := time.Now().UTC()
	tests := []struct {
		name     string
		input    []stepAccum
		wantLen  int
		checkFn  func(t *testing.T, records []receipt.StepRecord)
	}{
		{
			name:    "empty accumulator",
			input:   nil,
			wantLen: 0,
		},
		{
			name: "single step",
			input: []stepAccum{
				{
					StepNumber:  1,
					Action:      "drop_marker_file",
					Tier:        0,
					Status:      "completed",
					EvidenceIDs: []string{"ev-1"},
					CleanupDone: true,
					StartedAt:   now,
					CompletedAt: now.Add(5 * time.Second),
				},
			},
			wantLen: 1,
			checkFn: func(t *testing.T, records []receipt.StepRecord) {
				r := records[0]
				assert.Equal(t, 1, r.StepNumber)
				assert.Equal(t, "drop_marker_file", r.Action)
				assert.Equal(t, "drop_marker_file", r.AgentType)
				assert.Equal(t, 0, r.Tier)
				assert.Equal(t, "completed", r.Status)
				assert.True(t, r.CleanupDone)
				assert.Equal(t, []string{"ev-1"}, r.EvidenceIDs)
				assert.Empty(t, r.ErrorMessage)
			},
		},
		{
			name: "multiple steps with error",
			input: []stepAccum{
				{StepNumber: 1, Action: "scan", Tier: 0, Status: "completed", StartedAt: now, CompletedAt: now.Add(time.Second)},
				{StepNumber: 2, Action: "exec", Tier: 1, Status: "failed", Error: "timeout", StartedAt: now, CompletedAt: now.Add(2 * time.Second)},
			},
			wantLen: 2,
			checkFn: func(t *testing.T, records []receipt.StepRecord) {
				assert.Equal(t, "completed", records[0].Status)
				assert.Equal(t, "failed", records[1].Status)
				assert.Equal(t, "timeout", records[1].ErrorMessage)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := e.buildStepRecords(tt.input)
			assert.Len(t, records, tt.wantLen)
			if tt.checkFn != nil {
				tt.checkFn(t, records)
			}
		})
	}
}

// ─── RunEngine.ExecuteRun Integration Tests ───────────────────────────────────

func TestRunEngine_ExecuteRun_Tier0_FullPipeline(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	var executedAgents []agentsdk.AgentType
	var mu sync.Mutex
	trackAgent := func(name agentsdk.AgentType) {
		mu.Lock()
		executedAgents = append(executedAgents, name)
		mu.Unlock()
	}

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				trackAgent(agentsdk.AgentPlanner)
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "step_one", Tier: 0},
						{StepNumber: 2, Action: "step_two", Tier: 0},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				trackAgent(agentsdk.AgentPolicyEnforcer)
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentExecutor: &mockAgent{
			name: agentsdk.AgentExecutor, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				trackAgent(agentsdk.AgentExecutor)
				return &agentsdk.Result{
					TaskID:      task.ID,
					Status:      agentsdk.StatusCompleted,
					Outputs:     json.RawMessage(`{"executed":true}`),
					CleanupDone: true,
				}, nil
			},
		},
		agentsdk.AgentEvidence: &mockAgent{
			name: agentsdk.AgentEvidence, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				trackAgent(agentsdk.AgentEvidence)
				return &agentsdk.Result{TaskID: task.ID, Status: agentsdk.StatusCompleted, EvidenceIDs: []string{"ev-1"}}, nil
			},
		},
		agentsdk.AgentTelemetryVerifier: &mockAgent{
			name: agentsdk.AgentTelemetryVerifier, squad: agentsdk.SquadValidation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				trackAgent(agentsdk.AgentTelemetryVerifier)
				return &agentsdk.Result{TaskID: task.ID, Status: agentsdk.StatusCompleted, Outputs: json.RawMessage(`{"telemetry_found":true}`)}, nil
			},
		},
		agentsdk.AgentDetectionEvaluator: &mockAgent{
			name: agentsdk.AgentDetectionEvaluator, squad: agentsdk.SquadValidation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				trackAgent(agentsdk.AgentDetectionEvaluator)
				return &agentsdk.Result{TaskID: task.ID, Status: agentsdk.StatusCompleted, Outputs: json.RawMessage(`{"alerts_found":0}`)}, nil
			},
		},
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				trackAgent(agentsdk.AgentReceipt)
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err)

	// Verify core agents were called
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, executedAgents, agentsdk.AgentPlanner)
	assert.Contains(t, executedAgents, agentsdk.AgentPolicyEnforcer)
	assert.Contains(t, executedAgents, agentsdk.AgentExecutor)
	// Receipt fires as post-run agent
	assert.Contains(t, executedAgents, agentsdk.AgentReceipt)
}

func TestRunEngine_ExecuteRun_EmptyPlan(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID:   task.ID,
					Status:   agentsdk.StatusCompleted,
					NextSteps: nil, // no steps
				}, nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err)
	// Run completes immediately with no steps — no error.
}

func TestRunEngine_ExecuteRun_PlannerFails(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return nil, fmt.Errorf("planner exploded")
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "planner exploded")
}

func TestRunEngine_ExecuteRun_PolicyBlocks(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	var stepStatus string

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "risky_action", Tier: 3},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				stepStatus = "blocked"
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusBlocked,
					Error:  "tier 3 not allowed",
				}, nil
			},
		},
		// Receipt agent for post-run phase
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err, "run should complete even when steps are blocked")
	assert.Equal(t, "blocked", stepStatus)
}

func TestRunEngine_ExecuteRun_Tier2NeedsApproval(t *testing.T) {
	eng := newTestEngagement()
	eng.AllowedTiers = []int{0, 1, 2}
	run := newTestRun(eng.ID)

	var approvalRequested bool

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "kerberoast", Tier: 2},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusNeedsApproval,
				}, nil
			},
		},
		agentsdk.AgentApprovalGate: &mockAgent{
			name: agentsdk.AgentApprovalGate, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				approvalRequested = true
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusNeedsApproval,
				}, nil
			},
		},
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err)
	assert.True(t, approvalRequested, "approval gate should have been invoked for tier 2 step")
}

func TestRunEngine_ExecuteRun_ExecutorFails(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "exec_step", Tier: 0},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentExecutor: &mockAgent{
			name: agentsdk.AgentExecutor, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID:  task.ID,
					Status:  agentsdk.StatusFailed,
					Error:   "sandbox timeout",
					Outputs: json.RawMessage(`{"error":"sandbox timeout"}`),
				}, nil
			},
		},
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err, "run should complete even when executor fails a step")
}

func TestRunEngine_ExecuteRun_KillSwitchEngaged(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "first", Tier: 0},
						{StepNumber: 2, Action: "second", Tier: 0},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentExecutor: &mockAgent{
			name: agentsdk.AgentExecutor, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID:  task.ID,
					Status:  agentsdk.StatusCompleted,
					Outputs: json.RawMessage(`{}`),
				}, nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	// Engage kill switch before run starts iterating steps
	engine.killSwitch.Engage()

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err)
	// The run should exit via the kill switch check in the step loop.
}

func TestRunEngine_ExecuteRun_ContextCancelled(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	executorCallCount := 0

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "first", Tier: 0},
						{StepNumber: 2, Action: "second", Tier: 0},
						{StepNumber: 3, Action: "third", Tier: 0},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentExecutor: &mockAgent{
			name: agentsdk.AgentExecutor, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				executorCallCount++
				return &agentsdk.Result{
					TaskID:  task.ID,
					Status:  agentsdk.StatusCompleted,
					Outputs: json.RawMessage(`{}`),
				}, nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately — the context will be cancelled when the step loop checks
	cancel()

	err := engine.ExecuteRun(ctx, run)
	// The run may or may not error; the key is it doesn't hang.
	// With cancelled context, UpdateStatus calls will fail, but that's expected.
	_ = err
}

func TestRunEngine_ExecuteRun_NoPolicyEnforcer(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	executorCalled := false

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "test", Tier: 0},
					},
				}, nil
			},
		},
		// No AgentPolicyEnforcer registered — fail-closed
		agentsdk.AgentExecutor: &mockAgent{
			name: agentsdk.AgentExecutor, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				executorCalled = true
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err)
	assert.False(t, executorCalled, "executor should NOT be called when policy enforcer is missing (fail-closed)")
}

func TestRunEngine_ExecuteRun_NoExecutor(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "test", Tier: 0},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
		// No AgentExecutor registered
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err, "run should complete even when executor is missing — steps just fail")
}

func TestRunEngine_ExecuteRun_PlannerReturnsNonCompletedStatus(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusFailed,
					Error:  "no playbooks matched",
				}, nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "planning failed")
}

func TestRunEngine_ExecuteRun_PolicyEnforcerErrors(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	executorCalled := false

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "test", Tier: 0},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return nil, fmt.Errorf("policy engine crashed")
			},
		},
		agentsdk.AgentExecutor: &mockAgent{
			name: agentsdk.AgentExecutor, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				executorCalled = true
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err)
	assert.False(t, executorCalled, "executor should NOT be called when policy enforcer errors (fail-closed)")
}

func TestRunEngine_ExecuteRun_PostRunAgentsFire(t *testing.T) {
	eng := newTestEngagement()
	run := newTestRun(eng.ID)

	var postRunAgents []agentsdk.AgentType
	var mu sync.Mutex
	track := func(name agentsdk.AgentType) {
		mu.Lock()
		postRunAgents = append(postRunAgents, name)
		mu.Unlock()
	}

	agents := map[agentsdk.AgentType]agentsdk.Agent{
		agentsdk.AgentPlanner: &mockAgent{
			name: agentsdk.AgentPlanner, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID: task.ID,
					Status: agentsdk.StatusCompleted,
					NextSteps: []agentsdk.Task{
						{StepNumber: 1, Action: "test_action", Tier: 0, Inputs: json.RawMessage(`{"technique_id":"T1059.001"}`)},
					},
				}, nil
			},
		},
		agentsdk.AgentPolicyEnforcer: &mockAgent{
			name: agentsdk.AgentPolicyEnforcer, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentExecutor: &mockAgent{
			name: agentsdk.AgentExecutor, squad: agentsdk.SquadEmulation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				return &agentsdk.Result{
					TaskID:      task.ID,
					Status:      agentsdk.StatusCompleted,
					Outputs:     json.RawMessage(`{"ok":true}`),
					CleanupDone: true,
					Findings: []agentsdk.FindingOutput{
						{Title: "test finding", Severity: "medium", Confidence: "high"},
					},
				}, nil
			},
		},
		agentsdk.AgentResponseAutomator: &mockAgent{
			name: agentsdk.AgentResponseAutomator, squad: agentsdk.SquadValidation,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				track(agentsdk.AgentResponseAutomator)
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentCoverageMapper: &mockAgent{
			name: agentsdk.AgentCoverageMapper, squad: agentsdk.SquadImprovement,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				track(agentsdk.AgentCoverageMapper)
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentRegression: &mockAgent{
			name: agentsdk.AgentRegression, squad: agentsdk.SquadImprovement,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				track(agentsdk.AgentRegression)
				return completedResult(task.ID), nil
			},
		},
		agentsdk.AgentReceipt: &mockAgent{
			name: agentsdk.AgentReceipt, squad: agentsdk.SquadGovernance,
			handler: func(_ context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
				track(agentsdk.AgentReceipt)
				return completedResult(task.ID), nil
			},
		},
	}

	reg := newTestAgentRegistry(agents)
	engine := newTestRunEngine(reg, eng)

	err := engine.ExecuteRun(context.Background(), run)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, postRunAgents, agentsdk.AgentResponseAutomator, "response automator should fire with findings")
	assert.Contains(t, postRunAgents, agentsdk.AgentCoverageMapper, "coverage mapper should fire with technique results")
	assert.Contains(t, postRunAgents, agentsdk.AgentRegression, "regression should fire")
	assert.Contains(t, postRunAgents, agentsdk.AgentReceipt, "receipt should fire")
}
