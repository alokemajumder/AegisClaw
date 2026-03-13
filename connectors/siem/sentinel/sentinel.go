package sentinel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// Config holds the Azure Sentinel / Log Analytics connector configuration.
type Config struct {
	WorkspaceID    string `json:"workspace_id"`
	TenantID       string `json:"tenant_id"`
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	SubscriptionID string `json:"subscription_id"`
	ResourceGroup  string `json:"resource_group"`
}

// tokenCache holds a cached OAuth2 access token and its expiry time.
type tokenCache struct {
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// Connector implements the Sentinel / Log Analytics SIEM connector.
type Connector struct {
	cfg         Config
	initialized bool
	httpClient  *http.Client
	token       tokenCache
}

// Compile-time interface assertions.
var (
	_ connectorsdk.Connector    = (*Connector)(nil)
	_ connectorsdk.EventQuerier = (*Connector)(nil)
	_ connectorsdk.DeepLinker   = (*Connector)(nil)
)

// New returns a new, uninitialised Sentinel connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "sentinel" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategorySIEM }
func (c *Connector) Version() string                 { return "1.0.0" }
func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{connectorsdk.CapQueryEvents, connectorsdk.CapDeepLink}
}

// ConfigSchema returns the JSON Schema that describes the expected
// configuration fields. client_id and client_secret are listed as required
// so that the settings UI renders them, but at runtime they can also be
// provided via the Secrets map.
func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "workspace_id":    {"type": "string", "title": "Workspace ID",    "description": "Log Analytics workspace GUID"},
    "tenant_id":       {"type": "string", "title": "Tenant ID",       "description": "Azure AD / Entra tenant GUID"},
    "client_id":       {"type": "string", "title": "Client ID",       "description": "App registration client ID"},
    "client_secret":   {"type": "string", "title": "Client Secret",   "description": "App registration client secret", "format": "password"},
    "subscription_id": {"type": "string", "title": "Subscription ID", "description": "Azure subscription GUID"},
    "resource_group":  {"type": "string", "title": "Resource Group",  "description": "Resource group containing the Sentinel workspace"}
  },
  "required": ["workspace_id", "tenant_id", "client_id", "client_secret"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Init parses configuration and prepares the HTTP client.
// Secrets from cfg.Secrets (keys "client_id", "client_secret") take precedence
// over inline config values — this is the production path where secrets are
// resolved from a vault.
func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing sentinel config: %w", err)
	}

	// Allow secrets to override inline config values.
	if v, ok := cfg.Secrets["client_id"]; ok && v != "" {
		c.cfg.ClientID = v
	}
	if v, ok := cfg.Secrets["client_secret"]; ok && v != "" {
		c.cfg.ClientSecret = v
	}

	// Validate required fields.
	if c.cfg.WorkspaceID == "" {
		return fmt.Errorf("sentinel config: workspace_id is required")
	}
	if c.cfg.TenantID == "" {
		return fmt.Errorf("sentinel config: tenant_id is required")
	}
	if c.cfg.ClientID == "" {
		return fmt.Errorf("sentinel config: client_id is required (set in config or secrets)")
	}
	if c.cfg.ClientSecret == "" {
		return fmt.Errorf("sentinel config: client_secret is required (set in config or secrets)")
	}

	c.httpClient = &http.Client{Timeout: 30 * time.Second}
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
// OAuth2 token management
// ---------------------------------------------------------------------------

// getToken returns a valid OAuth2 access token, refreshing it if expired or
// not yet acquired. The token is cached and protected by a mutex so
// concurrent callers don't stampede the token endpoint.
func (c *Connector) getToken(ctx context.Context) (string, error) {
	c.token.mu.Lock()
	defer c.token.mu.Unlock()

	// Return cached token if still valid (with 60-second safety margin).
	if c.token.accessToken != "" && time.Now().Before(c.token.expiresAt.Add(-60*time.Second)) {
		return c.token.accessToken, nil
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", c.cfg.TenantID)

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("scope", "https://api.loganalytics.io/.default")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, truncateBody(body, 512))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"` // seconds
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("token response did not contain access_token")
	}

	c.token.accessToken = tokenResp.AccessToken
	c.token.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return c.token.accessToken, nil
}

// ---------------------------------------------------------------------------
// Health & validation
// ---------------------------------------------------------------------------

// HealthCheck performs a lightweight KQL query against the workspace and
// reports the observed latency.
func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	start := time.Now()

	_, err := c.runKQLQuery(ctx, "SecurityEvent | take 1")
	latency := time.Since(start)

	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("health-check query failed: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	return &connectorsdk.HealthStatus{
		Status:    "healthy",
		Message:   "Sentinel connector operational",
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}, nil
}

