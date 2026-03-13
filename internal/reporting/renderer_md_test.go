package reporting

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

func TestRenderMarkdown_Executive(t *testing.T) {
	desc := "SQL injection in login endpoint"
	data := &ReportData{
		TotalAssets:   10,
		CompletedRuns: 5,
		Findings: []models.Finding{
			{ID: uuid.New(), Title: "SQLi in Login", Severity: "critical", Description: &desc},
			{ID: uuid.New(), Title: "XSS in Search", Severity: "high"},
		},
		FindingsBySev: map[string]int{
			"critical": 1,
			"high":     1,
		},
		CoverageEntries: []models.CoverageEntry{
			{TechniqueID: "T1059"},
		},
		CoverageGaps: []models.CoverageEntry{
			{TechniqueID: "T1055"},
		},
	}

	cfg := ReportConfig{Type: ReportExecutive, Title: "Q1 Security Report"}
	result := RenderMarkdown(cfg, data)

	assert.Contains(t, result, "# Q1 Security Report")
	assert.Contains(t, result, "**Report Type:** executive")
	assert.Contains(t, result, "## Executive Summary")
	assert.Contains(t, result, "**Total Assets Under Management:** 10")
	assert.Contains(t, result, "**Validation Runs Completed:** 5")
	assert.Contains(t, result, "**Total Findings:** 2")
	assert.Contains(t, result, "| critical | 1 |")
	assert.Contains(t, result, "| high | 1 |")
	assert.Contains(t, result, "**URGENT:** Address critical findings immediately")
	assert.Contains(t, result, "Prioritize remediation of high-severity findings")
	assert.Contains(t, result, "Improve detection coverage for identified gaps")
}

func TestRenderMarkdown_Technical(t *testing.T) {
	desc := "Found SQL injection vulnerability"
	remed := "Use parameterized queries"
	now := time.Now()
	runID := uuid.New()

	data := &ReportData{
		Findings: []models.Finding{
			{
				ID:           uuid.New(),
				Title:        "SQLi Vulnerability",
				Severity:     "critical",
				Confidence:   "high",
				Status:       "confirmed",
				TechniqueIDs: []string{"T1190", "T1059"},
				Description:  &desc,
				Remediation:  &remed,
			},
		},
		FindingsBySev: map[string]int{"critical": 1},
		Runs: []models.Run{
			{
				ID:             runID,
				Status:         "completed",
				Tier:           1,
				StepsCompleted: 5,
				StepsTotal:     5,
				StartedAt:      &now,
			},
		},
	}

	cfg := ReportConfig{Type: ReportTechnical, Title: "Technical Report"}
	result := RenderMarkdown(cfg, data)

	assert.Contains(t, result, "## Technical Findings Detail")
	assert.Contains(t, result, "### 1. SQLi Vulnerability")
	assert.Contains(t, result, "**Severity:** critical")
	assert.Contains(t, result, "**Confidence:** high")
	assert.Contains(t, result, "**Techniques:** T1190, T1059")
	assert.Contains(t, result, "Found SQL injection vulnerability")
	assert.Contains(t, result, "**Remediation:** Use parameterized queries")
	assert.Contains(t, result, "## Run History")
	assert.Contains(t, result, runID.String()[:8])
}

func TestRenderMarkdown_Coverage(t *testing.T) {
	data := &ReportData{
		FindingsBySev: map[string]int{},
		CoverageEntries: []models.CoverageEntry{
			{TechniqueID: "T1059", HasTelemetry: true, HasDetection: true, HasAlert: true},
			{TechniqueID: "T1055", HasTelemetry: true, HasDetection: false, HasAlert: false},
		},
		CoverageGaps: []models.CoverageEntry{
			{TechniqueID: "T1055", HasTelemetry: true, HasDetection: false, HasAlert: false},
		},
	}

	cfg := ReportConfig{Type: ReportCoverage, Title: "Coverage Report"}
	result := RenderMarkdown(cfg, data)

	assert.Contains(t, result, "## Coverage Matrix")
	assert.Contains(t, result, "| T1059 | Yes | Yes | Yes |")
	assert.Contains(t, result, "| T1055 | Yes | No | No |")
	assert.Contains(t, result, "## Coverage Gaps")
	assert.Contains(t, result, "**T1055**: Missing detection, alert")
}

func TestRenderMarkdown_DefaultFallsToExecutive(t *testing.T) {
	data := &ReportData{
		FindingsBySev: map[string]int{},
	}

	cfg := ReportConfig{Type: "unknown", Title: "Fallback"}
	result := RenderMarkdown(cfg, data)

	assert.Contains(t, result, "## Executive Summary")
}

func TestRenderMarkdown_NoRecommendations(t *testing.T) {
	data := &ReportData{
		FindingsBySev: map[string]int{
			"low": 1,
		},
	}

	cfg := ReportConfig{Type: ReportExecutive, Title: "Mild Report"}
	result := RenderMarkdown(cfg, data)

	assert.NotContains(t, result, "URGENT")
	assert.NotContains(t, result, "Prioritize remediation")
	assert.Contains(t, result, "Continue regular validation runs")
}

func TestBoolIcon(t *testing.T) {
	assert.Equal(t, "Yes", boolIcon(true))
	assert.Equal(t, "No", boolIcon(false))
}

func TestRenderMarkdown_EmptyData(t *testing.T) {
	data := &ReportData{
		FindingsBySev: map[string]int{},
	}

	cfg := ReportConfig{Type: ReportTechnical, Title: "Empty Report"}
	result := RenderMarkdown(cfg, data)

	assert.Contains(t, result, "# Empty Report")
	assert.Contains(t, result, "## Technical Findings Detail")
	// Should have run history header even with no runs
	assert.Contains(t, result, "## Run History")
	// Should not have any findings entries
	assert.Equal(t, 0, strings.Count(result, "### "))
}
