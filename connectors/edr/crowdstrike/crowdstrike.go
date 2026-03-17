package crowdstrike

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

// Config holds the CrowdStrike Falcon connector configuration.
type Config struct {
	BaseURL      string `json:"base_url"`      // e.g. https://api.crowdstrike.com
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// tokenCache holds a cached OAuth2 access token and its expiry time.
type tokenCache struct {
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// Connector implements the CrowdStrike Falcon EDR connector.
type Connector struct {
	cfg         Config
	httpClient  *http.Client
	token       tokenCache
	initialized bool
}

// Compile-time interface assertions.
var (
	_ connectorsdk.Connector    = (*Connector)(nil)
	_ connectorsdk.EventQuerier = (*Connector)(nil)
	_ connectorsdk.AssetFetcher = (*Connector)(nil)
	_ connectorsdk.DeepLinker   = (*Connector)(nil)
)

// New returns a new, uninitialised CrowdStrike connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "crowdstrike" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryEDR }
func (c *Connector) Version() string                 { return "1.0.0" }

func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{
		connectorsdk.CapQueryEvents,
		connectorsdk.CapFetchAssets,
		connectorsdk.CapDeepLink,
	}
}

func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "base_url":      {"type": "string", "title": "Base URL",      "description": "CrowdStrike API base URL", "default": "https://api.crowdstrike.com"},
    "client_id":     {"type": "string", "title": "Client ID",     "description": "OAuth2 API client ID"},
    "client_secret": {"type": "string", "title": "Client Secret", "description": "OAuth2 API client secret", "format": "password"}
  },
  "required": ["client_id", "client_secret"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing crowdstrike config: %w", err)
	}

	if v, ok := cfg.Secrets["client_id"]; ok && v != "" {
		c.cfg.ClientID = v
	}
	if v, ok := cfg.Secrets["client_secret"]; ok && v != "" {
		c.cfg.ClientSecret = v
	}

	if c.cfg.ClientID == "" {
		return fmt.Errorf("crowdstrike config: client_id is required")
	}
	if c.cfg.ClientSecret == "" {
		return fmt.Errorf("crowdstrike config: client_secret is required")
	}
	if c.cfg.BaseURL == "" {
		c.cfg.BaseURL = "https://api.crowdstrike.com"
	}
	c.cfg.BaseURL = strings.TrimRight(c.cfg.BaseURL, "/")

	c.httpClient = &http.Client{Timeout: 30 * time.Second}
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
// OAuth2 token management
// ---------------------------------------------------------------------------

