package elastic

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// Config holds the Elastic Security / Elasticsearch connector configuration.
type Config struct {
	URL          string `json:"url"`           // e.g. https://elasticsearch.example.com:9200
	APIKey       string `json:"api_key"`       // Elasticsearch API key (preferred)
	Username     string `json:"username"`      // Basic auth fallback
	Password     string `json:"password"`      // Basic auth fallback
	CloudID      string `json:"cloud_id"`      // Elastic Cloud deployment ID (optional)
	IndexPattern string `json:"index_pattern"` // Default index pattern for queries
	VerifyTLS    bool   `json:"verify_tls"`    // Defaults to true
}

// Connector implements the Elastic Security SIEM connector.
type Connector struct {
	cfg         Config
	httpClient  *http.Client
	initialized bool
}

// Compile-time interface assertions.
var (
	_ connectorsdk.Connector    = (*Connector)(nil)
	_ connectorsdk.EventQuerier = (*Connector)(nil)
	_ connectorsdk.DeepLinker   = (*Connector)(nil)
)

// New returns a new, uninitialised Elastic connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "elastic" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategorySIEM }
func (c *Connector) Version() string                 { return "1.0.0" }

func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{connectorsdk.CapQueryEvents, connectorsdk.CapDeepLink}
}

func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "url":           {"type": "string", "title": "Elasticsearch URL", "description": "Elasticsearch cluster URL (e.g. https://es.example.com:9200)"},
    "api_key":       {"type": "string", "title": "API Key",          "description": "Elasticsearch API key for authentication", "format": "password"},
    "username":      {"type": "string", "title": "Username",         "description": "Username for Basic auth (if not using API key)"},
    "password":      {"type": "string", "title": "Password",         "description": "Password for Basic auth", "format": "password"},
    "cloud_id":      {"type": "string", "title": "Cloud ID",         "description": "Elastic Cloud deployment ID (optional)"},
    "index_pattern": {"type": "string", "title": "Index Pattern",    "description": "Default index pattern for security searches", "default": ".siem-signals-*,logs-*"},
    "verify_tls":    {"type": "boolean","title": "Verify TLS",       "description": "Verify TLS certificates", "default": true}
  },
  "required": ["url"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	c.cfg.VerifyTLS = true

	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing elastic config: %w", err)
	}

	if v, ok := cfg.Secrets["api_key"]; ok && v != "" {
		c.cfg.APIKey = v
	}
	if v, ok := cfg.Secrets["username"]; ok && v != "" {
		c.cfg.Username = v
	}
	if v, ok := cfg.Secrets["password"]; ok && v != "" {
		c.cfg.Password = v
	}

	if c.cfg.URL == "" {
		return fmt.Errorf("elastic config: url is required")
	}
	c.cfg.URL = strings.TrimRight(c.cfg.URL, "/")

	if c.cfg.APIKey == "" && (c.cfg.Username == "" || c.cfg.Password == "") {
		return fmt.Errorf("elastic config: either api_key or username+password is required")
	}

	if c.cfg.IndexPattern == "" {
		c.cfg.IndexPattern = ".siem-signals-*,logs-*"
	}

	transport := &http.Transport{}
	if !c.cfg.VerifyTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // User-controlled config
	}

	c.httpClient = &http.Client{Timeout: 60 * time.Second, Transport: transport}
	c.initialized = true
	return nil
}

