package defender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// Config holds the Defender for Endpoint connector configuration.
type Config struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	APIURL       string `json:"api_url"`
}

// tokenCache holds a cached OAuth2 access token with its expiry time.
type tokenCache struct {
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// Connector implements the Microsoft Defender for Endpoint connector.
type Connector struct {
	cfg         Config
	httpClient  *http.Client
	token       tokenCache
	initialized bool
}

// New creates a new uninitialised Defender connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "defender" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryEDR }
func (c *Connector) Version() string                 { return "1.0.0" }

func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{
		connectorsdk.CapQueryEvents,
		connectorsdk.CapFetchAssets,
		connectorsdk.CapDeepLink,
	}
}

// ConfigSchema returns the JSON Schema that describes the connector's
// configuration surface. client_id and client_secret may alternatively be
// supplied via the Secrets map (keys "client_id", "client_secret").
func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "tenant_id": {
      "type": "string",
      "title": "Tenant ID",
      "description": "Azure AD tenant ID (GUID)"
    },
    "client_id": {
      "type": "string",
      "title": "Client ID",
      "description": "Azure AD app registration client ID"
    },
    "client_secret": {
      "type": "string",
      "title": "Client Secret",
      "description": "Azure AD app registration client secret",
      "x-secret": true
    },
    "api_url": {
      "type": "string",
      "title": "API URL",
      "description": "MDE API base URL",
      "default": "https://api.securitycenter.microsoft.com"
    }
  },
  "required": ["tenant_id", "client_id", "client_secret"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// Init parses configuration, merges secrets, sets defaults, and prepares the
// HTTP client. It does NOT attempt a network call; use ValidateCredentials for
// that.
func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing defender config: %w", err)
	}

	// Allow secrets to override / supplement the config-level values.
	if v, ok := cfg.Secrets["client_id"]; ok && v != "" {
		c.cfg.ClientID = v
	}
	if v, ok := cfg.Secrets["client_secret"]; ok && v != "" {
		c.cfg.ClientSecret = v
	}

	if c.cfg.TenantID == "" {
		return fmt.Errorf("defender: tenant_id is required")
	}
	if c.cfg.ClientID == "" {
		return fmt.Errorf("defender: client_id is required (config or secrets)")
	}
	if c.cfg.ClientSecret == "" {
		return fmt.Errorf("defender: client_secret is required (config or secrets)")
	}
	if c.cfg.APIURL == "" {
		c.cfg.APIURL = "https://api.securitycenter.microsoft.com"
	}
	// Trim trailing slash so callers can just append paths.
	c.cfg.APIURL = strings.TrimRight(c.cfg.APIURL, "/")

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

