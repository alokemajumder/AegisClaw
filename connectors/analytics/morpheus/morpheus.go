package morpheus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// Config holds the NVIDIA Morpheus connector configuration.
type Config struct {
	// TritonURL is the Triton Inference Server endpoint (e.g. http://localhost:8000).
	TritonURL string `json:"triton_url"`
	// KafkaBrokers is a comma-separated list of Kafka brokers for log ingestion.
	KafkaBrokers string `json:"kafka_brokers"`
	// ModelName is the Morpheus pipeline model to use (e.g. sid-minibert, phishing-bert).
	ModelName string `json:"model_name"`
	// APIKey is an optional API key for authenticated Triton/Morpheus endpoints.
	APIKey string `json:"api_key"`
}

// Connector implements the NVIDIA Morpheus GPU-accelerated security analytics connector.
// Morpheus uses Triton Inference Server + RAPIDS + Kafka for real-time security log analysis,
// anomaly detection, and sensitive information detection at GPU-accelerated speeds.
type Connector struct {
	cfg         Config
	initialized bool
	httpClient  *http.Client
}

// Compile-time interface assertions.
var (
	_ connectorsdk.Connector    = (*Connector)(nil)
	_ connectorsdk.EventQuerier = (*Connector)(nil)
)

// New returns a new, uninitialised Morpheus connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "morpheus" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryAnalytics }
func (c *Connector) Version() string                 { return "1.0.0" }
func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{connectorsdk.CapQueryEvents}
}

// ConfigSchema returns the JSON Schema for Morpheus configuration.
func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "triton_url":    {"type": "string", "title": "Triton URL",    "description": "Triton Inference Server endpoint (e.g. http://localhost:8000)"},
    "kafka_brokers": {"type": "string", "title": "Kafka Brokers", "description": "Comma-separated Kafka broker addresses for log ingestion"},
    "model_name":    {"type": "string", "title": "Model Name",    "description": "Morpheus pipeline model (sid-minibert, phishing-bert, anomaly-ae, etc.)", "default": "sid-minibert"},
    "api_key":       {"type": "string", "title": "API Key",       "description": "Optional API key for authenticated endpoints", "format": "password"}
  },
  "required": ["triton_url", "model_name"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Init parses configuration and prepares the HTTP client.
func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing morpheus config: %w", err)
	}

	// Allow secrets to override inline config values.
	if v, ok := cfg.Secrets["api_key"]; ok && v != "" {
		c.cfg.APIKey = v
	}

	if c.cfg.TritonURL == "" {
		return fmt.Errorf("morpheus config: triton_url is required")
	}
	if c.cfg.ModelName == "" {
		c.cfg.ModelName = "sid-minibert"
	}

	c.httpClient = &http.Client{Timeout: 60 * time.Second}
	c.initialized = true
	return nil
}

// Close releases resources held by the connector.
func (c *Connector) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Health & validation
// ---------------------------------------------------------------------------

