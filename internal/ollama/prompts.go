package ollama

import (
	"fmt"
	"strings"
)

// PromptTemplate defines a named prompt template with placeholders.
type PromptTemplate struct {
	Name     string
	Template string
}

var (
	// PlannerAnalysis helps the planner agent decide which techniques to test.
	PlannerAnalysis = PromptTemplate{
		Name: "planner_analysis",
		Template: `You are a security validation planner for the AegisClaw platform.

Given the following asset information and available playbooks, recommend which validation steps to execute and in what order. Focus on high-impact techniques that are most likely to reveal coverage gaps.

Asset Information:
%s

Available Playbooks:
%s

Respond with a JSON array of objects, each containing:
- "playbook_id": the playbook identifier
- "priority": 1 (highest) to 5 (lowest)
- "rationale": brief explanation for the recommendation`,
	}

	// FindingExplanation helps explain a security finding to non-technical stakeholders.
	FindingExplanation = PromptTemplate{
		Name: "finding_explanation",
		Template: `You are a security analyst providing findings explanations for the AegisClaw platform.

Explain the following security validation finding in clear, actionable language suitable for a security operations team:

Finding Title: %s
Severity: %s
Technique: %s
Description: %s

Provide:
1. A brief explanation of what this finding means
2. The potential business impact
3. Recommended remediation steps`,
	}

	// CoverageRecommendation suggests improvements to detection coverage.
	CoverageRecommendation = PromptTemplate{
		Name: "coverage_recommendation",
		Template: `You are a detection engineering advisor for the AegisClaw platform.

Given the following coverage gaps in the security detection matrix, recommend specific improvements:

Coverage Gaps:
%s

For each gap, suggest:
1. Specific log sources or telemetry to enable
2. Detection rules or analytics to implement
3. Priority level (critical/high/medium/low)

Respond in a structured format.`,
	}
)

// FormatPrompt fills a prompt template with the provided arguments.
func FormatPrompt(tmpl PromptTemplate, args ...any) string {
	return fmt.Sprintf(tmpl.Template, args...)
}

// TruncatePrompt trims a prompt to the maximum allowed length.
func TruncatePrompt(prompt string, maxLen int) string {
	if len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen] + "\n... [truncated]"
}

// SanitizeForPrompt removes potentially problematic characters from user-provided data
// before embedding it in a prompt.
func SanitizeForPrompt(input string) string {
	// Remove any attempt at prompt injection markers
	result := strings.ReplaceAll(input, "```", "")
	result = strings.ReplaceAll(result, "SYSTEM:", "")
	result = strings.ReplaceAll(result, "ASSISTANT:", "")
	result = strings.ReplaceAll(result, "USER:", "")
	return result
}
