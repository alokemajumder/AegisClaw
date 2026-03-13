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

// Client communicates with a local Ollama instance.
type Client struct {
	baseURL    string
	httpClient *http.Client
	allowlist  map[string]bool
	logger     *slog.Logger
}

// NewClient creates a new Ollama client.
func NewClient(baseURL string, timeoutSeconds int, allowedModels []string, logger *slog.Logger) *Client {
	allow := make(map[string]bool)
	for _, m := range allowedModels {
		allow[m] = true
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		allowlist: allow,
		logger:    logger,
	}
}

// GenerateRequest is the request body for /api/generate.
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// GenerateResponse is the response from /api/generate.
type GenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends a prompt to Ollama and returns the response.
func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	if !c.allowlist[model] {
		return "", fmt.Errorf("model %q not in allowlist", model)
	}

	if err := ValidatePrompt(prompt); err != nil {
		return "", fmt.Errorf("prompt validation: %w", err)
	}

	reqBody := GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var genResp GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	c.logger.Debug("ollama generate complete", "model", model, "response_length", len(genResp.Response))
	return genResp.Response, nil
}

// IsAvailable checks if the Ollama service is reachable.
func (c *Client) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
