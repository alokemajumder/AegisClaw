package reporting

import (
	"context"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
)

// ReportData holds aggregated data for report generation.
type ReportData struct {
	Findings          []models.Finding
	FindingsBySev     map[string]int
	Runs              []models.Run
	CompletedRuns     int
	FailedRuns        int
	CoverageEntries   []models.CoverageEntry
	CoverageGaps      []models.CoverageEntry
	TotalAssets       int
}

// GatherData collects all data needed for report generation.
func GatherData(ctx context.Context, orgID uuid.UUID, findings *repository.FindingRepo, runs *repository.RunRepo, coverage *repository.CoverageRepo, assets *repository.AssetRepo) (*ReportData, error) {
	data := &ReportData{
		FindingsBySev: make(map[string]int),
	}

	// Findings
	allFindings, _, err := findings.ListByOrgID(ctx, orgID, models.PaginationParams{Page: 1, PerPage: 1000}, "", "")
	if err == nil {
		data.Findings = allFindings
		for _, f := range allFindings {
			data.FindingsBySev[string(f.Severity)]++
		}
	}

	// Runs
	allRuns, _, err := runs.ListByOrgID(ctx, orgID, models.PaginationParams{Page: 1, PerPage: 1000}, "")
	if err == nil {
		data.Runs = allRuns
		for _, r := range allRuns {
			switch r.Status {
			case models.RunCompleted:
				data.CompletedRuns++
			case models.RunFailed:
				data.FailedRuns++
			}
		}
	}

	// Coverage
	entries, err := coverage.ListByOrgID(ctx, orgID)
	if err == nil {
		data.CoverageEntries = entries
	}
	gaps, err := coverage.GetGaps(ctx, orgID)
	if err == nil {
		data.CoverageGaps = gaps
	}

	// Assets
	count, err := assets.CountByOrgID(ctx, orgID)
	if err == nil {
		data.TotalAssets = count
	}

	return data, nil
}
