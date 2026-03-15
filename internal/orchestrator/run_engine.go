package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/connector"
	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/internal/receipt"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// KillSwitch tracks the global kill switch state.
type KillSwitch struct {
	mu      sync.RWMutex
	engaged bool
}

func NewKillSwitch() *KillSwitch {
	return &KillSwitch{}
}

func (ks *KillSwitch) IsEngaged() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.engaged
}

func (ks *KillSwitch) Engage() {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.engaged = true
}

func (ks *KillSwitch) Disengage() {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.engaged = false
}

// stepAccum accumulates results from each step for post-run agents.
type stepAccum struct {
	StepNumber   int
	Action       string
	Tier         int
	Status       string
	TechniqueID  string
	EvidenceIDs  []string
	Findings     []agentsdk.FindingOutput
	HasTelemetry bool
	HasDetection bool
	HasAlert     bool
	CleanupDone  bool
	Error        string
	StartedAt    time.Time
	CompletedAt  time.Time
}

// RunEngine executes runs end-to-end.
type RunEngine struct {
	agents       *AgentRegistry
	runs         *repository.RunRepo
	steps        *repository.RunStepRepo
	findings     *repository.FindingRepo
	engagements  *repository.EngagementRepo
	connectorSvc *connector.Service
	coverageRepo *repository.CoverageRepo
	killSwitch   *KillSwitch
	logger       *slog.Logger
}

// NewRunEngine creates a new run engine.
func NewRunEngine(
	agents *AgentRegistry,
	runs *repository.RunRepo,
	steps *repository.RunStepRepo,
	findings *repository.FindingRepo,
	engagements *repository.EngagementRepo,
	connectorSvc *connector.Service,
	coverageRepo *repository.CoverageRepo,
	killSwitch *KillSwitch,
	logger *slog.Logger,
) *RunEngine {
	return &RunEngine{
		agents:       agents,
		runs:         runs,
		steps:        steps,
		findings:     findings,
		engagements:  engagements,
		connectorSvc: connectorSvc,
		coverageRepo: coverageRepo,
		killSwitch:   killSwitch,
		logger:       logger,
	}
}

// ExecuteRun runs an engagement end-to-end through the full agent pipeline.
func (e *RunEngine) ExecuteRun(ctx context.Context, run *models.Run) error {
	e.logger.Info("starting run execution", "run_id", run.ID, "engagement_id", run.EngagementID)
	runStart := time.Now().UTC()

	// Update status to running
	if err := e.runs.UpdateStatus(ctx, run.ID, models.RunRunning); err != nil {
		return fmt.Errorf("updating run status: %w", err)
	}

	eng, err := e.engagements.GetByID(ctx, run.EngagementID)
	if err != nil {
		e.failRun(ctx, run.ID, "engagement not found")
		return fmt.Errorf("getting engagement: %w", err)
	}

	// Resolve connector IDs from the engagement
	connectorMap := e.resolveConnectors(ctx, eng)

	// Snapshot pre-run coverage for drift detection
	preCoverage := e.snapshotCoverage(ctx, eng.OrgID)

	// ── Phase 1: Planning ──────────────────────────────────────────────
	policyCtx := &agentsdk.PolicyContext{
		AllowedTiers:     eng.AllowedTiers,
		TargetAllowlist:  eng.TargetAllowlist,
		TargetExclusions: eng.TargetExclusions,
		RateLimit:        eng.RateLimit,
		ConcurrencyCap:   eng.ConcurrencyCap,
	}

	planTask := &agentsdk.Task{
		ID:            uuid.New().String(),
		RunID:         run.ID,
		EngagementID:  eng.ID,
		OrgID:         eng.OrgID,
		StepNumber:    0,
		Action:        "plan",
		Tier:          0,
		PolicyContext: policyCtx,
		CreatedAt:     time.Now().UTC(),
	}

	planner, err := e.agents.Get(agentsdk.AgentPlanner)
	if err != nil {
		e.failRun(ctx, run.ID, "planner agent not found")
		return err
	}

	planResult, err := planner.HandleTask(ctx, planTask)
	if err != nil {
		e.failRun(ctx, run.ID, "planning failed: "+err.Error())
		return err
	}

	if planResult.Status != agentsdk.StatusCompleted {
		e.failRun(ctx, run.ID, "planning returned non-completed status")
		return fmt.Errorf("planning failed: %s", planResult.Error)
	}

	plannedSteps := planResult.NextSteps
	if len(plannedSteps) == 0 {
		e.logger.Info("no steps planned, completing run", "run_id", run.ID)
		return e.runs.UpdateStatus(ctx, run.ID, models.RunCompleted)
	}

	if err := e.runs.SetStepsTotal(ctx, run.ID, len(plannedSteps)); err != nil {
		e.logger.Error("setting steps total", "error", err)
	}

	// ── Phase 2: Per-step execution ────────────────────────────────────
	var allAccum []stepAccum
	var allEvidenceIDs []string

	for _, stepTask := range plannedSteps {
		if e.killSwitch.IsEngaged() {
			e.logger.Warn("kill switch engaged, stopping run", "run_id", run.ID)
			return e.runs.UpdateStatus(ctx, run.ID, models.RunKilled)
		}
		if ctx.Err() != nil {
			return e.runs.UpdateStatus(ctx, run.ID, models.RunCancelled)
		}

		accum := e.executeStep(ctx, run, eng, &stepTask, policyCtx, connectorMap)
		allAccum = append(allAccum, accum)
		allEvidenceIDs = append(allEvidenceIDs, accum.EvidenceIDs...)
	}

	// ── Phase 3: Post-run agents ───────────────────────────────────────
	e.runPostStepAgents(ctx, run, eng, allAccum, allEvidenceIDs, connectorMap, preCoverage, policyCtx, runStart)

	// Complete run
	return e.runs.UpdateStatus(ctx, run.ID, models.RunCompleted)
}

