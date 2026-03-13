package reporting

import (
	"fmt"
	"strings"
	"time"
)

// RenderMarkdown generates a markdown report from gathered data.
func RenderMarkdown(cfg ReportConfig, data *ReportData) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", cfg.Title))
	b.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Report Type:** %s\n\n", cfg.Type))
	b.WriteString("---\n\n")

	switch cfg.Type {
	case ReportExecutive:
		renderExecutive(&b, data)
	case ReportTechnical:
		renderTechnical(&b, data)
	case ReportCoverage:
		renderCoverageReport(&b, data)
	default:
		renderExecutive(&b, data)
	}

	return b.String()
}

func renderExecutive(b *strings.Builder, data *ReportData) {
	b.WriteString("## Executive Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Total Assets Under Management:** %d\n", data.TotalAssets))
	b.WriteString(fmt.Sprintf("- **Validation Runs Completed:** %d\n", data.CompletedRuns))
	b.WriteString(fmt.Sprintf("- **Total Findings:** %d\n", len(data.Findings)))
	b.WriteString("\n")

	b.WriteString("## Findings by Severity\n\n")
	b.WriteString("| Severity | Count |\n")
	b.WriteString("|----------|-------|\n")
	for _, sev := range []string{"critical", "high", "medium", "low", "informational"} {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", sev, data.FindingsBySev[sev]))
	}
	b.WriteString("\n")

	b.WriteString("## Coverage Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Total Coverage Entries:** %d\n", len(data.CoverageEntries)))
	b.WriteString(fmt.Sprintf("- **Coverage Gaps:** %d\n", len(data.CoverageGaps)))
	b.WriteString("\n")

	b.WriteString("## Recommendations\n\n")
	if data.FindingsBySev["critical"] > 0 {
		b.WriteString("- **URGENT:** Address critical findings immediately\n")
	}
	if data.FindingsBySev["high"] > 0 {
		b.WriteString("- Prioritize remediation of high-severity findings\n")
	}
	if len(data.CoverageGaps) > 0 {
		b.WriteString("- Improve detection coverage for identified gaps\n")
	}
	b.WriteString("- Continue regular validation runs to maintain security posture\n")
}

func renderTechnical(b *strings.Builder, data *ReportData) {
	b.WriteString("## Technical Findings Detail\n\n")
	for i, f := range data.Findings {
		b.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, f.Title))
		b.WriteString(fmt.Sprintf("- **Severity:** %s\n", f.Severity))
		b.WriteString(fmt.Sprintf("- **Confidence:** %s\n", f.Confidence))
		b.WriteString(fmt.Sprintf("- **Status:** %s\n", f.Status))
		if len(f.TechniqueIDs) > 0 {
			b.WriteString(fmt.Sprintf("- **Techniques:** %s\n", strings.Join(f.TechniqueIDs, ", ")))
		}
		if f.Description != nil {
			b.WriteString(fmt.Sprintf("\n%s\n", *f.Description))
		}
		if f.Remediation != nil {
			b.WriteString(fmt.Sprintf("\n**Remediation:** %s\n", *f.Remediation))
		}
		b.WriteString("\n---\n\n")
	}

	b.WriteString("## Run History\n\n")
	b.WriteString("| Run ID | Status | Tier | Steps | Started |\n")
	b.WriteString("|--------|--------|------|-------|---------|\n")
	for _, r := range data.Runs {
		started := "N/A"
		if r.StartedAt != nil {
			started = r.StartedAt.Format("2006-01-02 15:04")
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %d | %d/%d | %s |\n",
			r.ID.String()[:8], r.Status, r.Tier, r.StepsCompleted, r.StepsTotal, started))
	}
}

func renderCoverageReport(b *strings.Builder, data *ReportData) {
	b.WriteString("## Coverage Matrix\n\n")
	b.WriteString("| Technique | Telemetry | Detection | Alert |\n")
	b.WriteString("|-----------|-----------|-----------|-------|\n")
	for _, e := range data.CoverageEntries {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			e.TechniqueID,
			boolIcon(e.HasTelemetry),
			boolIcon(e.HasDetection),
			boolIcon(e.HasAlert),
		))
	}
	b.WriteString("\n")

	if len(data.CoverageGaps) > 0 {
		b.WriteString("## Coverage Gaps\n\n")
		for _, g := range data.CoverageGaps {
			missing := []string{}
			if !g.HasTelemetry {
				missing = append(missing, "telemetry")
			}
			if !g.HasDetection {
				missing = append(missing, "detection")
			}
			if !g.HasAlert {
				missing = append(missing, "alert")
			}
			b.WriteString(fmt.Sprintf("- **%s**: Missing %s\n", g.TechniqueID, strings.Join(missing, ", ")))
		}
	}
}

func boolIcon(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}