// ValidateCredentials attempts to obtain an OAuth2 token and returns an error
// if authentication fails.
func (c *Connector) ValidateCredentials(ctx context.Context) error {
	_, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("sentinel credential validation failed: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Event querying (Log Analytics API)
// ---------------------------------------------------------------------------

// kqlRequest is the body sent to the Log Analytics query API.
type kqlRequest struct {
	Query    string      `json:"query"`
	Timespan *string     `json:"timespan,omitempty"`
}

// kqlResponse models the Log Analytics query response.
type kqlResponse struct {
	Tables []kqlTable `json:"tables"`
}

type kqlTable struct {
	Name    string      `json:"name"`
	Columns []kqlColumn `json:"columns"`
	Rows    [][]json.RawMessage `json:"rows"`
}

type kqlColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// QueryEvents executes a KQL query against the Log Analytics workspace and
// returns normalised EventResult records.
func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("sentinel connector not initialised: call Init first")
	}

	kql := query.Query
	if kql == "" {
		return nil, fmt.Errorf("sentinel query: KQL query string is required")
	}

	tableResp, err := c.runKQLQuery(ctx, kql)
	if err != nil {
		return nil, fmt.Errorf("sentinel query: %w", err)
	}

	events, err := c.convertToEvents(tableResp, query.MaxResults)
	if err != nil {
		return nil, fmt.Errorf("sentinel query result conversion: %w", err)
	}

	truncated := false
	if query.MaxResults > 0 && len(events) >= query.MaxResults {
		truncated = true
	}

	return &connectorsdk.EventResult{
		Events:     events,
		TotalCount: len(events),
		Truncated:  truncated,
	}, nil
}

// runKQLQuery executes a KQL query against the Log Analytics API and returns
// the raw table response.
func (c *Connector) runKQLQuery(ctx context.Context, kql string) (*kqlResponse, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtaining token: %w", err)
	}

	queryURL := fmt.Sprintf("https://api.loganalytics.io/v1/workspaces/%s/query", c.cfg.WorkspaceID)

	reqBody := kqlRequest{Query: kql}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling KQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, queryURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("building query request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing query request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading query response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query request failed (HTTP %d): %s", resp.StatusCode, truncateBody(respBody, 512))
	}

	var result kqlResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing query response: %w", err)
	}

	return &result, nil
}

// convertToEvents transforms the Log Analytics table response into a slice of
// normalised connectorsdk.Event values.
func (c *Connector) convertToEvents(resp *kqlResponse, maxResults int) ([]connectorsdk.Event, error) {
	if len(resp.Tables) == 0 {
		return nil, nil
	}

	table := resp.Tables[0]

	// Build a column-name-to-index map for fast lookup.
	colIdx := make(map[string]int, len(table.Columns))
	for i, col := range table.Columns {
		colIdx[col.Name] = i
	}

	events := make([]connectorsdk.Event, 0, len(table.Rows))
	for i, row := range table.Rows {
		if maxResults > 0 && i >= maxResults {
			break
		}

		event := connectorsdk.Event{
			ID:       fmt.Sprintf("sentinel-%s-%d", c.cfg.WorkspaceID, i),
			Source:   "sentinel",
			Metadata: map[string]string{"workspace_id": c.cfg.WorkspaceID},
		}

		// Try to extract well-known columns.
		event.Timestamp = c.extractTime(row, colIdx, "TimeGenerated")
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now().UTC()
		}
		event.Category = c.extractString(row, colIdx, "Category")
		event.Severity = c.extractSeverity(row, colIdx)
		event.Description = c.extractDescription(row, colIdx)

		// Attach the full row as raw data for downstream consumers.
		rawRow := c.rowToMap(row, table.Columns)
		if rawBytes, err := json.Marshal(rawRow); err == nil {
			event.RawData = rawBytes
		}

		events = append(events, event)
	}

	return events, nil
}

// ---------------------------------------------------------------------------
// Deep linking
// ---------------------------------------------------------------------------

// GenerateDeepLink builds a URL to the Sentinel blade in the Azure portal.
func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	return fmt.Sprintf("https://portal.azure.com/#blade/Microsoft_Azure_Security_Insights/MainMenuBlade/7/subscriptionId/%s/resourceGroup/%s/workspaceName/%s",
		c.cfg.SubscriptionID, c.cfg.ResourceGroup, c.cfg.WorkspaceID), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractString safely pulls a string value from a row by column name.
func (c *Connector) extractString(row []json.RawMessage, colIdx map[string]int, colName string) string {
	idx, ok := colIdx[colName]
	if !ok || idx >= len(row) {
		return ""
	}
	var s string
	if err := json.Unmarshal(row[idx], &s); err != nil {
		return ""
	}
	return s
}

// extractTime attempts to parse a timestamp column from the row.
func (c *Connector) extractTime(row []json.RawMessage, colIdx map[string]int, colName string) time.Time {
	s := c.extractString(row, colIdx, colName)
	if s == "" {
		return time.Time{}
	}
	// Log Analytics returns ISO 8601 timestamps.
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// extractSeverity looks for common severity column names.
func (c *Connector) extractSeverity(row []json.RawMessage, colIdx map[string]int) string {
	for _, col := range []string{"Severity", "Level", "EventLevel", "SeverityLevel"} {
		if v := c.extractString(row, colIdx, col); v != "" {
			return v
		}
	}
	return ""
}

// extractDescription looks for common description column names.
func (c *Connector) extractDescription(row []json.RawMessage, colIdx map[string]int) string {
	for _, col := range []string{"Description", "Activity", "OperationName", "Message"} {
		if v := c.extractString(row, colIdx, col); v != "" {
			return v
		}
	}
	return ""
}

// rowToMap converts a raw row into a map[string]interface{} keyed by column name.
func (c *Connector) rowToMap(row []json.RawMessage, columns []kqlColumn) map[string]interface{} {
	m := make(map[string]interface{}, len(columns))
	for i, col := range columns {
		if i >= len(row) {
			break
		}
		var v interface{}
		if err := json.Unmarshal(row[i], &v); err != nil {
			m[col.Name] = string(row[i])
		} else {
			m[col.Name] = v
		}
	}
	return m
}

// truncateBody returns at most maxLen bytes of body as a string, appending
// "..." if it was truncated.
func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
