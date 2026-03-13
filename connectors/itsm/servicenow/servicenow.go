package servicenow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// priorityMap translates normalized priority names to ServiceNow integer values.
// ServiceNow uses 1=Critical, 2=High, 3=Medium, 4=Low.
var priorityMap = map[string]string{
	"critical": "1",
	"high":     "2",
	"medium":   "3",
	"low":      "4",
}

// Config holds the ServiceNow connector configuration.
type Config struct {
	InstanceURL string `json:"instance_url"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	APIVersion  string `json:"api_version"`
}

// Connector implements the ServiceNow ITSM connector.
type Connector struct {
	cfg    Config
	client *http.Client
}

// New creates a new ServiceNow connector instance.
func New() *Connector {
	return &Connector{}
}

// Type returns the connector type identifier.
func (c *Connector) Type() string { return "servicenow" }

// Category returns the connector category.
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryITSM }

// Version returns the connector version.
func (c *Connector) Version() string { return "1.0.0" }

// Capabilities returns the list of capabilities this connector supports.
func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{
		connectorsdk.CapCreateTicket,
		connectorsdk.CapUpdateTicket,
		connectorsdk.CapDeepLink,
	}
}

// ConfigSchema returns the JSON Schema describing the connector configuration.
func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"instance_url": {
				"type": "string",
				"title": "Instance URL",
				"description": "ServiceNow instance URL (e.g. https://mycompany.service-now.com)",
				"pattern": "^https://.+\\.service-now\\.com$"
			},
			"username": {
				"type": "string",
				"title": "Username",
				"description": "ServiceNow user account for API access"
			},
			"password": {
				"type": "string",
				"title": "Password",
				"description": "Password for the ServiceNow user account",
				"format": "password"
			},
			"api_version": {
				"type": "string",
				"title": "API Version",
				"description": "ServiceNow REST API version prefix (default: now)",
				"default": "now"
			}
		},
		"required": ["instance_url", "username"]
	}`)
}

// Init initializes the connector with the provided configuration.
// Credentials are resolved from the config JSON first, then overridden by
// any values present in cfg.Secrets (keys: "username", "password").
func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing servicenow config: %w", err)
	}

	// Allow secrets map to override username/password so that credentials
	// can be managed through the platform's secret store rather than
	// being embedded in the config JSON.
	if v, ok := cfg.Secrets["username"]; ok && v != "" {
		c.cfg.Username = v
	}
	if v, ok := cfg.Secrets["password"]; ok && v != "" {
		c.cfg.Password = v
	}

	if c.cfg.InstanceURL == "" {
		return fmt.Errorf("servicenow config: instance_url is required")
	}
	// Strip trailing slash for consistent URL construction.
	c.cfg.InstanceURL = strings.TrimRight(c.cfg.InstanceURL, "/")

	if c.cfg.Username == "" {
		return fmt.Errorf("servicenow config: username is required (set in config or secrets)")
	}
	if c.cfg.Password == "" {
		return fmt.Errorf("servicenow config: password is required (set in config or secrets)")
	}

	if c.cfg.APIVersion == "" {
		c.cfg.APIVersion = "now"
	}

	c.client = &http.Client{
		Timeout: 30 * time.Second,
	}

	return nil
}

// Close releases any resources held by the connector.
func (c *Connector) Close() error {
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	return nil
}

// HealthCheck verifies connectivity to the ServiceNow instance by querying
// the sys_properties table with a single-row limit. It measures round-trip
// latency and returns an appropriate health status.
func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	url := fmt.Sprintf("%s/api/%s/table/sys_properties?sysparm_limit=1", c.cfg.InstanceURL, c.cfg.APIVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("failed to build health check request: %v", err),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	c.setHeaders(req)

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start)

	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("health check request failed: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	// Drain body to allow connection reuse.
	io.Copy(io.Discard, resp.Body)

	status := "healthy"
	message := fmt.Sprintf("ServiceNow instance reachable (%d), latency %s", resp.StatusCode, latency.Round(time.Millisecond))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		status = "unhealthy"
		message = fmt.Sprintf("authentication failed (HTTP %d)", resp.StatusCode)
	} else if resp.StatusCode >= 400 {
		status = "degraded"
		message = fmt.Sprintf("unexpected response (HTTP %d)", resp.StatusCode)
	} else if latency > 5*time.Second {
		status = "degraded"
		message = fmt.Sprintf("ServiceNow instance reachable but slow (%s)", latency.Round(time.Millisecond))
	}

	return &connectorsdk.HealthStatus{
		Status:    status,
		Message:   message,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}, nil
}

