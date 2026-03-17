package okta

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// Config holds the Okta connector configuration.
type Config struct {
	OrgURL   string `json:"org_url"`   // e.g. https://mycompany.okta.com
	APIToken string `json:"api_token"` // Okta API token (SSWS)
}

// Connector implements the Okta Identity connector.
type Connector struct {
	cfg         Config
	httpClient  *http.Client
	initialized bool
}

// Compile-time interface assertions.
var (
	_ connectorsdk.Connector    = (*Connector)(nil)
	_ connectorsdk.AssetFetcher = (*Connector)(nil)
	_ connectorsdk.EventQuerier = (*Connector)(nil)
	_ connectorsdk.DeepLinker   = (*Connector)(nil)
)

// New returns a new, uninitialised Okta connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "okta" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryIdentity }
func (c *Connector) Version() string                 { return "1.0.0" }

func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{
		connectorsdk.CapFetchAssets,
		connectorsdk.CapQueryEvents,
		connectorsdk.CapDeepLink,
	}
}

func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "org_url":   {"type": "string", "title": "Organization URL", "description": "Okta org URL (e.g. https://mycompany.okta.com)"},
    "api_token": {"type": "string", "title": "API Token",        "description": "Okta API token (SSWS)", "format": "password"}
  },
  "required": ["org_url", "api_token"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing okta config: %w", err)
	}

	if v, ok := cfg.Secrets["api_token"]; ok && v != "" {
		c.cfg.APIToken = v
	}

	if c.cfg.OrgURL == "" {
		return fmt.Errorf("okta config: org_url is required")
	}
	c.cfg.OrgURL = strings.TrimRight(c.cfg.OrgURL, "/")

	if c.cfg.APIToken == "" {
		return fmt.Errorf("okta config: api_token is required (set in config or secrets)")
	}

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
// Health & validation
// ---------------------------------------------------------------------------

func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	start := time.Now()

	endpoint := c.cfg.OrgURL + "/api/v1/org"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
			Message:   fmt.Sprintf("Okta unreachable: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	status := "healthy"
	message := fmt.Sprintf("Okta reachable (HTTP %d), latency %s", resp.StatusCode, latency.Round(time.Millisecond))

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
		return fmt.Errorf("okta credential validation failed: %w", err)
	}
	if hs.Status == "unhealthy" {
		return fmt.Errorf("okta credential validation failed: %s", hs.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// AssetFetcher — Users and Groups
// ---------------------------------------------------------------------------

// oktaUser models a subset of the Okta user response.
type oktaUser struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Profile struct {
		FirstName   string `json:"firstName"`
		LastName    string `json:"lastName"`
		Email       string `json:"email"`
		Login       string `json:"login"`
		Department  string `json:"department"`
		Title       string `json:"title"`
		MobilePhone string `json:"mobilePhone"`
	} `json:"profile"`
	LastLogin   *string `json:"lastLogin"`
	Created     string  `json:"created"`
	LastUpdated string  `json:"lastUpdated"`
}

func (c *Connector) FetchAssets(ctx context.Context, filter connectorsdk.AssetFilter) (*connectorsdk.AssetResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("okta connector not initialised: call Init first")
	}

	params := url.Values{}
	if filter.MaxResults > 0 {
		params.Set("limit", strconv.Itoa(filter.MaxResults))
	} else {
		params.Set("limit", "200")
	}

	// Support a search filter via Okta's expression language.
	if search, ok := filter.Filters["search"]; ok && search != "" {
		params.Set("search", search)
	}

	endpoint := c.cfg.OrgURL + "/api/v1/users?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("okta: building users request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("okta: users request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("okta: reading users response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("okta: users API returned HTTP %d: %s", resp.StatusCode, truncateBody(body, 512))
	}

	var users []oktaUser
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("okta: parsing users response: %w", err)
	}

	assets := make([]connectorsdk.AssetRecord, 0, len(users))
	for _, u := range users {
		name := u.Profile.FirstName + " " + u.Profile.LastName
		if name == " " {
			name = u.Profile.Login
		}

		meta := map[string]string{
			"email":       u.Profile.Email,
			"login":       u.Profile.Login,
			"status":      u.Status,
			"department":  u.Profile.Department,
			"title":       u.Profile.Title,
			"created":     u.Created,
			"last_updated": u.LastUpdated,
		}
		if u.LastLogin != nil {
			meta["last_login"] = *u.LastLogin
		}

		assets = append(assets, connectorsdk.AssetRecord{
			ExternalID: u.ID,
			Name:       name,
			Type:       "user",
			Metadata:   meta,
		})
	}

	return &connectorsdk.AssetResult{
		Assets:     assets,
		TotalCount: len(assets),
	}, nil
}