// executeStep runs a single step through the per-step agent pipeline.
func (e *RunEngine) executeStep(
	ctx context.Context,
	run *models.Run,
	eng *models.Engagement,
	stepTask *agentsdk.Task,
	policyCtx *agentsdk.PolicyContext,
	connectorMap map[string]string,
) stepAccum {
	accum := stepAccum{
		StepNumber: stepTask.StepNumber,
		Action:     stepTask.Action,
		Tier:       stepTask.Tier,
		StartedAt:  time.Now().UTC(),
	}

	// Extract technique ID from step inputs
	if stepTask.Inputs != nil {
		var inputs map[string]any
		_ = json.Unmarshal(stepTask.Inputs, &inputs)
		if tid, ok := inputs["technique_id"].(string); ok {
			accum.TechniqueID = tid
		}
	}

	// Create step record
	stepRecord := &models.RunStep{
		RunID:      run.ID,
		StepNumber: stepTask.StepNumber,
		AgentType:  stepTask.Action,
		Action:     stepTask.Action,
		Tier:       stepTask.Tier,
		Status:     models.StepPending,
		Inputs:     stepTask.Inputs,
	}
	if stepRecord.Inputs == nil {
		stepRecord.Inputs = json.RawMessage(`{}`)
	}
	if err := e.steps.Create(ctx, stepRecord); err != nil {
		e.logger.Error("creating step record", "error", err)
		accum.Status = "failed"
		accum.Error = "failed to create step record"
		accum.CompletedAt = time.Now().UTC()
		return accum
	}

	if err := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepRunning, nil); err != nil {
		e.logger.Error("updating step to running", "error", err, "step_id", stepRecord.ID)
	}

	// ─ 2a. PolicyEnforcer (mandatory — fail-closed) ─
	enforcer, err := e.agents.Get(agentsdk.AgentPolicyEnforcer)
	if err != nil {
		errMsg := "policy enforcer agent not found — cannot proceed without policy evaluation"
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepBlocked, &errMsg); updateErr != nil {
			e.logger.Error("updating step to blocked (no policy enforcer)", "error", updateErr, "step_id", stepRecord.ID)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps (no policy enforcer)", "error", incErr, "run_id", run.ID)
		}
		accum.Status = "blocked"
		accum.Error = errMsg
		accum.CompletedAt = time.Now().UTC()
		return accum
	}
	{
		policyResult, policyErr := enforcer.HandleTask(ctx, &agentsdk.Task{
			ID:            uuid.New().String(),
			RunID:         run.ID,
			EngagementID:  eng.ID,
			OrgID:         eng.OrgID,
			StepNumber:    stepTask.StepNumber,
			Action:        stepTask.Action,
			Tier:          stepTask.Tier,
			PolicyContext: policyCtx,
			CreatedAt:     time.Now().UTC(),
		})
		// SECURITY: Fail-closed — if PolicyEnforcer errors or returns nil, block the step.
		if policyErr != nil || policyResult == nil {
			errMsg := "policy enforcer failed — blocking step (fail-closed)"
			if policyErr != nil {
				errMsg = "policy enforcer error: " + policyErr.Error()
			}
			e.logger.Error(errMsg, "step", stepTask.StepNumber)
			if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepBlocked, &errMsg); updateErr != nil {
				e.logger.Error("updating step to blocked (policy error)", "error", updateErr, "step_id", stepRecord.ID)
			}
			if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
				e.logger.Error("incrementing failed steps (policy error)", "error", incErr, "run_id", run.ID)
			}
			accum.Status = "blocked"
			accum.Error = errMsg
			accum.CompletedAt = time.Now().UTC()
			return accum
		}
		if policyResult.Status == agentsdk.StatusBlocked {
			errMsg := "blocked by policy: " + policyResult.Error
			if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepBlocked, &errMsg); updateErr != nil {
				e.logger.Error("updating step to blocked by policy", "error", updateErr, "step_id", stepRecord.ID)
			}
			if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
				e.logger.Error("incrementing failed steps after policy block", "error", incErr, "run_id", run.ID)
			}
			accum.Status = "blocked"
			accum.Error = errMsg
			accum.CompletedAt = time.Now().UTC()
			return accum
		}
		if policyResult.Status == agentsdk.StatusNeedsApproval {
			// ─ 2b. ApprovalGate ─
			accum = e.handleApproval(ctx, run, eng, stepTask, stepRecord, accum)
			if accum.Status == "awaiting_approval" {
				return accum
			}
		}
	}

	// ─ 2c. Executor ─
	executor, err := e.agents.Get(agentsdk.AgentExecutor)
	if err != nil {
		errMsg := "executor agent not found"
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg); updateErr != nil {
			e.logger.Error("updating step to failed (executor not found)", "error", updateErr, "step_id", stepRecord.ID)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps (executor not found)", "error", incErr, "run_id", run.ID)
		}
		accum.Status = "failed"
		accum.Error = errMsg
		accum.CompletedAt = time.Now().UTC()
		return accum
	}

	execResult, execErr := executor.HandleTask(ctx, &agentsdk.Task{
		ID:            uuid.New().String(),
		RunID:         run.ID,
		EngagementID:  eng.ID,
		OrgID:         eng.OrgID,
		StepNumber:    stepTask.StepNumber,
		Action:        stepTask.Action,
		Tier:          stepTask.Tier,
		Inputs:        stepTask.Inputs,
		PolicyContext: policyCtx,
		CreatedAt:     time.Now().UTC(),
	})
	if execErr != nil || execResult == nil {
		errMsg := "executor returned nil result"
		if execErr != nil {
			errMsg = execErr.Error()
		}
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg); updateErr != nil {
			e.logger.Error("updating step to failed (executor error)", "error", updateErr, "step_id", stepRecord.ID)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps (executor error)", "error", incErr, "run_id", run.ID)
		}
		accum.Status = "failed"
		accum.Error = errMsg
		accum.CompletedAt = time.Now().UTC()
		return accum
	}

	accum.CleanupDone = execResult.CleanupDone
	accum.Findings = append(accum.Findings, execResult.Findings...)
	accum.EvidenceIDs = append(accum.EvidenceIDs, execResult.EvidenceIDs...)

	if execResult.Status == agentsdk.StatusFailed {
		errMsg := execResult.Error
		if setErr := e.steps.SetOutputs(ctx, stepRecord.ID, execResult.Outputs, execResult.EvidenceIDs, execResult.CleanupDone); setErr != nil {
			e.logger.Error("setting step outputs (executor failed)", "error", setErr, "step_id", stepRecord.ID)
		}
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg); updateErr != nil {
			e.logger.Error("updating step to failed (executor result)", "error", updateErr, "step_id", stepRecord.ID)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps (executor result)", "error", incErr, "run_id", run.ID)
		}
		accum.Status = "failed"
		accum.Error = errMsg
		accum.CompletedAt = time.Now().UTC()
		e.createFindings(ctx, eng, run, stepRecord, execResult.Findings)
		return accum
	}

	// ─ 2d. EvidenceAgent ─
	evidenceAgent, evAgentErr := e.agents.Get(agentsdk.AgentEvidence)
	if evAgentErr != nil {
		e.logger.Warn("evidence agent not registered, skipping", "error", evAgentErr)
	} else if evidenceAgent != nil {
		evInputs, _ := json.Marshal(map[string]any{
			"executor_outputs": execResult.Outputs,
			"evidence_ids":     execResult.EvidenceIDs,
			"action":           stepTask.Action,
		})
		evResult, evErr := evidenceAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			StepNumber:   stepTask.StepNumber,
			Action:       stepTask.Action,
			Tier:         stepTask.Tier,
			Inputs:       evInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if evErr != nil {
			e.logger.Error("evidence agent failed", "error", evErr, "step", stepTask.StepNumber)
		} else if evResult != nil {
			accum.EvidenceIDs = append(accum.EvidenceIDs, evResult.EvidenceIDs...)
		}
	}

	// ─ 2e. TelemetryVerifier ─
	tvAgent, tvAgentErr := e.agents.Get(agentsdk.AgentTelemetryVerifier)
	if tvAgentErr != nil {
		e.logger.Warn("telemetry verifier not registered, skipping", "error", tvAgentErr)
	} else if tvAgent != nil {
		tvInputs, _ := json.Marshal(map[string]any{
			"siem_connector_id":  connectorMap["siem_connector_id"],
			"edr_connector_id":   connectorMap["edr_connector_id"],
			"action":             stepTask.Action,
			"time_range_minutes": 60,
		})
		tvResult, tvErr := tvAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			StepNumber:   stepTask.StepNumber,
			Action:       stepTask.Action,
			Tier:         stepTask.Tier,
			Inputs:       tvInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if tvErr != nil {
			e.logger.Error("telemetry verifier failed", "error", tvErr, "step", stepTask.StepNumber)
		} else if tvResult != nil {
			accum.Findings = append(accum.Findings, tvResult.Findings...)
			// Parse telemetry result for coverage tracking
			var tvOut map[string]any
			if unmarshalErr := json.Unmarshal(tvResult.Outputs, &tvOut); unmarshalErr == nil {
				if found, ok := tvOut["telemetry_found"].(bool); ok {
					accum.HasTelemetry = found
				}
			}
		}
	}

	// ─ 2f. DetectionEvaluator ─
	deAgent, deAgentErr := e.agents.Get(agentsdk.AgentDetectionEvaluator)
	if deAgentErr != nil {
		e.logger.Warn("detection evaluator not registered, skipping", "error", deAgentErr)
	} else if deAgent != nil {
		deInputs, _ := json.Marshal(map[string]any{
			"siem_connector_id":  connectorMap["siem_connector_id"],
			"edr_connector_id":   connectorMap["edr_connector_id"],
			"action":             stepTask.Action,
			"expected_technique": accum.TechniqueID,
			"max_latency_sec":    120,
		})
		deResult, deErr := deAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			StepNumber:   stepTask.StepNumber,
			Action:       stepTask.Action,
			Tier:         stepTask.Tier,
			Inputs:       deInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if deErr != nil {
			e.logger.Error("detection evaluator failed", "error", deErr, "step", stepTask.StepNumber)
		} else if deResult != nil {
			accum.Findings = append(accum.Findings, deResult.Findings...)
			var deOut map[string]any
			if unmarshalErr := json.Unmarshal(deResult.Outputs, &deOut); unmarshalErr == nil {
				if alerts, ok := deOut["alerts_found"].(float64); ok && alerts > 0 {
					accum.HasDetection = true
					accum.HasAlert = true
				}
			}
		}
	}

	// Update step record with combined results
	allStepEvidence := accum.EvidenceIDs
	if setErr := e.steps.SetOutputs(ctx, stepRecord.ID, execResult.Outputs, allStepEvidence, accum.CleanupDone); setErr != nil {
		e.logger.Error("setting step outputs", "error", setErr, "step_id", stepRecord.ID)
	}
	if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepCompleted, nil); updateErr != nil {
		e.logger.Error("updating step to completed", "error", updateErr, "step_id", stepRecord.ID)
	}
	if incErr := e.runs.IncrementSteps(ctx, run.ID, 1, 0); incErr != nil {
		e.logger.Error("incrementing completed steps", "error", incErr, "run_id", run.ID)
	}

	accum.Status = "completed"
	accum.CompletedAt = time.Now().UTC()

	// Create findings from all agents for this step
	e.createFindings(ctx, eng, run, stepRecord, accum.Findings)

	return accum
}

