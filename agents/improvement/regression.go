package improvement

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// regressionTaskInput holds inputs for the regression tester.
type regressionTaskInput struct {
	// BaselineRunID is the run to compare against.
	BaselineRunID string `json:"baseline_run_id"`
	// CurrentRunID is the latest run to check for regressions.
	CurrentRunID string `json:"current_run_id"`
}

// RegressionAgent reruns relevant validations after changes (deployments, SIEM/EDR updates).
type RegressionAgent struct {
	logger      *slog.Logger
	deps        agentsdk.AgentDeps
	runRepo     *repository.RunRepo
	findingRepo *repository.FindingRepo
}

func NewRegressionAgent() *RegressionAgent {
	return &RegressionAgent{}
}

func (a *RegressionAgent) Name() agentsdk.AgentType { return agentsdk.AgentRegression }
func (a *RegressionAgent) Squad() agentsdk.Squad    { return agentsdk.SquadImprovement }

func (a *RegressionAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	if pool, ok := deps.DB.(*pgxpool.Pool); ok {
		a.runRepo = repository.NewRunRepo(pool)
		a.findingRepo = repository.NewFindingRepo(pool)
		a.logger.Info("regression agent initialized with DB")
	} else {
		a.logger.Warn("regression agent initialized without DB — regression detection will be simulated")
	}

	return nil
}

func (a *RegressionAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("regression agent checking for validation reruns",
		"task_id", task.ID,
	)

	// If no DB is available, return simulated results
	if a.runRepo == nil || a.findingRepo == nil {
		return a.handleSimulated(task)
	}

	// Parse task inputs for baseline and current run IDs
	var input regressionTaskInput
	if len(task.Inputs) > 0 {
		if err := json.Unmarshal(task.Inputs, &input); err != nil {
			a.logger.Warn("failed to parse regression task inputs — querying recent runs",
				"error", err,
				"task_id", task.ID,
			)
		}
	}

	// Query recent completed runs for this org to find baselines
	pagination := models.PaginationParams{Page: 1, PerPage: 10}
	recentRuns, _, err := a.runRepo.ListByOrgID(ctx, task.OrgID, pagination, string(models.RunCompleted))
	if err != nil {
		a.logger.Error("failed to query recent runs", "error", err)
		return a.handleSimulated(task)
	}

	if len(recentRuns) < 2 {
		a.logger.Info("not enough completed runs for regression comparison",
			"completed_runs", len(recentRuns),
		)
		outputs, _ := json.Marshal(map[string]any{
			"changes_detected":   0,
			"validations_queued": 0,
			"regressions_found":  0,
			"reason":             "insufficient completed runs for comparison",
		})
		return &agentsdk.Result{
			TaskID:      task.ID,
			Status:      agentsdk.StatusCompleted,
			Outputs:     outputs,
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Use the two most recent runs for comparison
	currentRun := recentRuns[0]
	baselineRun := recentRuns[1]

	a.logger.Info("comparing runs for regressions",
		"current_run", currentRun.ID,
		"baseline_run", baselineRun.ID,
	)

	// Get findings from both runs
	baselineFindings, err := a.findingRepo.ListByRunID(ctx, baselineRun.ID)
	if err != nil {
		a.logger.Error("failed to query baseline findings", "error", err)
		return a.handleSimulated(task)
	}

	currentFindings, err := a.findingRepo.ListByRunID(ctx, currentRun.ID)
	if err != nil {
		a.logger.Error("failed to query current findings", "error", err)
		return a.handleSimulated(task)
	}

	// Build a set of finding fingerprints from the baseline for comparison.
	// Use title + severity + technique IDs for more robust matching than title alone.
	baselineFingerprints := make(map[string]bool)
	for _, f := range baselineFindings {
		baselineFingerprints[findingFingerprint(f)] = true
	}

	currentFingerprints := make(map[string]bool)
	for _, f := range currentFindings {
		currentFingerprints[findingFingerprint(f)] = true
	}

	// Regressions: findings in current that were not in baseline
	var regressionFindings []agentsdk.FindingOutput
	for _, f := range currentFindings {
		fp := findingFingerprint(f)
		if !baselineFingerprints[fp] {
			regressionFindings = append(regressionFindings, agentsdk.FindingOutput{
				Title:       fmt.Sprintf("Regression: %s", f.Title),
				Description: fmt.Sprintf("Finding appeared in run %s but was not present in baseline run %s", currentRun.ID, baselineRun.ID),
				Severity:    string(f.Severity),
				Confidence:  string(f.Confidence),
				TechniqueIDs: f.TechniqueIDs,
				Remediation: "Investigate the regression and verify that previously passing controls are still effective",
			})
		}
	}

	// Count resolved findings: in baseline but not in current
	resolvedCount := 0
	for _, f := range baselineFindings {
		fp := findingFingerprint(f)
		if !currentFingerprints[fp] {
			resolvedCount++
		}
	}

	a.logger.Info("regression analysis complete",
		"regressions_found", len(regressionFindings),
		"resolved", resolvedCount,
		"baseline_findings", len(baselineFindings),
		"current_findings", len(currentFindings),
	)

	outputs, _ := json.Marshal(map[string]any{
		"baseline_run_id":    baselineRun.ID,
		"current_run_id":     currentRun.ID,
		"baseline_findings":  len(baselineFindings),
		"current_findings":   len(currentFindings),
		"regressions_found":  len(regressionFindings),
		"resolved_findings":  resolvedCount,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		Findings:    regressionFindings,
		CompletedAt: time.Now().UTC(),
	}, nil
}

// handleSimulated returns simulated regression data when DB is unavailable.
func (a *RegressionAgent) handleSimulated(task *agentsdk.Task) (*agentsdk.Result, error) {
	outputs, _ := json.Marshal(map[string]any{
		"changes_detected":   0,
		"validations_queued": 0,
		"regressions_found":  0,
		"no_data":            true,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *RegressionAgent) Shutdown(_ context.Context) error {
	a.logger.Info("regression agent shutting down")
	return nil
}

// findingFingerprint creates a stable fingerprint for a finding using title,
// severity, and technique IDs. This is more robust than title-only matching
// which can be spoofed or can miss findings that change severity.
func findingFingerprint(f models.Finding) string {
	techniques := make([]string, len(f.TechniqueIDs))
	copy(techniques, f.TechniqueIDs)
	sort.Strings(techniques)
	return fmt.Sprintf("%s|%s|%s", f.Title, f.Severity, strings.Join(techniques, ","))
}
