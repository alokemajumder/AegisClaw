package improvement

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
)

// techniqueResult represents parsed validation results for a single technique.
type techniqueResult struct {
	TechniqueID  string `json:"technique_id"`
	HasTelemetry bool   `json:"has_telemetry"`
	HasDetection bool   `json:"has_detection"`
	HasAlert     bool   `json:"has_alert"`
}

// coverageTaskInput is the expected structure of task.Inputs for coverage mapping.
type coverageTaskInput struct {
	TechniqueResults []techniqueResult `json:"technique_results"`
}

// CoverageMapperAgent maintains the ATT&CK x Asset x Telemetry coverage matrix.
type CoverageMapperAgent struct {
	logger       *slog.Logger
	deps         agentsdk.AgentDeps
	coverageRepo *repository.CoverageRepo
}

func NewCoverageMapperAgent() *CoverageMapperAgent {
	return &CoverageMapperAgent{}
}

func (a *CoverageMapperAgent) Name() agentsdk.AgentType { return agentsdk.AgentCoverageMapper }
func (a *CoverageMapperAgent) Squad() agentsdk.Squad    { return agentsdk.SquadImprovement }

func (a *CoverageMapperAgent) Init(_ context.Context, deps agentsdk.AgentDeps) error {
	a.deps = deps
	if l, ok := deps.Logger.(*slog.Logger); ok {
		a.logger = l
	} else {
		a.logger = slog.Default()
	}

	if pool, ok := deps.DB.(*pgxpool.Pool); ok {
		a.coverageRepo = repository.NewCoverageRepo(pool)
		a.logger.Info("coverage mapper agent initialized with DB")
	} else {
		a.logger.Warn("coverage mapper agent initialized without DB — coverage data will be simulated")
	}

	return nil
}

func (a *CoverageMapperAgent) HandleTask(ctx context.Context, task *agentsdk.Task) (*agentsdk.Result, error) {
	a.logger.Info("coverage mapper updating matrix",
		"task_id", task.ID,
		"run_id", task.RunID,
	)

	// If no DB is available, return simulated results
	if a.coverageRepo == nil {
		return a.handleSimulated(task)
	}

	// Parse task inputs to get technique validation results
	var input coverageTaskInput
	if len(task.Inputs) > 0 {
		if err := json.Unmarshal(task.Inputs, &input); err != nil {
			a.logger.Warn("failed to parse task inputs — using simulated data",
				"error", err,
				"task_id", task.ID,
			)
			return a.handleSimulated(task)
		}
	}

	// If no technique results provided, use simulated data
	if len(input.TechniqueResults) == 0 {
		a.logger.Warn("no technique results in task inputs — using simulated data",
			"task_id", task.ID,
		)
		return a.handleSimulated(task)
	}

	// Upsert coverage entries for each technique validated
	techniquesUpdated := 0
	blindSpots := 0
	now := time.Now().UTC()

	for _, tr := range input.TechniqueResults {
		entry := &models.CoverageEntry{
			OrgID:           task.OrgID,
			TechniqueID:     tr.TechniqueID,
			HasTelemetry:    tr.HasTelemetry,
			HasDetection:    tr.HasDetection,
			HasAlert:        tr.HasAlert,
			LastValidatedAt: &now,
			LastRunID:       uuidPtr(task.RunID),
		}

		if err := a.coverageRepo.Upsert(ctx, entry); err != nil {
			a.logger.Error("failed to upsert coverage entry",
				"error", err,
				"technique_id", tr.TechniqueID,
				"task_id", task.ID,
			)
			continue
		}
		techniquesUpdated++

		// Count blind spots: has telemetry but no detection, or no telemetry at all
		if !tr.HasTelemetry || (tr.HasTelemetry && !tr.HasDetection) {
			blindSpots++
		}
	}

	// Query gaps for overall coverage percentage
	allEntries, err := a.coverageRepo.ListByOrgID(ctx, task.OrgID)
	if err != nil {
		a.logger.Error("failed to query coverage entries", "error", err)
	}

	gaps, err := a.coverageRepo.GetGaps(ctx, task.OrgID)
	if err != nil {
		a.logger.Error("failed to query coverage gaps", "error", err)
	}

	coveragePct := 0.0
	if len(allEntries) > 0 {
		covered := len(allEntries) - len(gaps)
		coveragePct = float64(covered) / float64(len(allEntries)) * 100.0
	}

	a.logger.Info("coverage matrix updated",
		"techniques_updated", techniquesUpdated,
		"blind_spots", blindSpots,
		"total_entries", len(allEntries),
		"gaps", len(gaps),
		"coverage_pct", coveragePct,
	)

	outputs, _ := json.Marshal(map[string]any{
		"techniques_updated": techniquesUpdated,
		"blind_spots_found":  blindSpots,
		"total_entries":      len(allEntries),
		"gaps":               len(gaps),
		"coverage_pct":       coveragePct,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

// handleSimulated returns simulated coverage data when DB or inputs are unavailable.
func (a *CoverageMapperAgent) handleSimulated(task *agentsdk.Task) (*agentsdk.Result, error) {
	outputs, _ := json.Marshal(map[string]any{
		"techniques_updated": 5,
		"blind_spots_found":  1,
		"coverage_pct":       78.5,
		"simulated":          true,
	})

	return &agentsdk.Result{
		TaskID:      task.ID,
		Status:      agentsdk.StatusCompleted,
		Outputs:     outputs,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (a *CoverageMapperAgent) Shutdown(_ context.Context) error {
	a.logger.Info("coverage mapper agent shutting down")
	return nil
}

// uuidPtr is a helper to create a pointer to a UUID.
func uuidPtr(id uuid.UUID) *uuid.UUID {
	return &id
}