// getToken returns a valid access token, refreshing it if the cached one has
// expired (or is within a 60-second safety margin of expiry).
func (c *Connector) getToken(ctx context.Context) (string, error) {
	c.token.mu.Lock()
	defer c.token.mu.Unlock()

	// Return cached token if still valid (with 60s safety margin).
	if c.token.accessToken != "" && time.Now().Before(c.token.expiresAt.Add(-60*time.Second)) {
		return c.token.accessToken, nil
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", c.cfg.TenantID)

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"scope":         {"https://api.securitycenter.microsoft.com/.default"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("defender: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("defender: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("defender: reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("defender: token endpoint returned %d: %s", resp.StatusCode, truncate(string(body), 512))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"` // seconds
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("defender: parsing token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("defender: empty access_token in response")
	}

	c.token.accessToken = tokenResp.AccessToken
	c.token.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return c.token.accessToken, nil
}

// ---------------------------------------------------------------------------
// Health / Validation
// ---------------------------------------------------------------------------

// HealthCheck verifies connectivity by listing machines with a limit of 1 and
// measuring the round-trip latency.
func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	start := time.Now()

	token, err := c.getToken(ctx)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("token acquisition failed: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	endpoint := c.cfg.APIURL + "/api/machines?$top=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("defender: building health check request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("machines API unreachable: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	// Drain body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)

	latency := time.Since(start)

	status := "healthy"
	msg := "Defender for Endpoint reachable"
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		status = "degraded"
		msg = fmt.Sprintf("authentication issue (HTTP %d)", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		status = "unhealthy"
		msg = fmt.Sprintf("machines API returned HTTP %d", resp.StatusCode)
	}

	return &connectorsdk.HealthStatus{
		Status:    status,
		Message:   msg,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}, nil
}

// ValidateCredentials attempts to acquire an OAuth2 token. A nil return means
// the client_id / client_secret / tenant_id triple is valid.
func (c *Connector) ValidateCredentials(ctx context.Context) error {
	_, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("defender: credential validation failed: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// EventQuerier — Advanced Hunting
// ---------------------------------------------------------------------------

// advancedHuntingRequest is the payload for the MDE Advanced Hunting API.
type advancedHuntingRequest struct {
	Query string `json:"Query"`
}

// advancedHuntingResponse mirrors the MDE advanced hunting JSON shape.
type advancedHuntingResponse struct {
	Schema []struct {
		Name string `json:"Name"`
		Type string `json:"Type"`
	} `json:"Schema"`
	Results []map[string]json.RawMessage `json:"Results"`
}

// QueryEvents runs a KQL query against the MDE Advanced Hunting API and
// returns normalised events.
func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	kql := query.Query
	if kql == "" {
		return nil, fmt.Errorf("defender: query string (KQL) is required")
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("defender: obtaining token for query: %w", err)
	}

	payload, err := json.Marshal(advancedHuntingRequest{Query: kql})
	if err != nil {
		return nil, fmt.Errorf("defender: marshalling query payload: %w", err)
	}

	endpoint := c.cfg.APIURL + "/api/advancedqueries/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("defender: building advanced hunting request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("defender: advanced hunting request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("defender: reading advanced hunting response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("defender: advanced hunting returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}

	var huntResp advancedHuntingResponse
	if err := json.Unmarshal(body, &huntResp); err != nil {
		return nil, fmt.Errorf("defender: parsing advanced hunting response: %w", err)
	}

	events := make([]connectorsdk.Event, 0, len(huntResp.Results))
	for i, row := range huntResp.Results {
		evt := connectorsdk.Event{
			ID:       fmt.Sprintf("defender-ah-%d-%d", time.Now().UnixNano(), i),
			Source:   "defender",
			Metadata: make(map[string]string),
		}

		// Try to extract well-known columns.
		if ts, ok := row["Timestamp"]; ok {
			var tsStr string
			if json.Unmarshal(ts, &tsStr) == nil {
				if parsed, perr := time.Parse(time.RFC3339, tsStr); perr == nil {
					evt.Timestamp = parsed.UTC()
				}
			}
		}
		if evt.Timestamp.IsZero() {
			evt.Timestamp = time.Now().UTC()
		}

		if sev, ok := row["Severity"]; ok {
			var sevStr string
			if json.Unmarshal(sev, &sevStr) == nil {
				evt.Severity = sevStr
			}
		}

		if cat, ok := row["Category"]; ok {
			var catStr string
			if json.Unmarshal(cat, &catStr) == nil {
				evt.Category = catStr
			}
		}

		if title, ok := row["Title"]; ok {
			var titleStr string
			if json.Unmarshal(title, &titleStr) == nil {
				evt.Description = titleStr
			}
		}

		// Stash raw row.
		rawRow, _ := json.Marshal(row)
		evt.RawData = rawRow

		// Copy all string-valued columns into metadata for downstream use.
		for _, col := range huntResp.Schema {
			if raw, ok := row[col.Name]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil {
					evt.Metadata[col.Name] = s
				}
			}
		}

		events = append(events, evt)
	}

	maxResults := query.MaxResults
	truncated := false
	if maxResults > 0 && len(events) > maxResults {
		events = events[:maxResults]
		truncated = true
	}

	return &connectorsdk.EventResult{
		Events:     events,
		TotalCount: len(events),
		Truncated:  truncated,
	}, nil
}

// ---------------------------------------------------------------------------
// AssetFetcher — Machines
// ---------------------------------------------------------------------------

// machinesResponse mirrors the MDE list-machines OData response.
type machinesResponse struct {
	Value []json.RawMessage `json:"value"`
}

// machineRecord holds the subset of machine fields we normalise.
type machineRecord struct {
	ID                   string `json:"id"`
	ComputerDNSName      string `json:"computerDnsName"`
	OSPlatform           string `json:"osPlatform"`
	OSVersion            string `json:"osVersion"`
	HealthStatus         string `json:"healthStatus"`
	RiskScore            string `json:"riskScore"`
	ExposureLevel        string `json:"exposureLevel"`
	MachineTags          []string `json:"machineTags"`
	LastSeen             string `json:"lastSeen"`
	LastIPAddress        string `json:"lastIpAddress"`
	LastExternalIPAddress string `json:"lastExternalIpAddress"`
}

// FetchAssets retrieves machines from the MDE Machines API.
func (c *Connector) FetchAssets(ctx context.Context, filter connectorsdk.AssetFilter) (*connectorsdk.AssetResult, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("defender: obtaining token for assets: %w", err)
	}

	endpoint := c.cfg.APIURL + "/api/machines"

	// Build OData query parameters.
	params := url.Values{}
	if filter.MaxResults > 0 {
		params.Set("$top", strconv.Itoa(filter.MaxResults))
	}

	// Apply arbitrary OData filters from the filter map.
	if odata, ok := filter.Filters["$filter"]; ok && odata != "" {
		params.Set("$filter", odata)
	}

	if len(params) > 0 {
		endpoint = endpoint + "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("defender: building machines request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("defender: machines request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("defender: reading machines response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("defender: machines API returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}

	var machinesResp machinesResponse
	if err := json.Unmarshal(body, &machinesResp); err != nil {
		return nil, fmt.Errorf("defender: parsing machines response: %w", err)
	}

	assets := make([]connectorsdk.AssetRecord, 0, len(machinesResp.Value))
	for _, raw := range machinesResp.Value {
		var m machineRecord
		if err := json.Unmarshal(raw, &m); err != nil {
			continue // skip unparseable records
		}

		meta := map[string]string{
			"os_platform":    m.OSPlatform,
			"os_version":     m.OSVersion,
			"health_status":  m.HealthStatus,
			"risk_score":     m.RiskScore,
			"exposure_level": m.ExposureLevel,
			"last_seen":      m.LastSeen,
			"last_ip":        m.LastIPAddress,
			"last_external_ip": m.LastExternalIPAddress,
		}
		if len(m.MachineTags) > 0 {
			meta["tags"] = strings.Join(m.MachineTags, ",")
		}

		name := m.ComputerDNSName
		if name == "" {
			name = m.ID
		}

		assets = append(assets, connectorsdk.AssetRecord{
			ExternalID: m.ID,
			Name:       name,
			Type:       "endpoint",
			Metadata:   meta,
		})
	}

	return &connectorsdk.AssetResult{
		Assets:     assets,
		TotalCount: len(assets),
	}, nil
}

// ---------------------------------------------------------------------------
// DeepLinker
// ---------------------------------------------------------------------------

// GenerateDeepLink builds a URL into the Microsoft 365 Defender portal for the
// given entity.
func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	base := "https://security.microsoft.com"
	switch strings.ToLower(entityType) {
	case "alert":
		return fmt.Sprintf("%s/alerts/%s", base, entityID), nil
	case "machine", "device", "endpoint":
		return fmt.Sprintf("%s/machines/%s", base, entityID), nil
	case "incident":
		return fmt.Sprintf("%s/incidents/%s", base, entityID), nil
	default:
		return fmt.Sprintf("%s/alerts/%s", base, entityID), nil
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// truncate shortens s to at most maxLen runes, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
