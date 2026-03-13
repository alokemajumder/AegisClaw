package reporting

import (
	"encoding/json"
	"time"
)

// RenderJSON generates a structured JSON report.
func RenderJSON(cfg ReportConfig, data *ReportData) ([]byte, error) {
	report := map[string]any{
		"title":        cfg.Title,
		"type":         cfg.Type,
		"generated_at": time.Now().UTC(),
		"summary": map[string]any{
			"total_assets":      data.TotalAssets,
			"completed_runs":    data.CompletedRuns,
			"failed_runs":       data.FailedRuns,
			"total_findings":    len(data.Findings),
			"findings_by_severity": data.FindingsBySev,
			"coverage_entries":  len(data.CoverageEntries),
			"coverage_gaps":     len(data.CoverageGaps),
		},
		"findings": data.Findings,
		"runs":     data.Runs,
	}

	return json.MarshalIndent(report, "", "  ")
}