func (c *Connector) getToken(ctx context.Context) (string, error) {
	c.token.mu.Lock()
	defer c.token.mu.Unlock()

	if c.token.accessToken != "" && time.Now().Before(c.token.expiresAt.Add(-60*time.Second)) {
		return c.token.accessToken, nil
	}

	tokenURL := c.cfg.BaseURL + "/oauth2/token"

	form := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("crowdstrike: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("crowdstrike: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("crowdstrike: reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("crowdstrike: token endpoint returned %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("crowdstrike: parsing token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("crowdstrike: empty access_token in response")
	}

	c.token.accessToken = tokenResp.AccessToken
	c.token.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return c.token.accessToken, nil
}

// ---------------------------------------------------------------------------
// Health & validation
// ---------------------------------------------------------------------------

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

	// Use the /sensors/queries/sensors/v1 endpoint with limit=1 as a health probe.
	endpoint := c.cfg.BaseURL + "/sensors/queries/sensors/v1?limit=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: building health check request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("sensors API unreachable: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	status := "healthy"
	msg := "CrowdStrike Falcon reachable"
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		status = "degraded"
		msg = fmt.Sprintf("authentication issue (HTTP %d)", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		status = "unhealthy"
		msg = fmt.Sprintf("API returned HTTP %d", resp.StatusCode)
	}

	return &connectorsdk.HealthStatus{
		Status:    status,
		Message:   msg,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func (c *Connector) ValidateCredentials(ctx context.Context) error {
	_, err := c.getToken(ctx)
	if err != nil {
		return fmt.Errorf("crowdstrike: credential validation failed: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// EventQuerier — Detections
// ---------------------------------------------------------------------------

// detectionsResponse models the CrowdStrike detections query response.
type detectionsResponse struct {
	Resources []string `json:"resources"` // Detection IDs
}

// detectionDetail models a single detection detail from CrowdStrike.
type detectionDetail struct {
	DetectionID      string `json:"detection_id"`
	MaxSeverityName  string `json:"max_severity_displayname"`
	MaxConfidence    int    `json:"max_confidence"`
	FirstBehavior    string `json:"first_behavior"`
	LastBehavior     string `json:"last_behavior"`
	Status           string `json:"status"`
	HostName         string `json:"device,omitempty"`
	Behaviors        []struct {
		TacticID    string `json:"tactic_id"`
		Tactic      string `json:"tactic"`
		TechniqueID string `json:"technique_id"`
		Technique   string `json:"technique"`
		Description string `json:"description"`
	} `json:"behaviors"`
}

func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("crowdstrike connector not initialised: call Init first")
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: obtaining token for query: %w", err)
	}

	// Step 1: Query detection IDs.
	fql := query.Query
	if fql == "" {
		fql = "status:['new','in_progress']"
	}

	params := url.Values{"filter": {fql}}
	if query.MaxResults > 0 {
		params.Set("limit", strconv.Itoa(query.MaxResults))
	} else {
		params.Set("limit", "100")
	}

	endpoint := c.cfg.BaseURL + "/detects/queries/detects/v1?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: building detections query: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: detections query failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: reading detections response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crowdstrike: detections query returned HTTP %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var detectResp detectionsResponse
	if err := json.Unmarshal(body, &detectResp); err != nil {
		return nil, fmt.Errorf("crowdstrike: parsing detections response: %w", err)
	}

	if len(detectResp.Resources) == 0 {
		return &connectorsdk.EventResult{Events: nil, TotalCount: 0}, nil
	}

	// Step 2: Get detection details.
	detailPayload, _ := json.Marshal(map[string][]string{"ids": detectResp.Resources})
	detailReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/detects/entities/summaries/GET/v1", bytes.NewReader(detailPayload))
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: building detail request: %w", err)
	}
	detailReq.Header.Set("Authorization", "Bearer "+token)
	detailReq.Header.Set("Content-Type", "application/json")

	detailResp, err := c.httpClient.Do(detailReq)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: detail request failed: %w", err)
	}
	defer detailResp.Body.Close()

	detailBody, err := io.ReadAll(detailResp.Body)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: reading detail response: %w", err)
	}

	if detailResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crowdstrike: detail request returned HTTP %d: %s", detailResp.StatusCode, truncate(detailBody, 512))
	}

	var detailsWrapper struct {
		Resources []detectionDetail `json:"resources"`
	}
	if err := json.Unmarshal(detailBody, &detailsWrapper); err != nil {
		return nil, fmt.Errorf("crowdstrike: parsing details: %w", err)
	}

	events := make([]connectorsdk.Event, 0, len(detailsWrapper.Resources))
	for _, d := range detailsWrapper.Resources {
		evt := connectorsdk.Event{
			ID:       d.DetectionID,
			Source:   "crowdstrike",
			Severity: d.MaxSeverityName,
			Metadata: map[string]string{
				"status":     d.Status,
				"confidence": strconv.Itoa(d.MaxConfidence),
			},
		}

		if d.FirstBehavior != "" {
			if t, perr := time.Parse(time.RFC3339, d.FirstBehavior); perr == nil {
				evt.Timestamp = t.UTC()
			}
		}
		if evt.Timestamp.IsZero() {
			evt.Timestamp = time.Now().UTC()
		}

		if len(d.Behaviors) > 0 {
			evt.Category = d.Behaviors[0].Tactic
			evt.Description = d.Behaviors[0].Description
			evt.Metadata["technique_id"] = d.Behaviors[0].TechniqueID
			evt.Metadata["technique"] = d.Behaviors[0].Technique
		}

		rawBytes, _ := json.Marshal(d)
		evt.RawData = rawBytes

		events = append(events, evt)
	}

	return &connectorsdk.EventResult{
		Events:     events,
		TotalCount: len(events),
		Truncated:  query.MaxResults > 0 && len(events) >= query.MaxResults,
	}, nil
}