// handleApproval dispatches to the ApprovalGate agent for Tier 2+ steps.
func (e *RunEngine) handleApproval(
	ctx context.Context,
	run *models.Run,
	eng *models.Engagement,
	stepTask *agentsdk.Task,
	stepRecord *models.RunStep,
	accum stepAccum,
) stepAccum {
	approvalAgent, _ := e.agents.Get(agentsdk.AgentApprovalGate)
	if approvalAgent == nil {
		// No approval agent — block the step
		errMsg := "tier 2+ requires approval but no approval gate agent available"
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepBlocked, &errMsg); updateErr != nil {
			e.logger.Error("updating step to blocked (no approval agent)", "error", updateErr, "step_id", stepRecord.ID)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps (no approval agent)", "error", incErr, "run_id", run.ID)
		}
		accum.Status = "blocked"
		accum.Error = errMsg
		accum.CompletedAt = time.Now().UTC()
		return accum
	}

	approvalResult, approvalErr := approvalAgent.HandleTask(ctx, &agentsdk.Task{
		ID:           uuid.New().String(),
		RunID:        run.ID,
		EngagementID: eng.ID,
		OrgID:        eng.OrgID,
		StepNumber:   stepTask.StepNumber,
		Action:       stepTask.Action,
		Tier:         stepTask.Tier,
		CreatedAt:    time.Now().UTC(),
	})

	if approvalErr != nil {
		errMsg := "approval gate error: " + approvalErr.Error()
		e.logger.Error(errMsg, "step", stepTask.StepNumber)
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepBlocked, &errMsg); updateErr != nil {
			e.logger.Error("updating step to blocked (approval error)", "error", updateErr, "step_id", stepRecord.ID)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps (approval error)", "error", incErr, "run_id", run.ID)
		}
		accum.Status = "blocked"
		accum.Error = errMsg
		accum.CompletedAt = time.Now().UTC()
		return accum
	}
	if approvalResult != nil && approvalResult.Status == agentsdk.StatusNeedsApproval {
		e.logger.Info("step awaiting human approval, skipping execution",
			"run_id", run.ID,
			"step", stepTask.StepNumber,
			"action", stepTask.Action,
			"tier", stepTask.Tier,
		)
		errMsg := "awaiting human approval for tier 2+ action"
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepBlocked, &errMsg); updateErr != nil {
			e.logger.Error("updating step to blocked (awaiting approval)", "error", updateErr, "step_id", stepRecord.ID)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps (awaiting approval)", "error", incErr, "run_id", run.ID)
		}
		accum.Status = "awaiting_approval"
		accum.Error = errMsg
		accum.CompletedAt = time.Now().UTC()
		return accum
	}

	// Approval was auto-granted (shouldn't happen for tier 2+, but handle gracefully)
	return accum
}