func (c *Connector) Close() error {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Health & validation
// ---------------------------------------------------------------------------

func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL+"/_cluster/health", nil)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("failed to build health request: %v", err),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("cluster health check failed: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "degraded",
			Message:   "unable to read health response",
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("authentication failed (HTTP %d)", resp.StatusCode),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &connectorsdk.HealthStatus{
			Status:    "degraded",
			Message:   fmt.Sprintf("unexpected response (HTTP %d)", resp.StatusCode),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	var health struct {
		ClusterName string `json:"cluster_name"`
		Status      string `json:"status"` // green, yellow, red
	}
	_ = json.Unmarshal(body, &health)

	status := "healthy"
	msg := fmt.Sprintf("cluster %q status: %s, latency %s", health.ClusterName, health.Status, latency.Round(time.Millisecond))
	if health.Status == "red" {
		status = "unhealthy"
	} else if health.Status == "yellow" {
		status = "degraded"
	}

	return &connectorsdk.HealthStatus{
		Status:    status,
		Message:   msg,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func (c *Connector) ValidateCredentials(ctx context.Context) error {
	hs, err := c.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("elastic credential validation failed: %w", err)
	}
	if hs.Status == "unhealthy" {
		return fmt.Errorf("elastic credential validation failed: %s", hs.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Event querying (Elasticsearch _search API)
// ---------------------------------------------------------------------------

// esSearchResponse models the Elasticsearch search response.
type esSearchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			ID     string                     `json:"_id"`
			Index  string                     `json:"_index"`
			Source map[string]json.RawMessage  `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("elastic connector not initialised: call Init first")
	}

	// Build the query body. If the caller provided a raw query string, wrap it
	// in a query_string query. Otherwise use match_all.
	var queryBody map[string]any
	if query.Query != "" {
		queryBody = map[string]any{
			"query": map[string]any{
				"query_string": map[string]any{
					"query": query.Query,
				},
			},
			"sort": []map[string]any{
				{"@timestamp": map[string]string{"order": "desc"}},
			},
		}
	} else {
		queryBody = map[string]any{
			"query": map[string]any{"match_all": map[string]any{}},
			"sort":  []map[string]any{{"@timestamp": map[string]string{"order": "desc"}}},
		}
	}

	size := 100
	if query.MaxResults > 0 {
		size = query.MaxResults
	}
	queryBody["size"] = size

	// Add time range filter if provided.
	if !query.TimeRange.Start.IsZero() || !query.TimeRange.End.IsZero() {
		rangeFilter := map[string]any{}
		if !query.TimeRange.Start.IsZero() {
			rangeFilter["gte"] = query.TimeRange.Start.Format(time.RFC3339)
		}
		if !query.TimeRange.End.IsZero() {
			rangeFilter["lte"] = query.TimeRange.End.Format(time.RFC3339)
		}
		queryBody["query"] = map[string]any{
			"bool": map[string]any{
				"must": queryBody["query"],
				"filter": []map[string]any{
					{"range": map[string]any{"@timestamp": rangeFilter}},
				},
			},
		}
	}

	bodyBytes, err := json.Marshal(queryBody)
	if err != nil {
		return nil, fmt.Errorf("elastic: marshalling query: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s/_search", c.cfg.URL, c.cfg.IndexPattern)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("elastic: building search request: %w", err)
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elastic: search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("elastic: reading search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elastic: search returned HTTP %d: %s", resp.StatusCode, truncateBody(respBody, 512))
	}

	var esResp esSearchResponse
	if err := json.Unmarshal(respBody, &esResp); err != nil {
		return nil, fmt.Errorf("elastic: parsing search response: %w", err)
	}

	events := make([]connectorsdk.Event, 0, len(esResp.Hits.Hits))
	for _, hit := range esResp.Hits.Hits {
		evt := connectorsdk.Event{
			ID:       hit.ID,
			Source:   "elastic",
			Metadata: make(map[string]string),
		}

		// Extract well-known fields.
		evt.Timestamp = extractTime(hit.Source, "@timestamp", "timestamp")
		if evt.Timestamp.IsZero() {
			evt.Timestamp = time.Now().UTC()
		}
		evt.Severity = extractStr(hit.Source, "event.severity", "signal.rule.severity", "severity")
		evt.Category = extractStr(hit.Source, "event.category", "signal.rule.name", "rule.name")
		evt.Description = extractStr(hit.Source, "message", "signal.rule.description", "event.action")

		rawBytes, _ := json.Marshal(hit.Source)
		evt.RawData = rawBytes
		evt.Metadata["_index"] = hit.Index

		for k, v := range hit.Source {
			var s string
			if json.Unmarshal(v, &s) == nil {
				evt.Metadata[k] = s
			}
		}

		events = append(events, evt)
	}

	truncated := esResp.Hits.Total.Value > len(events)

	return &connectorsdk.EventResult{
		Events:     events,
		TotalCount: esResp.Hits.Total.Value,
		Truncated:  truncated,
	}, nil
}

// ---------------------------------------------------------------------------
// Deep linking
// ---------------------------------------------------------------------------

func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	// Link to Kibana Discover with the document ID.
	kibanaURL := strings.Replace(c.cfg.URL, ":9200", ":5601", 1)
	return fmt.Sprintf("%s/app/security/%s/%s", kibanaURL, entityType, entityID), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (c *Connector) setAuth(req *http.Request) {
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+c.cfg.APIKey)
	} else {
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}
}

func extractStr(source map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		// Handle dotted keys (e.g., "event.severity") by checking top-level first.
		if raw, ok := source[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return s
			}
		}
		// Try nested: split on first dot.
		parts := strings.SplitN(k, ".", 2)
		if len(parts) == 2 {
			if raw, ok := source[parts[0]]; ok {
				var nested map[string]json.RawMessage
				if json.Unmarshal(raw, &nested) == nil {
					if inner, ok := nested[parts[1]]; ok {
						var s string
						if json.Unmarshal(inner, &s) == nil && s != "" {
							return s
						}
					}
				}
			}
		}
	}
	return ""
}

func extractTime(source map[string]json.RawMessage, keys ...string) time.Time {
	for _, k := range keys {
		if raw, ok := source[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
					if t, err := time.Parse(layout, s); err == nil {
						return t.UTC()
					}
				}
			}
		}
	}
	return time.Time{}
}

func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
