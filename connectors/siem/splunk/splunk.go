package splunk

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

// Config holds the Splunk Enterprise / Splunk Cloud connector configuration.
type Config struct {
	BaseURL  string `json:"base_url"`  // e.g. https://splunk.example.com:8089
	Token    string `json:"token"`     // Splunk bearer token (preferred)
	Username string `json:"username"`  // Basic auth fallback
	Password string `json:"password"`  // Basic auth fallback
	Index    string `json:"index"`     // Default index for searches
	VerifyTLS bool  `json:"verify_tls"` // Defaults to true
}

// Connector implements the Splunk SIEM connector.
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

// New returns a new, uninitialised Splunk connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "splunk" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategorySIEM }
func (c *Connector) Version() string                 { return "1.0.0" }

func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{connectorsdk.CapQueryEvents, connectorsdk.CapDeepLink}
}

func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "base_url":   {"type": "string", "title": "Base URL",   "description": "Splunk management URL (e.g. https://splunk.example.com:8089)"},
    "token":      {"type": "string", "title": "Bearer Token", "description": "Splunk authentication token", "format": "password"},
    "username":   {"type": "string", "title": "Username",   "description": "Splunk username (if not using token auth)"},
    "password":   {"type": "string", "title": "Password",   "description": "Splunk password", "format": "password"},
    "index":      {"type": "string", "title": "Default Index", "description": "Default Splunk index for searches", "default": "main"},
    "verify_tls": {"type": "boolean", "title": "Verify TLS", "description": "Verify TLS certificates (disable for self-signed)", "default": true}
  },
  "required": ["base_url"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	// Default verify_tls to true before unmarshalling.
	c.cfg.VerifyTLS = true

	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing splunk config: %w", err)
	}

	if v, ok := cfg.Secrets["token"]; ok && v != "" {
		c.cfg.Token = v
	}
	if v, ok := cfg.Secrets["username"]; ok && v != "" {
		c.cfg.Username = v
	}
	if v, ok := cfg.Secrets["password"]; ok && v != "" {
		c.cfg.Password = v
	}

	if c.cfg.BaseURL == "" {
		return fmt.Errorf("splunk config: base_url is required")
	}
	c.cfg.BaseURL = strings.TrimRight(c.cfg.BaseURL, "/")

	if c.cfg.Token == "" && (c.cfg.Username == "" || c.cfg.Password == "") {
		return fmt.Errorf("splunk config: either token or username+password is required")
	}

	if c.cfg.Index == "" {
		c.cfg.Index = "main"
	}

	transport := &http.Transport{}
	if !c.cfg.VerifyTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // User-controlled config for self-signed certs
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

	endpoint := c.cfg.BaseURL + "/services/server/info?output_mode=json&count=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("failed to build health check request: %v", err),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("health check failed: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	status := "healthy"
	message := fmt.Sprintf("Splunk reachable (HTTP %d), latency %s", resp.StatusCode, latency.Round(time.Millisecond))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		status = "unhealthy"
		message = fmt.Sprintf("authentication failed (HTTP %d)", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		status = "degraded"
		message = fmt.Sprintf("unexpected response (HTTP %d)", resp.StatusCode)
	}

	return &connectorsdk.HealthStatus{
		Status:    status,
		Message:   message,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func (c *Connector) ValidateCredentials(ctx context.Context) error {
	hs, err := c.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("splunk credential validation failed: %w", err)
	}
	if hs.Status == "unhealthy" {
		return fmt.Errorf("splunk credential validation failed: %s", hs.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Event querying (Splunk Search API — oneshot)
// ---------------------------------------------------------------------------

// searchResponse models the Splunk oneshot search JSON response.
type searchResponse struct {
	Results []map[string]json.RawMessage `json:"results"`
}

func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("splunk connector not initialised: call Init first")
	}

	spl := query.Query
	if spl == "" {
		return nil, fmt.Errorf("splunk query: SPL query string is required")
	}

	// Use the oneshot search endpoint for synchronous results.
	endpoint := c.cfg.BaseURL + "/services/search/jobs/export"

	form := "search=" + spl + "&output_mode=json&count=10000"
	if query.MaxResults > 0 {
		form = fmt.Sprintf("search=%s&output_mode=json&count=%d", spl, query.MaxResults)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte(form)))
	if err != nil {
		return nil, fmt.Errorf("splunk: building search request: %w", err)
	}
	c.setAuth(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("splunk: search request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("splunk: reading search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("splunk: search returned HTTP %d: %s", resp.StatusCode, truncateBody(body, 512))
	}

	// The export endpoint returns NDJSON — each line is a result object.
	// Try parsing as a single JSON array first, then fall back to NDJSON.
	events, err := c.parseResults(body, query.MaxResults)
	if err != nil {
		return nil, fmt.Errorf("splunk: parsing results: %w", err)
	}

	truncated := query.MaxResults > 0 && len(events) >= query.MaxResults

	return &connectorsdk.EventResult{
		Events:     events,
		TotalCount: len(events),
		Truncated:  truncated,
	}, nil
}

func (c *Connector) parseResults(body []byte, maxResults int) ([]connectorsdk.Event, error) {
	// Splunk export returns one JSON object per line (NDJSON).
	lines := bytes.Split(bytes.TrimSpace(body), []byte("\n"))
	events := make([]connectorsdk.Event, 0, len(lines))

	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		if maxResults > 0 && len(events) >= maxResults {
			break
		}

		var row struct {
			Result map[string]json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			// Try direct map parse (some Splunk versions).
			var direct map[string]json.RawMessage
			if err2 := json.Unmarshal(line, &direct); err2 != nil {
				continue
			}
			row.Result = direct
		}

		if row.Result == nil {
			continue
		}

		evt := connectorsdk.Event{
			ID:       fmt.Sprintf("splunk-%d-%d", time.Now().UnixNano(), i),
			Source:   "splunk",
			Metadata: make(map[string]string),
		}

		// Extract well-known fields.
		if raw, ok := row.Result["_time"]; ok {
			var ts string
			if json.Unmarshal(raw, &ts) == nil {
				for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000-07:00", "2006-01-02T15:04:05"} {
					if t, perr := time.Parse(layout, ts); perr == nil {
						evt.Timestamp = t.UTC()
						break
					}
				}
			}
		}
		if evt.Timestamp.IsZero() {
			evt.Timestamp = time.Now().UTC()
		}

		evt.Severity = extractString(row.Result, "severity", "urgency", "priority")
		evt.Category = extractString(row.Result, "source", "sourcetype")
		evt.Description = extractString(row.Result, "_raw", "message", "Message")

		rawBytes, _ := json.Marshal(row.Result)
		evt.RawData = rawBytes

		for k, v := range row.Result {
			var s string
			if json.Unmarshal(v, &s) == nil {
				evt.Metadata[k] = s
			}
		}

		events = append(events, evt)
	}

	return events, nil
}

// ---------------------------------------------------------------------------
// Deep linking
// ---------------------------------------------------------------------------

func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	// Link to the Splunk search UI with the entity as a query.
	return fmt.Sprintf("%s/app/search/search?q=%s=%s", c.cfg.BaseURL, entityType, entityID), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (c *Connector) setAuth(req *http.Request) {
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	} else {
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}
}

func extractString(row map[string]json.RawMessage, candidates ...string) string {
	for _, k := range candidates {
		if raw, ok := row[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				return s
			}
		}
	}
	return ""
}

func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