// ExecuteApprovedStep resumes a blocked step after human approval, skipping
// PolicyEnforcer and ApprovalGate since the step is already approved.
func (e *RunEngine) ExecuteApprovedStep(ctx context.Context, run *models.Run, eng *models.Engagement, stepNumber int) error {
	e.logger.Info("executing approved step", "run_id", run.ID, "step_number", stepNumber)

	// Find the blocked step record
	steps, err := e.steps.ListByRunID(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("listing run steps: %w", err)
	}

	var stepRecord *models.RunStep
	for i := range steps {
		if steps[i].StepNumber == stepNumber && steps[i].Status == models.StepBlocked {
			stepRecord = &steps[i]
			break
		}
	}
	if stepRecord == nil {
		return fmt.Errorf("blocked step %d not found for run %s", stepNumber, run.ID)
	}

	// Resolve connectors for agent pipeline
	connectorMap := e.resolveConnectors(ctx, eng)

	// Mark step as running
	if err := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepRunning, nil); err != nil {
		e.logger.Error("updating approved step to running", "error", err, "step_id", stepRecord.ID)
	}

	// Build a task from the step record
	stepTask := &agentsdk.Task{
		ID:           uuid.New().String(),
		RunID:        run.ID,
		EngagementID: eng.ID,
		OrgID:        eng.OrgID,
		StepNumber:   stepRecord.StepNumber,
		Action:       stepRecord.Action,
		Tier:         stepRecord.Tier,
		Inputs:       stepRecord.Inputs,
		CreatedAt:    time.Now().UTC(),
	}

	accum := stepAccum{
		StepNumber: stepRecord.StepNumber,
		Action:     stepRecord.Action,
		Tier:       stepRecord.Tier,
		StartedAt:  time.Now().UTC(),
	}

	// Extract technique ID from step inputs
	if stepRecord.Inputs != nil {
		var inputs map[string]any
		_ = json.Unmarshal(stepRecord.Inputs, &inputs)
		if tid, ok := inputs["technique_id"].(string); ok {
			accum.TechniqueID = tid
		}
	}

	// ─ Executor (skip PolicyEnforcer and ApprovalGate — already approved) ─
	executor, err := e.agents.Get(agentsdk.AgentExecutor)
	if err != nil {
		errMsg := "executor agent not found"
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg); updateErr != nil {
			e.logger.Error("updating step status after executor not found", "error", updateErr)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps", "error", incErr)
		}
		return fmt.Errorf("executor agent not found")
	}

	execResult, err := executor.HandleTask(ctx, stepTask)
	if err != nil {
		errMsg := err.Error()
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg); updateErr != nil {
			e.logger.Error("updating step status after executor error", "error", updateErr)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps", "error", incErr)
		}
		return fmt.Errorf("executor failed: %w", err)
	}

	accum.CleanupDone = execResult.CleanupDone
	accum.Findings = append(accum.Findings, execResult.Findings...)
	accum.EvidenceIDs = append(accum.EvidenceIDs, execResult.EvidenceIDs...)

	if execResult.Status == agentsdk.StatusFailed {
		errMsg := execResult.Error
		if setErr := e.steps.SetOutputs(ctx, stepRecord.ID, execResult.Outputs, execResult.EvidenceIDs, execResult.CleanupDone); setErr != nil {
			e.logger.Error("setting step outputs after executor failure", "error", setErr)
		}
		if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepFailed, &errMsg); updateErr != nil {
			e.logger.Error("updating step status after executor failure", "error", updateErr)
		}
		if incErr := e.runs.IncrementSteps(ctx, run.ID, 0, 1); incErr != nil {
			e.logger.Error("incrementing failed steps", "error", incErr)
		}
		e.createFindings(ctx, eng, run, stepRecord, execResult.Findings)
		return e.checkRunCompletion(ctx, run)
	}

	// ─ EvidenceAgent ─
	evidenceAgent, evAgentErr := e.agents.Get(agentsdk.AgentEvidence)
	if evAgentErr != nil {
		e.logger.Warn("evidence agent not registered, skipping", "error", evAgentErr)
	} else if evidenceAgent != nil {
		evInputs, _ := json.Marshal(map[string]any{
			"executor_outputs": execResult.Outputs,
			"evidence_ids":     execResult.EvidenceIDs,
			"action":           stepTask.Action,
		})
		evResult, evErr := evidenceAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			StepNumber:   stepTask.StepNumber,
			Action:       stepTask.Action,
			Tier:         stepTask.Tier,
			Inputs:       evInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if evErr != nil {
			e.logger.Error("evidence agent failed", "error", evErr, "step", stepTask.StepNumber)
		} else if evResult != nil {
			accum.EvidenceIDs = append(accum.EvidenceIDs, evResult.EvidenceIDs...)
		}
	}

	// ─ TelemetryVerifier ─
	tvAgent, tvAgentErr := e.agents.Get(agentsdk.AgentTelemetryVerifier)
	if tvAgentErr != nil {
		e.logger.Warn("telemetry verifier not registered, skipping", "error", tvAgentErr)
	} else if tvAgent != nil {
		tvInputs, _ := json.Marshal(map[string]any{
			"siem_connector_id":  connectorMap["siem_connector_id"],
			"edr_connector_id":   connectorMap["edr_connector_id"],
			"action":             stepTask.Action,
			"time_range_minutes": 60,
		})
		tvResult, tvErr := tvAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			StepNumber:   stepTask.StepNumber,
			Action:       stepTask.Action,
			Tier:         stepTask.Tier,
			Inputs:       tvInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if tvErr != nil {
			e.logger.Error("telemetry verifier failed", "error", tvErr, "step", stepTask.StepNumber)
		} else if tvResult != nil {
			accum.Findings = append(accum.Findings, tvResult.Findings...)
			var tvOut map[string]any
			if unmarshalErr := json.Unmarshal(tvResult.Outputs, &tvOut); unmarshalErr == nil {
				if found, ok := tvOut["telemetry_found"].(bool); ok {
					accum.HasTelemetry = found
				}
			}
		}
	}

	// ─ DetectionEvaluator ─
	deAgent, deAgentErr := e.agents.Get(agentsdk.AgentDetectionEvaluator)
	if deAgentErr != nil {
		e.logger.Warn("detection evaluator not registered, skipping", "error", deAgentErr)
	} else if deAgent != nil {
		deInputs, _ := json.Marshal(map[string]any{
			"siem_connector_id":  connectorMap["siem_connector_id"],
			"edr_connector_id":   connectorMap["edr_connector_id"],
			"action":             stepTask.Action,
			"expected_technique": accum.TechniqueID,
			"max_latency_sec":    120,
		})
		deResult, deErr := deAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			StepNumber:   stepTask.StepNumber,
			Action:       stepTask.Action,
			Tier:         stepTask.Tier,
			Inputs:       deInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if deErr != nil {
			e.logger.Error("detection evaluator failed", "error", deErr, "step", stepTask.StepNumber)
		} else if deResult != nil {
			accum.Findings = append(accum.Findings, deResult.Findings...)
			var deOut map[string]any
			if unmarshalErr := json.Unmarshal(deResult.Outputs, &deOut); unmarshalErr == nil {
				if alerts, ok := deOut["alerts_found"].(float64); ok && alerts > 0 {
					accum.HasDetection = true
					accum.HasAlert = true
				}
			}
		}
	}

	// Update step record with combined results
	if setErr := e.steps.SetOutputs(ctx, stepRecord.ID, execResult.Outputs, accum.EvidenceIDs, accum.CleanupDone); setErr != nil {
		e.logger.Error("setting step outputs for approved step", "error", setErr)
	}
	if updateErr := e.steps.UpdateStatus(ctx, stepRecord.ID, models.StepCompleted, nil); updateErr != nil {
		e.logger.Error("updating step status to completed for approved step", "error", updateErr)
	}
	if incErr := e.runs.IncrementSteps(ctx, run.ID, 1, 0); incErr != nil {
		e.logger.Error("incrementing completed steps for approved step", "error", incErr)
	}

	// Create findings from all agents for this step
	e.createFindings(ctx, eng, run, stepRecord, accum.Findings)

	// Check if the run can be completed
	return e.checkRunCompletion(ctx, run)
}

