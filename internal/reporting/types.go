package reporting

// ReportType defines the kind of report.
type ReportType string

const (
	ReportExecutive  ReportType = "executive"
	ReportTechnical  ReportType = "technical"
	ReportCoverage   ReportType = "coverage"
	ReportCompliance ReportType = "compliance"
)

// ReportConfig holds configuration for report generation.
type ReportConfig struct {
	Type   ReportType `json:"type"`
	Format string     `json:"format"` // markdown, json
	Title  string     `json:"title"`
}
