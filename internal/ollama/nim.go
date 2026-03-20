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
// alternative to Ollama for organisations with NVIDIA infrastructure.
//
// Recommended Nemotron 3 models (March 2026):
//   - nvidia/nemotron-3-nano-30b-a3b     — Hybrid Mamba-Transformer MoE, 1M context. Runs on RTX 4090/5090 (24GB). Best for SMB.
//   - nvidia/nemotron-3-super-120b-a12b  — Hybrid Mamba-Transformer MoE, 1M context. Multi-agent enterprise. 2×RTX or A100.
//   - nvidia/llama-nemotron-ultra-253b   — Maximum reasoning. DGX or multi-GPU.
//   - nvidia/nemotron-nano-vl-12b        — Vision-language: document intelligence, video analysis.
//
// Specialized models:
//   - nvidia/nemotron-safety             — Multilingual, multimodal safety guardrails.
//   - nvidia/nemotron-rag-*              — RAG extraction, embedding, reranking.
//
// All models support configurable thinking budget for accuracy/throughput tradeoffs.
// Models with strict tool-calling support: DeepSeek, GLM-4.7, Kimi-K2-Thinking.
//
// Supported deployment targets:
//   - Consumer/gaming GPUs: RTX 4090, RTX 5090, RTX 4080 (self-hosted NIM, SMB-optimized)
//   - NVIDIA DGX Spark (Grace Blackwell, $3999) — desktop supercomputer for small teams
//   - NVIDIA DGX BasePOD / SuperPOD (enterprise)
//   - NVIDIA API Catalog (build.nvidia.com) — 34+ hosted models, pay-per-token
//   - NemoClaw (build.nvidia.com/nemoclaw) — OpenShell-powered always-on assistant stack
type NIMClient struct {
	baseURL        string
	apiKey         string
	httpClient     *http.Client
	allowlist      map[string]bool
	thinkingBudget int // Configurable reasoning depth (0=default, 1-10 scale)
	logger         *slog.Logger
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

// SetThinkingBudget configures the reasoning depth for Nemotron models.
// 0 = default model behavior, 1-10 = shallow to deep reasoning.
// Higher values produce more accurate but slower responses.
// Maps to the model's configurable thinking budget feature.
func (c *NIMClient) SetThinkingBudget(budget int) {
	if budget < 0 {
		budget = 0
	}
	if budget > 10 {
		budget = 10
	}
	c.thinkingBudget = budget
}

// chatRequest is the OpenAI-compatible chat completions request.
// Supports tool-calling for agent function invocation (Nemotron 3, DeepSeek, GLM-4.7).
type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	Temperature    float64       `json:"temperature,omitempty"`
	Stream         bool          `json:"stream"`
	Tools          []Tool        `json:"tools,omitempty"`
	ToolChoice     string        `json:"tool_choice,omitempty"` // auto, none, required
	ThinkingBudget int           `json:"thinking_budget,omitempty"`
}

type chatMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // For role=tool responses
}

// Tool defines a function the model can call (OpenAI tool-calling format).
type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// ToolCall represents a function call requested by the model.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// chatResponse is the OpenAI-compatible chat completions response.
type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"` // stop, tool_calls, length
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
		MaxTokens:      4096,
		Temperature:    0.2, // Low temperature for deterministic security analysis
		Stream:         false,
		ThinkingBudget: c.thinkingBudget,
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

// GenerateWithTools sends a prompt with tool definitions and returns the model's response,
// which may include tool call requests. This enables agent function invocation where the
// model decides which tools to call based on the prompt context.
//
// Supported models for tool-calling: Nemotron 3 family, DeepSeek-V3, GLM-4.7, Kimi-K2.
func (c *NIMClient) GenerateWithTools(ctx context.Context, model, prompt string, tools []Tool) (*chatResponse, error) {
	if !c.allowlist[model] {
		return nil, fmt.Errorf("model %q not in NIM allowlist", model)
	}

	if err := ValidatePrompt(prompt); err != nil {
		return nil, fmt.Errorf("prompt validation: %w", err)
	}

	reqBody := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: "You are AegisClaw, an autonomous security validation assistant. Use the provided tools to execute actions. Provide precise, evidence-anchored analysis."},
			{Role: "user", Content: prompt},
		},
		MaxTokens:      4096,
		Temperature:    0.2,
		Stream:         false,
		Tools:          tools,
		ToolChoice:     "auto",
		ThinkingBudget: c.thinkingBudget,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling NIM tool-calling request: %w", err)
	}

	endpoint := c.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating NIM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling NIM endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("NIM returned status %d: %s", resp.StatusCode, truncateNIMBody(respBody, 512))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decoding NIM response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("NIM returned empty choices")
	}

	c.logger.Debug("NIM tool-calling complete",
		"model", model,
		"finish_reason", chatResp.Choices[0].FinishReason,
		"tool_calls", len(chatResp.Choices[0].Message.ToolCalls),
		"prompt_tokens", chatResp.Usage.PromptTokens,
		"completion_tokens", chatResp.Usage.CompletionTokens,
	)

	return &chatResp, nil
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