// checkRunCompletion checks if all steps in a run are done and completes the run if so.
func (e *RunEngine) checkRunCompletion(ctx context.Context, run *models.Run) error {
	steps, err := e.steps.ListByRunID(ctx, run.ID)
	if err != nil {
		return fmt.Errorf("listing steps for completion check: %w", err)
	}

	for _, s := range steps {
		if s.Status == models.StepPending || s.Status == models.StepRunning || s.Status == models.StepBlocked {
			e.logger.Info("run still has pending/blocked steps, not completing yet", "run_id", run.ID, "step_number", s.StepNumber, "status", s.Status)
			return nil
		}
	}

	e.logger.Info("all steps finished, completing run", "run_id", run.ID)
	return e.runs.UpdateStatus(ctx, run.ID, models.RunCompleted)
}

// runPostStepAgents runs agents that operate on the full run results.
func (e *RunEngine) runPostStepAgents(
	ctx context.Context,
	run *models.Run,
	eng *models.Engagement,
	allAccum []stepAccum,
	allEvidenceIDs []string,
	connectorMap map[string]string,
	preCoverage map[string]preCoverageState,
	policyCtx *agentsdk.PolicyContext,
	runStart time.Time,
) {
	// Collect all findings for response automator
	var allFindingSummaries []map[string]any
	for _, a := range allAccum {
		for _, f := range a.Findings {
			allFindingSummaries = append(allFindingSummaries, map[string]any{
				"title":       f.Title,
				"description": f.Description,
				"severity":    f.Severity,
			})
		}
	}

	// ─ 3a. ResponseAutomator ─
	raAgent, raErr := e.agents.Get(agentsdk.AgentResponseAutomator)
	if raErr != nil {
		e.logger.Warn("response automator not registered, skipping", "error", raErr)
	}
	_ = raErr
	if raAgent != nil && len(allFindingSummaries) > 0 {
		raInputs, _ := json.Marshal(map[string]any{
			"findings":                  allFindingSummaries,
			"itsm_connector_id":         connectorMap["itsm_connector_id"],
			"notification_connector_id": connectorMap["notification_connector_id"],
		})
		_, err := raAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			Action:       "automate_response",
			Inputs:       raInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if err != nil {
			e.logger.Error("response automator failed", "error", err)
		}
	}

	// Build technique results for coverage mapper
	var techniqueResults []map[string]any
	for _, a := range allAccum {
		if a.TechniqueID != "" && a.Status == "completed" {
			techniqueResults = append(techniqueResults, map[string]any{
				"technique_id":  a.TechniqueID,
				"has_telemetry": a.HasTelemetry,
				"has_detection": a.HasDetection,
				"has_alert":     a.HasAlert,
			})
		}
	}

	// ─ 3b. CoverageMapper ─
	cmAgent, cmErr := e.agents.Get(agentsdk.AgentCoverageMapper)
	if cmErr != nil {
		e.logger.Warn("coverage mapper not registered, skipping", "error", cmErr)
	}
	_ = cmErr
	if cmAgent != nil && len(techniqueResults) > 0 {
		cmInputs, _ := json.Marshal(map[string]any{
			"technique_results": techniqueResults,
		})
		_, err := cmAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			Action:       "update_coverage",
			Inputs:       cmInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if err != nil {
			e.logger.Error("coverage mapper failed", "error", err)
		}
	}

	// ─ 3c. DriftAgent ─
	driftAgent, driftErr := e.agents.Get(agentsdk.AgentDrift)
	if driftErr != nil {
		e.logger.Warn("drift agent not registered, skipping", "error", driftErr)
	}
	_ = driftErr
	if driftAgent != nil && len(preCoverage) > 0 {
		driftInputs, _ := json.Marshal(map[string]any{
			"previous_coverage": preCoverage,
		})
		driftResult, err := driftAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			Action:       "detect_drift",
			Inputs:       driftInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if err != nil {
			e.logger.Error("drift agent failed", "error", err)
		} else if driftResult != nil {
			// Persist drift findings
			for _, f := range driftResult.Findings {
				e.createSingleFinding(ctx, eng, run, nil, f)
			}
		}
	}

	// ─ 3d. RegressionAgent ─
	regAgent, regErr := e.agents.Get(agentsdk.AgentRegression)
	if regErr != nil {
		e.logger.Warn("regression agent not registered, skipping", "error", regErr)
	}
	_ = regErr
	if regAgent != nil {
		regInputs, _ := json.Marshal(map[string]any{
			"current_run_id": run.ID.String(),
		})
		regResult, err := regAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			Action:       "check_regressions",
			Inputs:       regInputs,
			CreatedAt:    time.Now().UTC(),
		})
		if err != nil {
			e.logger.Error("regression agent failed", "error", err)
		} else if regResult != nil {
			for _, f := range regResult.Findings {
				e.createSingleFinding(ctx, eng, run, nil, f)
			}
		}
	}

	// ─ 3e. ReceiptAgent (with full step data) ─
	receiptAgent, rcptErr := e.agents.Get(agentsdk.AgentReceipt)
	if rcptErr != nil {
		e.logger.Warn("receipt agent not registered, skipping", "error", rcptErr)
	}
	_ = rcptErr
	if receiptAgent != nil {
		stepRecords := e.buildStepRecords(allAccum)
		receiptInputs, _ := json.Marshal(map[string]any{
			"step_results": stepRecords,
			"evidence_ids": allEvidenceIDs,
			"outcome":      "completed",
			"scope_snapshot": receipt.ScopeSnapshot{
				TargetAllowlist:   policyCtx.TargetAllowlist,
				TargetExclusions:  policyCtx.TargetExclusions,
				AllowedTiers:      policyCtx.AllowedTiers,
				AllowedTechniques: eng.AllowedTechniques,
				RateLimit:         policyCtx.RateLimit,
				ConcurrencyCap:    policyCtx.ConcurrencyCap,
			},
		})
		_, err := receiptAgent.HandleTask(ctx, &agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        run.ID,
			EngagementID: eng.ID,
			OrgID:        eng.OrgID,
			Action:       "generate_receipt",
			Inputs:       receiptInputs,
			CreatedAt:    runStart,
		})
		if err != nil {
			e.logger.Error("receipt agent failed", "error", err)
		}
	}
}