// HealthCheck checks whether the Triton Inference Server is live and the
// configured model is ready.
func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	start := time.Now()

	// Check Triton server liveness
	liveURL := fmt.Sprintf("%s/v2/health/live", c.cfg.TritonURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, liveURL, nil)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("building health request: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("triton server unreachable: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("triton server returned HTTP %d", resp.StatusCode),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	// Check model readiness
	modelURL := fmt.Sprintf("%s/v2/models/%s/ready", c.cfg.TritonURL, c.cfg.ModelName)
	modelReq, err := http.NewRequestWithContext(ctx, http.MethodGet, modelURL, nil)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "degraded",
			Message:   fmt.Sprintf("triton live but model check failed: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	modelResp, err := c.httpClient.Do(modelReq)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "degraded",
			Message:   fmt.Sprintf("triton live but model unreachable: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	modelResp.Body.Close()

	if modelResp.StatusCode != http.StatusOK {
		return &connectorsdk.HealthStatus{
			Status:    "degraded",
			Message:   fmt.Sprintf("triton live but model %s not ready (HTTP %d)", c.cfg.ModelName, modelResp.StatusCode),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	return &connectorsdk.HealthStatus{
		Status:    "healthy",
		Message:   fmt.Sprintf("Morpheus connector operational (model: %s)", c.cfg.ModelName),
		Latency:   time.Since(start),
		CheckedAt: time.Now().UTC(),
	}, nil
}

// ValidateCredentials checks Triton connectivity and model availability.
func (c *Connector) ValidateCredentials(ctx context.Context) error {
	status, err := c.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("morpheus credential validation: %w", err)
	}
	if status.Status == "unhealthy" {
		return fmt.Errorf("morpheus validation failed: %s", status.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Event querying (Triton Inference)
// ---------------------------------------------------------------------------

// inferRequest is the body sent to Triton for inference.
type inferRequest struct {
	Inputs []inferInput `json:"inputs"`
}

type inferInput struct {
	Name     string     `json:"name"`
	Shape    []int      `json:"shape"`
	DataType string     `json:"datatype"`
	Data     [][]string `json:"data"`
}

// inferResponse is the response from Triton inference.
type inferResponse struct {
	ModelName    string        `json:"model_name"`
	ModelVersion string        `json:"model_version"`
	Outputs      []inferOutput `json:"outputs"`
}

type inferOutput struct {
	Name     string          `json:"name"`
	Shape    []int           `json:"shape"`
	DataType string          `json:"datatype"`
	Data     json.RawMessage `json:"data"`
}

// QueryEvents sends log data to the Morpheus pipeline via Triton and returns
// classified/scored security events.
func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("morpheus connector not initialised: call Init first")
	}

	logQuery := query.Query
	if logQuery == "" {
		return nil, fmt.Errorf("morpheus query: log data or query string is required")
	}

	// Build Triton inference request
	inferReq := inferRequest{
		Inputs: []inferInput{
			{
				Name:     "input",
				Shape:    []int{1, 1},
				DataType: "BYTES",
				Data:     [][]string{{logQuery}},
			},
		},
	}

	bodyBytes, err := json.Marshal(inferReq)
	if err != nil {
		return nil, fmt.Errorf("marshalling inference request: %w", err)
	}

	inferURL := fmt.Sprintf("%s/v2/models/%s/infer", c.cfg.TritonURL, c.cfg.ModelName)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, inferURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("building inference request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing inference request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading inference response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inference request failed (HTTP %d): %s", resp.StatusCode, truncateBody(respBody, 512))
	}

	var inferResp inferResponse
	if err := json.Unmarshal(respBody, &inferResp); err != nil {
		return nil, fmt.Errorf("parsing inference response: %w", err)
	}

	events := c.convertToEvents(inferResp, query.MaxResults)

	return &connectorsdk.EventResult{
		Events:     events,
		TotalCount: len(events),
		Truncated:  query.MaxResults > 0 && len(events) >= query.MaxResults,
	}, nil
}

// convertToEvents transforms Triton inference output into normalized events.
func (c *Connector) convertToEvents(resp inferResponse, maxResults int) []connectorsdk.Event {
	var events []connectorsdk.Event

	for i, output := range resp.Outputs {
		if maxResults > 0 && i >= maxResults {
			break
		}

		raw, _ := json.Marshal(output)
		event := connectorsdk.Event{
			ID:        fmt.Sprintf("morpheus-%s-%d", resp.ModelName, i),
			Timestamp: time.Now().UTC(),
			Source:    "morpheus",
			Category:  resp.ModelName,
			Metadata: map[string]string{
				"model_name":    resp.ModelName,
				"model_version": resp.ModelVersion,
				"output_name":   output.Name,
			},
			RawData: raw,
		}

		// Determine severity from model output name conventions
		switch output.Name {
		case "anomaly_score", "threat_score":
			event.Severity = "high"
			event.Description = fmt.Sprintf("Morpheus %s detection via %s", output.Name, resp.ModelName)
		case "classification", "label":
			event.Severity = "medium"
			event.Description = fmt.Sprintf("Morpheus classification via %s", resp.ModelName)
		default:
			event.Severity = "info"
			event.Description = fmt.Sprintf("Morpheus inference output: %s", output.Name)
		}

		events = append(events, event)
	}

	return events
}

// truncateBody returns at most maxLen bytes of body as a string.
func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