// ---------------------------------------------------------------------------
// AssetFetcher — Hosts
// ---------------------------------------------------------------------------

// hostRecord models a CrowdStrike host/device record.
type hostRecord struct {
	DeviceID           string   `json:"device_id"`
	Hostname           string   `json:"hostname"`
	LocalIP            string   `json:"local_ip"`
	ExternalIP         string   `json:"external_ip"`
	MacAddress         string   `json:"mac_address"`
	OSVersion          string   `json:"os_version"`
	PlatformName       string   `json:"platform_name"`
	Status             string   `json:"status"`
	LastSeen           string   `json:"last_seen"`
	Tags               []string `json:"tags"`
	AgentVersion       string   `json:"agent_version"`
	ReducedFunctionality string `json:"reduced_functionality_mode"`
}

func (c *Connector) FetchAssets(ctx context.Context, filter connectorsdk.AssetFilter) (*connectorsdk.AssetResult, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: obtaining token for hosts: %w", err)
	}

	// Query host IDs.
	params := url.Values{}
	if filter.MaxResults > 0 {
		params.Set("limit", strconv.Itoa(filter.MaxResults))
	} else {
		params.Set("limit", "100")
	}
	if fql, ok := filter.Filters["filter"]; ok && fql != "" {
		params.Set("filter", fql)
	}

	endpoint := c.cfg.BaseURL + "/devices/queries/devices/v1?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: building hosts query: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: hosts query failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: reading hosts response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crowdstrike: hosts query returned HTTP %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var idsResp struct {
		Resources []string `json:"resources"`
	}
	if err := json.Unmarshal(body, &idsResp); err != nil {
		return nil, fmt.Errorf("crowdstrike: parsing host IDs: %w", err)
	}

	if len(idsResp.Resources) == 0 {
		return &connectorsdk.AssetResult{Assets: nil, TotalCount: 0}, nil
	}

	// Get host details.
	detailPayload, _ := json.Marshal(map[string][]string{"ids": idsResp.Resources})
	detailReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/devices/entities/devices/v2", bytes.NewReader(detailPayload))
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: building host detail request: %w", err)
	}
	detailReq.Header.Set("Authorization", "Bearer "+token)
	detailReq.Header.Set("Content-Type", "application/json")

	detailResp, err := c.httpClient.Do(detailReq)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: host detail request failed: %w", err)
	}
	defer detailResp.Body.Close()

	detailBody, err := io.ReadAll(detailResp.Body)
	if err != nil {
		return nil, fmt.Errorf("crowdstrike: reading host detail response: %w", err)
	}

	var hostsResp struct {
		Resources []hostRecord `json:"resources"`
	}
	if err := json.Unmarshal(detailBody, &hostsResp); err != nil {
		return nil, fmt.Errorf("crowdstrike: parsing host details: %w", err)
	}

	assets := make([]connectorsdk.AssetRecord, 0, len(hostsResp.Resources))
	for _, h := range hostsResp.Resources {
		name := h.Hostname
		if name == "" {
			name = h.DeviceID
		}

		meta := map[string]string{
			"platform":      h.PlatformName,
			"os_version":    h.OSVersion,
			"status":        h.Status,
			"local_ip":      h.LocalIP,
			"external_ip":   h.ExternalIP,
			"last_seen":     h.LastSeen,
			"agent_version": h.AgentVersion,
		}
		if len(h.Tags) > 0 {
			meta["tags"] = strings.Join(h.Tags, ",")
		}

		assets = append(assets, connectorsdk.AssetRecord{
			ExternalID: h.DeviceID,
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

func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	base := "https://falcon.crowdstrike.com"
	switch strings.ToLower(entityType) {
	case "detection", "alert":
		return fmt.Sprintf("%s/activity/detections/detail/%s", base, entityID), nil
	case "host", "device", "endpoint":
		return fmt.Sprintf("%s/hosts/hosts/detail/%s", base, entityID), nil
	case "incident":
		return fmt.Sprintf("%s/incidents/detail/%s", base, entityID), nil
	default:
		return fmt.Sprintf("%s/activity/detections/detail/%s", base, entityID), nil
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func truncate(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "..."
}