// resolveConnectors maps engagement connector IDs to category-keyed IDs for agents.
func (e *RunEngine) resolveConnectors(ctx context.Context, eng *models.Engagement) map[string]string {
	result := map[string]string{}
	if e.connectorSvc == nil || len(eng.ConnectorIDs) == 0 {
		return result
	}

	// Look up each connector's category by resolving via the service
	for _, catStr := range []string{"siem", "edr", "itsm", "notification"} {
		ids, err := e.connectorSvc.ListByCategory(ctx, eng.OrgID, catStr)
		if err != nil {
			e.logger.Warn("failed to list connectors by category", "category", catStr, "error", err)
			continue
		}
		// Find the first connector in this category that's also in the engagement's connector list
		for _, cid := range ids {
			for _, engCID := range eng.ConnectorIDs {
				if cid == engCID {
					result[catStr+"_connector_id"] = cid.String()
					break
				}
			}
			if _, found := result[catStr+"_connector_id"]; found {
				break
			}
		}
	}

	e.logger.Info("resolved engagement connectors", "connectors", result)
	return result
}

// preCoverageState captures a technique's coverage before a run for drift detection.
type preCoverageState struct {
	HadTelemetry bool `json:"had_telemetry"`
	HadDetection bool `json:"had_detection"`
	HadAlert     bool `json:"had_alert"`
}

