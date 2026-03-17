package entraid

import (
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

// Config holds the Microsoft Entra ID (Azure AD) connector configuration.
type Config struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// tokenCache holds a cached OAuth2 access token and its expiry time.
type tokenCache struct {
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// Connector implements the Microsoft Entra ID Identity connector.
type Connector struct {
	cfg         Config
	httpClient  *http.Client
	token       tokenCache
	initialized bool
}

// Compile-time interface assertions.
var (
	_ connectorsdk.Connector    = (*Connector)(nil)
	_ connectorsdk.AssetFetcher = (*Connector)(nil)
	_ connectorsdk.EventQuerier = (*Connector)(nil)
	_ connectorsdk.DeepLinker   = (*Connector)(nil)
)

// New returns a new, uninitialised Entra ID connector.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "entraid" }
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
    "tenant_id":     {"type": "string", "title": "Tenant ID",     "description": "Azure AD / Entra tenant GUID"},
    "client_id":     {"type": "string", "title": "Client ID",     "description": "App registration client ID"},
    "client_secret": {"type": "string", "title": "Client Secret", "description": "App registration client secret", "format": "password"}
  },
  "required": ["tenant_id", "client_id", "client_secret"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing entraid config: %w", err)
	}

	if v, ok := cfg.Secrets["client_id"]; ok && v != "" {
		c.cfg.ClientID = v
	}
	if v, ok := cfg.Secrets["client_secret"]; ok && v != "" {
		c.cfg.ClientSecret = v
	}

	if c.cfg.TenantID == "" {
		return fmt.Errorf("entraid config: tenant_id is required")
	}
	if c.cfg.ClientID == "" {
		return fmt.Errorf("entraid config: client_id is required (config or secrets)")
	}
	if c.cfg.ClientSecret == "" {
		return fmt.Errorf("entraid config: client_secret is required (config or secrets)")
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
// OAuth2 token management (Microsoft Identity Platform)
// ---------------------------------------------------------------------------

func (c *Connector) getToken(ctx context.Context) (string, error) {
	c.token.mu.Lock()
	defer c.token.mu.Unlock()

	if c.token.accessToken != "" && time.Now().Before(c.token.expiresAt.Add(-60*time.Second)) {
		return c.token.accessToken, nil
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", c.cfg.TenantID)

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"scope":         {"https://graph.microsoft.com/.default"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("entraid: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("entraid: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("entraid: reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("entraid: token endpoint returned %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("entraid: parsing token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("entraid: empty access_token in response")
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

	// Use /v1.0/organization as a lightweight health probe.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.microsoft.com/v1.0/organization?$top=1", nil)
	if err != nil {
		return nil, fmt.Errorf("entraid: building health check request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("Graph API unreachable: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	status := "healthy"
	msg := "Entra ID / Graph API reachable"
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		status = "degraded"
		msg = fmt.Sprintf("authentication issue (HTTP %d)", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		status = "unhealthy"
		msg = fmt.Sprintf("Graph API returned HTTP %d", resp.StatusCode)
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
		return fmt.Errorf("entraid: credential validation failed: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// AssetFetcher — Users via Microsoft Graph
// ---------------------------------------------------------------------------

// graphUser models a subset of the Microsoft Graph user resource.
type graphUser struct {
	ID                string  `json:"id"`
	DisplayName       string  `json:"displayName"`
	UserPrincipalName string  `json:"userPrincipalName"`
	Mail              string  `json:"mail"`
	JobTitle          string  `json:"jobTitle"`
	Department        string  `json:"department"`
	AccountEnabled    bool    `json:"accountEnabled"`
	CreatedDateTime   string  `json:"createdDateTime"`
	LastSignIn        *struct {
		DateTime string `json:"dateTime"`
	} `json:"signInActivity,omitempty"`
}

func (c *Connector) FetchAssets(ctx context.Context, filter connectorsdk.AssetFilter) (*connectorsdk.AssetResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("entraid connector not initialised: call Init first")
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("entraid: obtaining token for users: %w", err)
	}

	params := url.Values{
		"$select": {"id,displayName,userPrincipalName,mail,jobTitle,department,accountEnabled,createdDateTime"},
	}
	if filter.MaxResults > 0 {
		params.Set("$top", strconv.Itoa(filter.MaxResults))
	} else {
		params.Set("$top", "100")
	}
	if odata, ok := filter.Filters["$filter"]; ok && odata != "" {
		params.Set("$filter", odata)
	}

	endpoint := "https://graph.microsoft.com/v1.0/users?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("entraid: building users request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("ConsistencyLevel", "eventual")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entraid: users request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("entraid: reading users response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("entraid: users API returned HTTP %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var usersResp struct {
		Value []graphUser `json:"value"`
	}
	if err := json.Unmarshal(body, &usersResp); err != nil {
		return nil, fmt.Errorf("entraid: parsing users response: %w", err)
	}

	assets := make([]connectorsdk.AssetRecord, 0, len(usersResp.Value))
	for _, u := range usersResp.Value {
		name := u.DisplayName
		if name == "" {
			name = u.UserPrincipalName
		}

		meta := map[string]string{
			"upn":        u.UserPrincipalName,
			"mail":       u.Mail,
			"job_title":  u.JobTitle,
			"department": u.Department,
			"enabled":    strconv.FormatBool(u.AccountEnabled),
			"created":    u.CreatedDateTime,
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
// EventQuerier — Sign-in Logs via Microsoft Graph
// ---------------------------------------------------------------------------

// signInLog models a Microsoft Graph sign-in log entry.
type signInLog struct {
	ID                  string `json:"id"`
	CreatedDateTime     string `json:"createdDateTime"`
	UserDisplayName     string `json:"userDisplayName"`
	UserPrincipalName   string `json:"userPrincipalName"`
	UserID              string `json:"userId"`
	AppDisplayName      string `json:"appDisplayName"`
	IPAddress           string `json:"ipAddress"`
	ClientAppUsed       string `json:"clientAppUsed"`
	ConditionalAccessStatus string `json:"conditionalAccessStatus"`
	IsInteractive       bool   `json:"isInteractive"`
	RiskDetail          string `json:"riskDetail"`
	RiskLevelAggregated string `json:"riskLevelAggregated"`
	RiskLevelDuringSignIn string `json:"riskLevelDuringSignIn"`
	RiskState           string `json:"riskState"`
	Status              struct {
		ErrorCode    int    `json:"errorCode"`
		FailureReason string `json:"failureReason"`
	} `json:"status"`
	Location struct {
		City    string `json:"city"`
		State   string `json:"state"`
		Country string `json:"countryOrRegion"`
	} `json:"location"`
}

func (c *Connector) QueryEvents(ctx context.Context, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("entraid connector not initialised: call Init first")
	}

	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("entraid: obtaining token for sign-in logs: %w", err)
	}

	params := url.Values{}
	if query.MaxResults > 0 {
		params.Set("$top", strconv.Itoa(query.MaxResults))
	} else {
		params.Set("$top", "100")
	}
	params.Set("$orderby", "createdDateTime desc")

	// Use the caller's query as an OData $filter.
	if query.Query != "" {
		params.Set("$filter", query.Query)
	} else if !query.TimeRange.Start.IsZero() {
		params.Set("$filter", fmt.Sprintf("createdDateTime ge %s", query.TimeRange.Start.Format(time.RFC3339)))
	}

	endpoint := "https://graph.microsoft.com/v1.0/auditLogs/signIns?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("entraid: building sign-in logs request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entraid: sign-in logs request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("entraid: reading sign-in logs response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("entraid: sign-in logs returned HTTP %d: %s", resp.StatusCode, truncate(body, 512))
	}

	var logsResp struct {
		Value []signInLog `json:"value"`
	}
	if err := json.Unmarshal(body, &logsResp); err != nil {
		return nil, fmt.Errorf("entraid: parsing sign-in logs: %w", err)
	}

	events := make([]connectorsdk.Event, 0, len(logsResp.Value))
	for _, si := range logsResp.Value {
		severity := "info"
		if si.RiskLevelAggregated == "high" || si.RiskLevelDuringSignIn == "high" {
			severity = "high"
		} else if si.RiskLevelAggregated == "medium" || si.RiskLevelDuringSignIn == "medium" {
			severity = "medium"
		} else if si.RiskLevelAggregated == "low" || si.RiskLevelDuringSignIn == "low" {
			severity = "low"
		} else if si.Status.ErrorCode != 0 {
			severity = "medium"
		}

		description := fmt.Sprintf("%s signed in to %s", si.UserDisplayName, si.AppDisplayName)
		if si.Status.FailureReason != "" {
			description = fmt.Sprintf("%s (failed: %s)", description, si.Status.FailureReason)
		}

		evt := connectorsdk.Event{
			ID:          si.ID,
			Source:      "entraid",
			Severity:    severity,
			Category:    "sign-in",
			Description: description,
			Metadata: map[string]string{
				"user_id":            si.UserID,
				"upn":               si.UserPrincipalName,
				"app":               si.AppDisplayName,
				"ip_address":        si.IPAddress,
				"client_app":        si.ClientAppUsed,
				"risk_level":        si.RiskLevelAggregated,
				"risk_state":        si.RiskState,
				"conditional_access": si.ConditionalAccessStatus,
				"location":          fmt.Sprintf("%s, %s, %s", si.Location.City, si.Location.State, si.Location.Country),
			},
		}

		if t, perr := time.Parse(time.RFC3339, si.CreatedDateTime); perr == nil {
			evt.Timestamp = t.UTC()
		} else {
			evt.Timestamp = time.Now().UTC()
		}

		rawBytes, _ := json.Marshal(si)
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
		return fmt.Sprintf("https://entra.microsoft.com/#view/Microsoft_AAD_UsersAndTenants/UserProfileMenuBlade/~/overview/userId/%s", entityID), nil
	case "group":
		return fmt.Sprintf("https://entra.microsoft.com/#view/Microsoft_AAD_IAM/GroupDetailsMenuBlade/~/Overview/groupId/%s", entityID), nil
	case "application", "app":
		return fmt.Sprintf("https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationMenuBlade/~/Overview/appId/%s", entityID), nil
	case "signin", "sign-in":
		return fmt.Sprintf("https://entra.microsoft.com/#view/Microsoft_AAD_IAM/SignInLogsMenuBlade/~/Details/signInId/%s", entityID), nil
	default:
		return fmt.Sprintf("https://entra.microsoft.com/#view/Microsoft_AAD_UsersAndTenants/UserProfileMenuBlade/~/overview/userId/%s", entityID), nil
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
