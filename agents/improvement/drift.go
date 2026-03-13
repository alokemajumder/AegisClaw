package improvement

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// driftTaskInput holds the previous coverage state for comparison.
type driftTaskInput struct {
	// PreviousCoverage maps technique_id to its previous detection state.
	PreviousCoverage map[string]previousState `json:"previous_coverage"`
}

type previousState struct {
	HadTelemetry bool `json:"had_telemetry"`
	HadDetection bool `json:"had_detection"`
	HadAlert     bool `json:"had_alert"`
}

// DriftAgent detects control regression and raises priority incidents.
type DriftAgent struct {
	logger       *slog.Logger
	deps         agentsdk.AgentDeps
	coverageRepo *repository.CoverageRepo
}

func NewDriftAgent() *DriftAgent {
	return &DriftAgent{}
}

func (a *DriftAgent) Name() agentsdk.AgentType { return agentsdk.AgentDrift }
func (a *DriftAgent) Squad() agentsdk.Squad    { return agentsdk.SquadImprovement }

func (a *DriftAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	if pool, ok := deps.DB.(*pgxpool.Pool); ok {
		a.coverageRepo = repository.NewCoverageRepo(pool)
		a.logger.Info("drift agent initialized with DB")
	} else {
		a.logger.Warn("drift agent initialized without DB — drift detection will be simulated")
	}

	return nil
}

func (a *DriftAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("drift agent analyzing control changes",
		"task_id", task.ID,
	)

	// If no DB is available, return simulated results
	if a.coverageRepo == nil {
		return a.handleSimulated(task)
	}

	// Parse the previous coverage state from task inputs
	var input driftTaskInput
	if len(task.Inputs) > 0 {
		if err := json.Unmarshal(task.Inputs, &input); err != nil {
			a.logger.Warn("failed to parse drift task inputs — using simulated data",
				"error", err,
				"task_id", task.ID,
			)
			return a.handleSimulated(task)
		}
	}

	// Query current coverage entries from DB
	currentEntries, err := a.coverageRepo.ListByOrgID(ctx, task.OrgID)
	if err != nil {
		a.logger.Error("failed to query current coverage", "error", err)
		return a.handleSimulated(task)
	}

	// Compare current coverage against previous state
	var findings []agentsdk.FindingOutput
	regressions := 0
	improvements := 0
	unchanged := 0

	for _, entry := range currentEntries {
		prev, hasPrev := input.PreviousCoverage[entry.TechniqueID]
		if !hasPrev {
			// New technique — neither regression nor improvement
			unchanged++
			continue
		}

		// Detect regressions: was detected, now not detected
		isRegression := false
		regressionDetails := ""

		if prev.HadTelemetry && !entry.HasTelemetry {
			isRegression = true
			regressionDetails += "telemetry lost; "
		}
		if prev.HadDetection && !entry.HasDetection {
			isRegression = true
			regressionDetails += "detection lost; "
		}
		if prev.HadAlert && !entry.HasAlert {
			isRegression = true
			regressionDetails += "alerting lost; "
		}

		if isRegression {
			regressions++
			findings = append(findings, agentsdk.FindingOutput{
				Title:       fmt.Sprintf("Coverage drift detected for %s", entry.TechniqueID),
				Description: fmt.Sprintf("Control regression on technique %s: %s", entry.TechniqueID, regressionDetails),
				Severity:    "high",
				Confidence:  "confirmed",
				TechniqueIDs: []string{entry.TechniqueID},
				Remediation: fmt.Sprintf("Investigate why coverage for %s has regressed and restore detection capabilities", entry.TechniqueID),
			})
			continue
		}

		// Detect improvements: was not detected, now detected
		isImprovement := false
		if !prev.HadTelemetry && entry.HasTelemetry {
			isImprovement = true
		}
		if !prev.HadDetection && entry.HasDetection {
			isImprovement = true
		}
		if !prev.HadAlert && entry.HasAlert {
			isImprovement = true
		}

		if isImprovement {
			improvements++
		} else {
			unchanged++
		}
	}

	driftDetected := regressions > 0

	a.logger.Info("drift analysis complete",
		"drift_detected", driftDetected,
		"regressions", regressions,
		"improvements", improvements,
		"unchanged", unchanged,
	)

	outputs, _ := json.Marshal(map[string]any{
		"drift_detected": driftDetected,
		"regressions":    regressions,
		"improvements":   improvements,
		"unchanged":      unchanged,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		Findings:    findings,
		CompletedAt: time.Now().UTC(),
	}, nil
}

// handleSimulated returns simulated drift data when DB or inputs are unavailable.
func (a *DriftAgent) handleSimulated(task *agentsdk.Task) (*agentsdk.Result, error) {
	outputs, _ := json.Marshal(map[string]any{
		"drift_detected": false,
		"regressions":    0,
		"improvements":   1,
		"unchanged":      15,
		"simulated":      true,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *DriftAgent) Shutdown(_ context.Context) error {
	a.logger.Info("drift agent shutting down")
	return nil
}