// snapshotCoverage captures the current coverage state for drift comparison.
func (e *RunEngine) snapshotCoverage(ctx context.Context, orgID uuid.UUID) map[string]preCoverageState {
	result := map[string]preCoverageState{}
	if e.coverageRepo == nil {
		return result
	}

	entries, err := e.coverageRepo.ListByOrgID(ctx, orgID)
	if err != nil {
		e.logger.Warn("failed to snapshot coverage for drift detection", "error", err)
		return result
	}

	for _, entry := range entries {
		result[entry.TechniqueID] = preCoverageState{
			HadTelemetry: entry.HasTelemetry,
			HadDetection: entry.HasDetection,
			HadAlert:     entry.HasAlert,
		}
	}

	return result
}

// buildStepRecords converts accumulated step data into receipt step records.
func (e *RunEngine) buildStepRecords(allAccum []stepAccum) []receipt.StepRecord {
	var records []receipt.StepRecord
	for _, a := range allAccum {
		records = append(records, receipt.StepRecord{
			StepNumber:   a.StepNumber,
			AgentType:    a.Action,
			Action:       a.Action,
			Tier:         a.Tier,
			Status:       a.Status,
			StartedAt:    a.StartedAt,
			CompletedAt:  a.CompletedAt,
			EvidenceIDs:  a.EvidenceIDs,
			CleanupDone:  a.CleanupDone,
			ErrorMessage: a.Error,
		})
	}
	return records
}

