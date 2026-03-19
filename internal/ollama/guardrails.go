package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// GuardrailsClient provides content safety, jailbreak detection, and topic control
// via NVIDIA NeMo Guardrails NIMs. Each guardrail runs as a separate NIM endpoint.
//
// When enabled, all LLM prompts and responses are screened before processing.
// This adds latency but provides enterprise-grade safety for autonomous agent reasoning.
type GuardrailsClient struct {
	contentSafetyURL string
	jailbreakURL     string
	topicControlURL  string
	httpClient       *http.Client
	logger           *slog.Logger
}

// GuardrailsResult contains the outcome of a guardrails check.
type GuardrailsResult struct {
	Safe     bool   `json:"safe"`
	Reason   string `json:"reason,omitempty"`
	Category string `json:"category,omitempty"` // "content_safety", "jailbreak", "topic_control"
}

type guardrailRequest struct {
	Model    string            `json:"model"`
	Messages []chatMessage     `json:"messages"`
}

type guardrailResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// NewGuardrailsClient creates a new NeMo Guardrails client.
func NewGuardrailsClient(contentSafetyURL, jailbreakURL, topicControlURL string, timeoutSeconds int, logger *slog.Logger) *GuardrailsClient {
	return &GuardrailsClient{
		contentSafetyURL: contentSafetyURL,
		jailbreakURL:     jailbreakURL,
		topicControlURL:  topicControlURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		logger: logger,
	}
}

// CheckPrompt runs all enabled guardrails checks on a prompt before sending to LLM.
// Returns nil if the prompt passes all checks, or a GuardrailsResult describing the violation.
func (g *GuardrailsClient) CheckPrompt(ctx context.Context, prompt string) (*GuardrailsResult, error) {
	// Content safety check
	if g.contentSafetyURL != "" {
		result, err := g.checkEndpoint(ctx, g.contentSafetyURL, "content_safety", prompt)
		if err != nil {
			g.logger.Warn("content safety check failed, allowing prompt (fail-open for availability)", "error", err)
		} else if result != nil && !result.Safe {
			return result, nil
		}
	}

	// Jailbreak detection
	if g.jailbreakURL != "" {
		result, err := g.checkEndpoint(ctx, g.jailbreakURL, "jailbreak", prompt)
		if err != nil {
			g.logger.Warn("jailbreak detection failed, allowing prompt (fail-open for availability)", "error", err)
		} else if result != nil && !result.Safe {
			return result, nil
		}
	}

	// Topic control — ensure prompt stays within security validation scope
	if g.topicControlURL != "" {
		result, err := g.checkEndpoint(ctx, g.topicControlURL, "topic_control", prompt)
		if err != nil {
			g.logger.Warn("topic control check failed, allowing prompt (fail-open for availability)", "error", err)
		} else if result != nil && !result.Safe {
			return result, nil
		}
	}

	return nil, nil // All checks passed
}

// checkEndpoint sends a prompt to a guardrails NIM endpoint and interprets the response.
func (g *GuardrailsClient) checkEndpoint(ctx context.Context, url, category, prompt string) (*GuardrailsResult, error) {
	reqBody := guardrailRequest{
		Model: category,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling guardrails request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating guardrails request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling guardrails endpoint %s: %w", category, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("guardrails %s returned status %d: %s", category, resp.StatusCode, truncateNIMBody(respBody, 256))
	}

	var grResp guardrailResponse
	if err := json.NewDecoder(resp.Body).Decode(&grResp); err != nil {
		return nil, fmt.Errorf("decoding guardrails response: %w", err)
	}

	// Guardrails NIMs return "safe" or "unsafe" with a reason in the content
	if len(grResp.Choices) > 0 {
		content := grResp.Choices[0].Message.Content
		if content == "unsafe" || content == "blocked" {
			return &GuardrailsResult{
				Safe:     false,
				Reason:   fmt.Sprintf("blocked by %s guardrail", category),
				Category: category,
			}, nil
		}
	}

	g.logger.Debug("guardrails check passed", "category", category)
	return &GuardrailsResult{Safe: true, Category: category}, nil
}

// IsAvailable checks if at least one guardrails endpoint is reachable.
func (g *GuardrailsClient) IsAvailable(ctx context.Context) bool {
	endpoints := []string{g.contentSafetyURL, g.jailbreakURL, g.topicControlURL}
	for _, url := range endpoints {
		if url == "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/v1/models", nil)
		if err != nil {
			continue
		}
		resp, err := g.httpClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return false
}