// ---------------------------------------------------------------------------
// EventQuerier — System Log
// ---------------------------------------------------------------------------

// oktaLogEvent models a single Okta System Log event.
type oktaLogEvent struct {
	UUID        string `json:"uuid"`
	Published   string `json:"published"`
	EventType   string `json:"eventType"`
	Severity    string `json:"severity"`
	DisplayMessage string `json:"displayMessage"`
	Actor       struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		DisplayName string `json:"displayName"`
	} `json:"actor"`
	Outcome struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
	} `json:"outcome"`
	Target []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		DisplayName string `json:"displayName"`
	} `json:"target"`
}

func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("okta connector not initialised: call Init first")
	}

	params := url.Values{}
	if query.MaxResults > 0 {
		params.Set("limit", strconv.Itoa(query.MaxResults))
	} else {
		params.Set("limit", "100")
	}

	// Okta System Log supports filter expression.
	if query.Query != "" {
		params.Set("filter", query.Query)
	}

	if !query.TimeRange.Start.IsZero() {
		params.Set("since", query.TimeRange.Start.Format(time.RFC3339))
	}
	if !query.TimeRange.End.IsZero() {
		params.Set("until", query.TimeRange.End.Format(time.RFC3339))
	}

	endpoint := c.cfg.OrgURL + "/api/v1/logs?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("okta: building logs request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("okta: logs request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("okta: reading logs response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("okta: logs API returned HTTP %d: %s", resp.StatusCode, truncateBody(body, 512))
	}

	var logEvents []oktaLogEvent
	if err := json.Unmarshal(body, &logEvents); err != nil {
		return nil, fmt.Errorf("okta: parsing logs response: %w", err)
	}

	events := make([]connectorsdk.Event, 0, len(logEvents))
	for _, le := range logEvents {
		evt := connectorsdk.Event{
			ID:          le.UUID,
			Source:      "okta",
			Category:    le.EventType,
			Severity:    le.Severity,
			Description: le.DisplayMessage,
			Metadata: map[string]string{
				"actor_id":   le.Actor.ID,
				"actor_type": le.Actor.Type,
				"actor_name": le.Actor.DisplayName,
				"outcome":    le.Outcome.Result,
			},
		}

		if t, err := time.Parse(time.RFC3339, le.Published); err == nil {
			evt.Timestamp = t.UTC()
		} else {
			evt.Timestamp = time.Now().UTC()
		}

		if len(le.Target) > 0 {
			evt.Metadata["target_id"] = le.Target[0].ID
			evt.Metadata["target_type"] = le.Target[0].Type
			evt.Metadata["target_name"] = le.Target[0].DisplayName
		}

		rawBytes, _ := json.Marshal(le)
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
// DeepLinker
// ---------------------------------------------------------------------------

func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	switch strings.ToLower(entityType) {
	case "user":
		return fmt.Sprintf("%s/admin/user/profile/view/%s", c.cfg.OrgURL, entityID), nil
	case "group":
		return fmt.Sprintf("%s/admin/group/%s", c.cfg.OrgURL, entityID), nil
	case "app", "application":
		return fmt.Sprintf("%s/admin/app/%s/instance/%s", c.cfg.OrgURL, entityType, entityID), nil
	default:
		return fmt.Sprintf("%s/admin/user/profile/view/%s", c.cfg.OrgURL, entityID), nil
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (c *Connector) setAuth(req *http.Request) {
	req.Header.Set("Authorization", "SSWS "+c.cfg.APIToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
}

func truncateBody(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "..."
}