// createFindings persists multiple agent findings to the database.
func (e *RunEngine) createFindings(ctx context.Context, eng *models.Engagement, run *models.Run, stepRecord *models.RunStep, findings []agentsdk.FindingOutput) {
	for _, f := range findings {
		e.createSingleFinding(ctx, eng, run, stepRecord, f)
	}
}

// createSingleFinding persists a single finding to the database.
func (e *RunEngine) createSingleFinding(ctx context.Context, eng *models.Engagement, run *models.Run, stepRecord *models.RunStep, f agentsdk.FindingOutput) {
	finding := &models.Finding{
		OrgID:          eng.OrgID,
		RunID:          &run.ID,
		Title:          f.Title,
		Description:    &f.Description,
		Severity:       models.Severity(f.Severity),
		Confidence:     models.Confidence(f.Confidence),
		Status:         models.FindingObserved,
		TechniqueIDs:   f.TechniqueIDs,
		EvidenceIDs:    f.EvidenceIDs,
		Remediation:    &f.Remediation,
		Metadata:       json.RawMessage(`{}`),
	}
	if stepRecord != nil {
		finding.RunStepID = &stepRecord.ID
	}
	if finding.TechniqueIDs == nil {
		finding.TechniqueIDs = []string{}
	}
	if finding.EvidenceIDs == nil {
		finding.EvidenceIDs = []string{}
	}
	if finding.AffectedAssets == nil {
		finding.AffectedAssets = []uuid.UUID{}
	}
	if err := e.findings.Create(ctx, finding); err != nil {
		e.logger.Error("creating finding", "error", err, "title", f.Title)
	}
}

func (e *RunEngine) failRun(ctx context.Context, runID uuid.UUID, reason string) {
	e.logger.Error("run failed", "run_id", runID, "reason", reason)
	if err := e.runs.UpdateStatus(ctx, runID, models.RunFailed); err != nil {
		e.logger.Error("updating run status to failed", "error", err, "run_id", runID)
	}
}
