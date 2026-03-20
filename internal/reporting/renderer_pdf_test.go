package reporting

import (
	"testing"

	"github.com/alokemajumder/AegisClaw/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPDF_Empty(t *testing.T) {
	cfg := ReportConfig{
		Title:  "Empty Report",
		Type:   ReportExecutive,
		Format: "pdf",
	}
	data := &ReportData{
		FindingsBySev: make(map[string]int),
	}

	out, err := RenderPDF(cfg, data)
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	assert.True(t, len(out) > 4, "PDF output should be more than 4 bytes")
	assert.Equal(t, "%PDF", string(out[:4]), "output should start with PDF magic bytes")
}

func TestRenderPDF_Executive(t *testing.T) {
	cfg := ReportConfig{
		Title:  "Q1 Security Report",
		Type:   ReportExecutive,
		Format: "pdf",
	}
	remediation := "Patch the vulnerability"
	data := &ReportData{
		TotalAssets:   10,
		CompletedRuns: 5,
		FailedRuns:    1,
		Findings: []models.Finding{
			{
				Title:        "SQL Injection in API",
				Severity:     "critical",
				Confidence:   "high",
				TechniqueIDs: []string{"T1190"},
				Remediation:  &remediation,
			},
			{
				Title:      "Weak TLS Config",
				Severity:   "medium",
				Confidence: "medium",
			},
		},
		FindingsBySev: map[string]int{
			"critical": 1,
			"medium":   1,
		},
		CoverageEntries: []models.CoverageEntry{
			{TechniqueID: "T1190", HasTelemetry: true, HasDetection: true, HasAlert: false},
			{TechniqueID: "T1059", HasTelemetry: true, HasDetection: false, HasAlert: false},
		},
		CoverageGaps: []models.CoverageEntry{
			{TechniqueID: "T1059", HasTelemetry: true, HasDetection: false, HasAlert: false},
		},
	}

	out, err := RenderPDF(cfg, data)
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	assert.Equal(t, "%PDF", string(out[:4]))
}

func TestRenderPDF_Technical(t *testing.T) {
	cfg := ReportConfig{
		Title:  "Technical Findings",
		Type:   ReportTechnical,
		Format: "pdf",
	}
	desc := "Detailed description of the finding"
	remediation := "Apply patches and rotate credentials immediately"
	data := &ReportData{
		CompletedRuns: 3,
		Findings: []models.Finding{
			{
				Title:        "Credential Exposure",
				Severity:     "high",
				Confidence:   "high",
				TechniqueIDs: []string{"T1552.001"},
				Description:  &desc,
				Remediation:  &remediation,
			},
		},
		FindingsBySev: map[string]int{
			"high": 1,
		},
	}

	out, err := RenderPDF(cfg, data)
	require.NoError(t, err)
	assert.Equal(t, "%PDF", string(out[:4]))
	assert.True(t, len(out) > 100, "PDF should contain substantial content")
}

func TestRenderPDF_Coverage(t *testing.T) {
	cfg := ReportConfig{
		Title:  "Coverage Matrix Report",
		Type:   ReportCoverage,
		Format: "pdf",
	}
	data := &ReportData{
		FindingsBySev: make(map[string]int),
		CoverageEntries: []models.CoverageEntry{
			{TechniqueID: "T1190", HasTelemetry: true, HasDetection: true, HasAlert: true},
			{TechniqueID: "T1059", HasTelemetry: true, HasDetection: false, HasAlert: false},
			{TechniqueID: "T1078", HasTelemetry: false, HasDetection: false, HasAlert: false},
		},
		CoverageGaps: []models.CoverageEntry{
			{TechniqueID: "T1059", HasTelemetry: true, HasDetection: false, HasAlert: false},
			{TechniqueID: "T1078", HasTelemetry: false, HasDetection: false, HasAlert: false},
		},
	}

	out, err := RenderPDF(cfg, data)
	require.NoError(t, err)
	assert.Equal(t, "%PDF", string(out[:4]))
}
