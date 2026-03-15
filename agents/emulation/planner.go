package emulation

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/playbook"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// PlannerAgent selects validations based on asset type, allowed tiers, and loaded playbooks.
type PlannerAgent struct {
	logger    *slog.Logger
	deps      agentsdk.AgentDeps
	playbooks []playbook.Playbook
}

func NewPlannerAgent() *PlannerAgent {
	return &PlannerAgent{}
}

func (a *PlannerAgent) Name() agentsdk.AgentType { return agentsdk.AgentPlanner }
func (a *PlannerAgent) Squad() agentsdk.Squad    { return agentsdk.SquadEmulation }

func (a *PlannerAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	// Load playbooks from PlaybookLoader dependency
	if loader, ok := deps.PlaybookLoader.(*playbook.Loader); ok {
		playbookDir := deps.PlaybookDir
		if playbookDir == "" {
			playbookDir = "./playbooks"
		}
		pbs, err := loader.LoadAll(playbookDir)
		if err != nil {
			a.logger.Warn("failed to load playbooks, will use fallback plan", "error", err, "dir", playbookDir)
		} else {
			a.playbooks = pbs
			a.logger.Info("planner loaded playbooks", "count", len(pbs), "dir", playbookDir)
		}
	} else {
		a.logger.Warn("no playbook loader available, will use fallback plan")
	}

	a.logger.Info("planner agent initialized")
	return nil
}

func (a *PlannerAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("planner generating validation campaign",
		"task_id", task.ID,
		"engagement_id", task.EngagementID,
	)

	var steps []agentsdk.Task

	if len(a.playbooks) > 0 && task.PolicyContext != nil {
		steps = a.planFromPlaybooks(task)
	}

	// Fallback to default steps if no playbooks matched
	if len(steps) == 0 {
		steps = a.fallbackPlan(task)
	}

	outputs, _ := json.Marshal(map[string]any{
		"planned_steps": len(steps),
		"source":        a.planSource(steps),
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		NextSteps:   steps,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *PlannerAgent) planFromPlaybooks(task *agentsdk.Task) []agentsdk.Task {
	// Filter playbooks by allowed tiers
	filtered := playbook.FilterByTier(a.playbooks, task.PolicyContext.AllowedTiers)
	if len(filtered) == 0 {
		a.logger.Info("no playbooks match allowed tiers", "tiers", task.PolicyContext.AllowedTiers)
		return nil
	}

	var steps []agentsdk.Task
	stepNum := 1

	for _, pb := range filtered {
		for _, pbStep := range pb.Steps {
			inputsData, _ := json.Marshal(map[string]any{
				"playbook_id":   pb.ID,
				"playbook_name": pb.Name,
				"step_name":     pbStep.Name,
				"action":        pbStep.Action,
				"technique_id":  pb.TechniqueID,
				"inputs":        pbStep.Inputs,
			})

			steps = append(steps, agentsdk.Task{
				ID:           uuid.New().String(),
				RunID:        task.RunID,
				EngagementID: task.EngagementID,
				OrgID:        task.OrgID,
				StepNumber:   stepNum,
				Action:       pbStep.Action,
				Tier:         pb.Tier,
				Inputs:       inputsData,
				CreatedAt:    time.Now().UTC(),
			})
			stepNum++
		}
	}

	a.logger.Info("planned steps from playbooks",
		"playbooks_matched", len(filtered),
		"steps_generated", len(steps),
	)
	return steps
}

func (a *PlannerAgent) fallbackPlan(task *agentsdk.Task) []agentsdk.Task {
	a.logger.Info("using fallback validation plan")

	// Define all default steps with their required tiers
	type defaultStep struct {
		action string
		tier   int
	}
	defaults := []defaultStep{
		{"query_telemetry", 0},
		{"check_edr_agents", 0},
		{"drop_marker_file", 1},
	}

	// SECURITY: Filter fallback steps by allowed tiers to prevent generating
	// steps that the engagement is not authorized to run.
	allowedTiers := map[int]bool{}
	if task.PolicyContext != nil {
		for _, t := range task.PolicyContext.AllowedTiers {
			allowedTiers[t] = true
		}
	}

	var steps []agentsdk.Task
	stepNum := 1
	for _, d := range defaults {
		// If PolicyContext is available, only include steps with allowed tiers
		if task.PolicyContext != nil && !allowedTiers[d.tier] {
			a.logger.Info("fallback step skipped: tier not allowed", "action", d.action, "tier", d.tier)
			continue
		}
		steps = append(steps, agentsdk.Task{
			ID:           uuid.New().String(),
			RunID:        task.RunID,
			EngagementID: task.EngagementID,
			OrgID:        task.OrgID,
			StepNumber:   stepNum,
			Action:       d.action,
			Tier:         d.tier,
			CreatedAt:    time.Now().UTC(),
		})
		stepNum++
	}
	return steps
}

func (a *PlannerAgent) planSource(steps []agentsdk.Task) string {
	if len(a.playbooks) > 0 {
		return "playbooks"
	}
	return "fallback"
}

func (a *PlannerAgent) Shutdown(_ context.Context) error {
	a.logger.Info("planner agent shutting down")
	return nil
}