// ValidateCredentials verifies that the configured username and password are
// accepted by the ServiceNow instance. It performs a lightweight GET against
// the sys_user table filtered to the configured username.
func (c *Connector) ValidateCredentials(ctx context.Context) error {
	url := fmt.Sprintf(
		"%s/api/%s/table/sys_user?sysparm_limit=1&sysparm_fields=user_name&user_name=%s",
		c.cfg.InstanceURL, c.cfg.APIVersion, c.cfg.Username,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building credentials validation request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("credentials validation request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("invalid credentials: ServiceNow returned HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("credentials validation failed: ServiceNow returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// incidentPayload is the JSON body sent to ServiceNow when creating or
// updating an incident record.
type incidentPayload struct {
	ShortDescription string `json:"short_description,omitempty"`
	Description      string `json:"description,omitempty"`
	Priority         string `json:"priority,omitempty"`
	Urgency          string `json:"urgency,omitempty"`
	Impact           string `json:"impact,omitempty"`
	AssignedTo       string `json:"assigned_to,omitempty"`
	State            string `json:"state,omitempty"`
	Comments         string `json:"comments,omitempty"`
}

// incidentResponse represents the ServiceNow response envelope for a single
// incident record.
type incidentResponse struct {
	Result struct {
		SysID  string `json:"sys_id"`
		Number string `json:"number"`
		State  string `json:"state"`
	} `json:"result"`
}

// CreateTicket creates a new incident in ServiceNow by POSTing to the
// incident table API. It maps the normalized TicketRequest fields to
// ServiceNow incident fields and returns the resulting sys_id and number.
func (c *Connector) CreateTicket(ctx context.Context, ticket connectorsdk.TicketRequest) (*connectorsdk.TicketResult, error) {
	payload := incidentPayload{
		ShortDescription: ticket.Title,
		Description:      ticket.Description,
	}

	// Map normalized priority to ServiceNow priority/urgency/impact values.
	if p, ok := priorityMap[strings.ToLower(ticket.Priority)]; ok {
		payload.Priority = p
		payload.Urgency = p
		payload.Impact = p
	} else if ticket.Priority != "" {
		// If the caller sent a raw numeric value, pass it through.
		payload.Priority = ticket.Priority
	}

	if ticket.Assignee != "" {
		payload.AssignedTo = ticket.Assignee
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling incident payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/table/incident", c.cfg.InstanceURL, c.cfg.APIVersion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building create incident request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create incident request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading create incident response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("create incident failed (HTTP %d): %s", resp.StatusCode, truncateBody(respBody, 512))
	}

	var incResp incidentResponse
	if err := json.Unmarshal(respBody, &incResp); err != nil {
		return nil, fmt.Errorf("parsing create incident response: %w", err)
	}

	if incResp.Result.SysID == "" {
		return nil, fmt.Errorf("create incident returned empty sys_id")
	}

	ticketURL := fmt.Sprintf("%s/nav_to.do?uri=incident.do%%3Fsys_id=%s", c.cfg.InstanceURL, incResp.Result.SysID)

	status := "new"
	if incResp.Result.State != "" {
		status = incResp.Result.State
	}

	return &connectorsdk.TicketResult{
		TicketID:  incResp.Result.SysID,
		TicketURL: ticketURL,
		Status:    status,
	}, nil
}

// updatePayload is the JSON body sent when patching an incident.
type updatePayload struct {
	State            string `json:"state,omitempty"`
	Priority         string `json:"priority,omitempty"`
	Urgency          string `json:"urgency,omitempty"`
	Impact           string `json:"impact,omitempty"`
	Comments         string `json:"comments,omitempty"`
	ShortDescription string `json:"short_description,omitempty"`
}

// UpdateTicket updates an existing incident in ServiceNow by PATCHing the
// incident table record identified by ticketID (sys_id).
func (c *Connector) UpdateTicket(ctx context.Context, ticketID string, update connectorsdk.TicketUpdate) error {
	payload := updatePayload{}
	hasFields := false

	if update.Status != nil && *update.Status != "" {
		payload.State = *update.Status
		hasFields = true
	}

	if update.Priority != nil && *update.Priority != "" {
		if p, ok := priorityMap[strings.ToLower(*update.Priority)]; ok {
			payload.Priority = p
			payload.Urgency = p
			payload.Impact = p
		} else {
			payload.Priority = *update.Priority
		}
		hasFields = true
	}

	if update.Comment != nil && *update.Comment != "" {
		payload.Comments = *update.Comment
		hasFields = true
	}

	// Map custom fields to supported ServiceNow incident fields.
	if v, ok := update.CustomFields["short_description"]; ok {
		payload.ShortDescription = v
		hasFields = true
	}

	if !hasFields {
		return nil // Nothing to update.
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling update payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/%s/table/incident/%s", c.cfg.InstanceURL, c.cfg.APIVersion, ticketID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building update incident request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("update incident request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading update incident response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("update incident failed (HTTP %d): %s", resp.StatusCode, truncateBody(respBody, 512))
	}

	return nil
}

// GenerateDeepLink generates a navigable URL to a ServiceNow record.
func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	return fmt.Sprintf("%s/nav_to.do?uri=%s.do%%3Fsys_id=%s", c.cfg.InstanceURL, entityType, entityID), nil
}

// setHeaders applies the standard ServiceNow REST API headers including
// Basic authentication to the given request.
func (c *Connector) setHeaders(req *http.Request) {
	creds := base64.StdEncoding.EncodeToString([]byte(c.cfg.Username + ":" + c.cfg.Password))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

// truncateBody returns the first n bytes of a response body as a string,
// appending "..." if truncation occurred. This is used for error messages
// to avoid logging excessively large response bodies.
func truncateBody(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "..."
}
