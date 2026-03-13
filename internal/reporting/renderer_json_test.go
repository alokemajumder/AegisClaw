package reporting

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func TestRenderJSON_BasicOutput(t *testing.T) {
	data := &ReportData{
		TotalAssets:   5,
		CompletedRuns: 3,
		FailedRuns:    1,
		Findings: []models.Finding{
			{ID: uuid.New(), Title: "Test Finding", Severity: "high"},
		},
		FindingsBySev: map[string]int{"high": 1},
		CoverageEntries: []models.CoverageEntry{
			{TechniqueID: "T1059"},
		},
		CoverageGaps: []models.CoverageEntry{},
		Runs: []models.Run{
			{ID: uuid.New(), Status: "completed"},
		},
	}

	cfg := ReportConfig{Type: ReportExecutive, Title: "JSON Test"}
	out, err := RenderJSON(cfg, data)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))

	assert.Equal(t, "JSON Test", parsed["title"])
	assert.Equal(t, "executive", parsed["type"])
	assert.Contains(t, parsed, "generated_at")
	assert.Contains(t, parsed, "summary")
	assert.Contains(t, parsed, "findings")
	assert.Contains(t, parsed, "runs")

	summary := parsed["summary"].(map[string]any)
	assert.EqualValues(t, 5, summary["total_assets"])
	assert.EqualValues(t, 3, summary["completed_runs"])
	assert.EqualValues(t, 1, summary["failed_runs"])
	assert.EqualValues(t, 1, summary["total_findings"])
	assert.EqualValues(t, 1, summary["coverage_entries"])
	assert.EqualValues(t, 0, summary["coverage_gaps"])
}

func TestRenderJSON_EmptyData(t *testing.T) {
	data := &ReportData{
		FindingsBySev: map[string]int{},
	}

	cfg := ReportConfig{Type: ReportTechnical, Title: "Empty"}
	out, err := RenderJSON(cfg, data)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(out, &parsed))

	summary := parsed["summary"].(map[string]any)
	assert.EqualValues(t, 0, summary["total_findings"])
	assert.EqualValues(t, 0, summary["completed_runs"])
}

func TestRenderJSON_ValidJSON(t *testing.T) {
	data := &ReportData{
		TotalAssets:   2,
		CompletedRuns: 1,
		Findings: []models.Finding{
			{ID: uuid.New(), Title: "Finding 1", Severity: "low"},
			{ID: uuid.New(), Title: "Finding 2", Severity: "medium"},
		},
		FindingsBySev: map[string]int{"low": 1, "medium": 1},
		Runs:          []models.Run{},
	}

	cfg := ReportConfig{Type: ReportCoverage, Title: "Validation Test"}
	out, err := RenderJSON(cfg, data)
	require.NoError(t, err)

	// Verify the JSON is valid and can be re-marshaled
	assert.True(t, json.Valid(out))
}
