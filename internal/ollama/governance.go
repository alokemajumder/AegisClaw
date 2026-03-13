package ollama

import (
	"fmt"
	"strings"
)

const (
	// MaxPromptLength is the maximum allowed prompt size in characters.
	MaxPromptLength = 50000

	// MaxResponseLength is the maximum expected response size.
	MaxResponseLength = 10000
)

// ValidatePrompt checks a prompt against governance rules.
func ValidatePrompt(prompt string) error {
	if len(prompt) == 0 {
		return fmt.Errorf("prompt must not be empty")
	}

	if len(prompt) > MaxPromptLength {
		return fmt.Errorf("prompt length %d exceeds maximum %d", len(prompt), MaxPromptLength)
	}

	// Check for common PII patterns (basic heuristic).
	lowerPrompt := strings.ToLower(prompt)
	piiIndicators := []string{
		"social security",
		"credit card",
		"password:",
		"secret key:",
		"api_key:",
		"private key",
	}
	for _, indicator := range piiIndicators {
		if strings.Contains(lowerPrompt, indicator) {
			return fmt.Errorf("prompt may contain PII or secrets (matched: %q)", indicator)
		}
	}

	return nil
}

// ValidateModel checks if a model name is safe to use.
func ValidateModel(model string, allowlist map[string]bool) error {
	if model == "" {
		return fmt.Errorf("model name must not be empty")
	}

	if len(model) > 100 {
		return fmt.Errorf("model name too long")
	}

	if !allowlist[model] {
		return fmt.Errorf("model %q is not in the allowed models list", model)
	}

	return nil
}
