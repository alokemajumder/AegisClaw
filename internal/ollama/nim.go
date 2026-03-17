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

// NIMClient communicates with an NVIDIA NIM (NeMo Inference Microservice) endpoint.
// NIM exposes an OpenAI-compatible chat completions API, making it a drop-in
// alternative to Ollama for organisations with NVIDIA infrastructure (DGX, RTX, cloud).
//
// Supported deployment targets:
//   - NVIDIA API Catalog (build.nvidia.com)
//   - Self-hosted NIM on DGX / DGX Spark / DGX Station
//   - NVIDIA NeMoClaw runtime (OpenShell-based agent execution)
type NIMClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	allowlist  map[string]bool
	logger     *slog.Logger
}

// NewNIMClient creates a new NVIDIA NIM client.
//
// baseURL is the NIM endpoint (e.g. "https://integrate.api.nvidia.com/v1" for
// cloud, or "http://localhost:8000/v1" for self-hosted).
// apiKey is the NVIDIA API key (required for cloud; optional for self-hosted).
func NewNIMClient(baseURL string, apiKey string, timeoutSeconds int, allowedModels []string, logger *slog.Logger) *NIMClient {
	allow := make(map[string]bool, len(allowedModels))
	for _, m := range allowedModels {
		allow[m] = true
	}

	return &NIMClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		allowlist: allow,
		logger:    logger,
	}
}

// chatRequest is the OpenAI-compatible chat completions request.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the OpenAI-compatible chat completions response.
type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Generate sends a prompt to the NIM endpoint and returns the response.
// It uses the OpenAI-compatible chat completions API that all NIM endpoints expose.
func (c *NIMClient) Generate(ctx context.Context, model, prompt string) (string, error) {
	if !c.allowlist[model] {
		return "", fmt.Errorf("model %q not in NIM allowlist", model)
	}

	if err := ValidatePrompt(prompt); err != nil {
		return "", fmt.Errorf("prompt validation: %w", err)
	}

	reqBody := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: "You are AegisClaw, an autonomous security validation assistant. Provide precise, evidence-anchored analysis."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   4096,
		Temperature: 0.2, // Low temperature for deterministic security analysis
		Stream:      false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling NIM request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating NIM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling NIM endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("NIM returned status %d: %s", resp.StatusCode, truncateNIMBody(respBody, 512))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decoding NIM response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("NIM returned empty choices")
	}

	result := chatResp.Choices[0].Message.Content

	c.logger.Debug("NIM generate complete",
		"model", model,
		"response_length", len(result),
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
	)

	return result, nil
}

// IsAvailable checks if the NIM endpoint is reachable.
func (c *NIMClient) IsAvailable(ctx context.Context) bool {
	// NIM endpoints expose /v1/models for model listing.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return false
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == http.StatusOK
}

func truncateNIMBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
