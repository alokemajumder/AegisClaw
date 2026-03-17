package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// priorityMap translates normalized priority names to Jira priority names.
var priorityMap = map[string]string{
	"critical": "Highest",
	"high":     "High",
	"medium":   "Medium",
	"low":      "Low",
}

// Config holds the Jira Service Management connector configuration.
type Config struct {
	InstanceURL string `json:"instance_url"` // e.g. https://mycompany.atlassian.net
	Email       string `json:"email"`        // Atlassian account email
	APIToken    string `json:"api_token"`    // Atlassian API token
	ProjectKey  string `json:"project_key"`  // Default project key (e.g. SEC)
	IssueType   string `json:"issue_type"`   // Default issue type (e.g. Task, Bug)
}

// Connector implements the Jira Service Management ITSM connector.
type Connector struct {
	cfg    Config
	client *http.Client
}

// Compile-time interface assertions.
var (
	_ connectorsdk.Connector    = (*Connector)(nil)
	_ connectorsdk.TicketManager = (*Connector)(nil)
	_ connectorsdk.DeepLinker   = (*Connector)(nil)
)

// New creates a new Jira connector instance.
func New() *Connector { return &Connector{} }

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (c *Connector) Type() string                    { return "jira" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryITSM }
func (c *Connector) Version() string                 { return "1.0.0" }

func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{
		connectorsdk.CapCreateTicket,
		connectorsdk.CapUpdateTicket,
		connectorsdk.CapDeepLink,
	}
}

func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "instance_url": {"type": "string", "title": "Instance URL",  "description": "Jira Cloud or Server URL (e.g. https://mycompany.atlassian.net)"},
    "email":        {"type": "string", "title": "Email",         "description": "Atlassian account email for API authentication"},
    "api_token":    {"type": "string", "title": "API Token",     "description": "Atlassian API token", "format": "password"},
    "project_key":  {"type": "string", "title": "Project Key",   "description": "Default Jira project key (e.g. SEC)"},
    "issue_type":   {"type": "string", "title": "Issue Type",    "description": "Default issue type for created tickets", "default": "Task"}
  },
  "required": ["instance_url", "email", "project_key"]
}`)
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing jira config: %w", err)
	}

	if v, ok := cfg.Secrets["email"]; ok && v != "" {
		c.cfg.Email = v
	}
	if v, ok := cfg.Secrets["api_token"]; ok && v != "" {
		c.cfg.APIToken = v
	}

	if c.cfg.InstanceURL == "" {
		return fmt.Errorf("jira config: instance_url is required")
	}
	c.cfg.InstanceURL = strings.TrimRight(c.cfg.InstanceURL, "/")

	if c.cfg.Email == "" {
		return fmt.Errorf("jira config: email is required (set in config or secrets)")
	}
	if c.cfg.APIToken == "" {
		return fmt.Errorf("jira config: api_token is required (set in config or secrets)")
	}
	if c.cfg.ProjectKey == "" {
		return fmt.Errorf("jira config: project_key is required")
	}
	if c.cfg.IssueType == "" {
		c.cfg.IssueType = "Task"
	}

	c.client = &http.Client{Timeout: 30 * time.Second}
	return nil
}

func (c *Connector) Close() error {
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Health & validation
// ---------------------------------------------------------------------------

func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	endpoint := c.cfg.InstanceURL + "/rest/api/3/myself"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("failed to build health check request: %v", err),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	c.setAuth(req)

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("Jira unreachable: %v", err),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	status := "healthy"
	message := fmt.Sprintf("Jira reachable (HTTP %d), latency %s", resp.StatusCode, latency.Round(time.Millisecond))

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
		return fmt.Errorf("jira credential validation failed: %w", err)
	}
	if hs.Status == "unhealthy" {
		return fmt.Errorf("jira credential validation failed: %s", hs.Message)
	}
	return nil
}

// ---------------------------------------------------------------------------
// TicketManager — Issue CRUD
// ---------------------------------------------------------------------------

// createIssuePayload is the Jira REST API v3 issue creation body.
type createIssuePayload struct {
	Fields issueFields `json:"fields"`
}

type issueFields struct {
	Project   projectRef `json:"project"`
	Summary   string     `json:"summary"`
	IssueType typeRef    `json:"issuetype"`
	Priority  *typeRef   `json:"priority,omitempty"`
	Labels    []string   `json:"labels,omitempty"`
	Description *adfDoc  `json:"description,omitempty"`
	Assignee  *userRef   `json:"assignee,omitempty"`
}

type projectRef struct {
	Key string `json:"key"`
}

type typeRef struct {
	Name string `json:"name"`
}

type userRef struct {
	AccountID string `json:"accountId"`
}

// adfDoc is a minimal Atlassian Document Format wrapper for descriptions.
type adfDoc struct {
	Type    string    `json:"type"`
	Version int       `json:"version"`
	Content []adfNode `json:"content"`
}

type adfNode struct {
	Type    string    `json:"type"`
	Content []adfText `json:"content,omitempty"`
}

type adfText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func makeADF(text string) *adfDoc {
	return &adfDoc{
		Type:    "doc",
		Version: 1,
		Content: []adfNode{
			{
				Type: "paragraph",
				Content: []adfText{
					{Type: "text", Text: text},
				},
			},
		},
	}
}

// createIssueResponse from Jira.
type createIssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

func (c *Connector) CreateTicket(ctx context.Context, ticket connectorsdk.TicketRequest) (*connectorsdk.TicketResult, error) {
	payload := createIssuePayload{
		Fields: issueFields{
			Project:   projectRef{Key: c.cfg.ProjectKey},
			Summary:   ticket.Title,
			IssueType: typeRef{Name: c.cfg.IssueType},
		},
	}

	if ticket.Description != "" {
		payload.Fields.Description = makeADF(ticket.Description)
	}

	if p, ok := priorityMap[strings.ToLower(ticket.Priority)]; ok {
		payload.Fields.Priority = &typeRef{Name: p}
	} else if ticket.Priority != "" {
		payload.Fields.Priority = &typeRef{Name: ticket.Priority}
	}

	if len(ticket.Labels) > 0 {
		payload.Fields.Labels = ticket.Labels
	} else {
		payload.Fields.Labels = []string{"aegisclaw", "security-validation"}
	}

	if ticket.Assignee != "" {
		payload.Fields.Assignee = &userRef{AccountID: ticket.Assignee}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("jira: marshalling issue payload: %w", err)
	}

	endpoint := c.cfg.InstanceURL + "/rest/api/3/issue"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jira: building create issue request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira: create issue request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jira: reading create issue response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira: create issue failed (HTTP %d): %s", resp.StatusCode, truncateBody(respBody, 512))
	}

	var issueResp createIssueResponse
	if err := json.Unmarshal(respBody, &issueResp); err != nil {
		return nil, fmt.Errorf("jira: parsing create issue response: %w", err)
	}

	ticketURL := fmt.Sprintf("%s/browse/%s", c.cfg.InstanceURL, issueResp.Key)

	return &connectorsdk.TicketResult{
		TicketID:  issueResp.Key,
		TicketURL: ticketURL,
		Status:    "open",
	}, nil
}

func (c *Connector) UpdateTicket(ctx context.Context, ticketID string, update connectorsdk.TicketUpdate) error {
	fields := make(map[string]any)
	hasFields := false

	if update.Priority != nil && *update.Priority != "" {
		if p, ok := priorityMap[strings.ToLower(*update.Priority)]; ok {
			fields["priority"] = map[string]string{"name": p}
		} else {
			fields["priority"] = map[string]string{"name": *update.Priority}
		}
		hasFields = true
	}

	if update.Comment != nil && *update.Comment != "" {
		// Add comment via separate API call.
		commentPayload, _ := json.Marshal(map[string]any{
			"body": makeADF(*update.Comment),
		})

		commentURL := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", c.cfg.InstanceURL, ticketID)
		commentReq, err := http.NewRequestWithContext(ctx, http.MethodPost, commentURL, bytes.NewReader(commentPayload))
		if err != nil {
			return fmt.Errorf("jira: building comment request: %w", err)
		}
		c.setAuth(commentReq)

		commentResp, err := c.client.Do(commentReq)
		if err != nil {
			return fmt.Errorf("jira: comment request failed: %w", err)
		}
		defer commentResp.Body.Close()
		io.Copy(io.Discard, commentResp.Body)

		if commentResp.StatusCode < 200 || commentResp.StatusCode >= 300 {
			return fmt.Errorf("jira: add comment failed (HTTP %d)", commentResp.StatusCode)
		}
	}

	// Map custom fields.
	for k, v := range update.CustomFields {
		fields[k] = v
		hasFields = true
	}

	if !hasFields {
		return nil
	}

	// Handle status transition via the transitions API.
	if update.Status != nil && *update.Status != "" {
		if err := c.transitionIssue(ctx, ticketID, *update.Status); err != nil {
			return fmt.Errorf("jira: transitioning issue: %w", err)
		}
	}

	body, err := json.Marshal(map[string]any{"fields": fields})
	if err != nil {
		return fmt.Errorf("jira: marshalling update payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s", c.cfg.InstanceURL, ticketID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("jira: building update request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("jira: update request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jira: update issue failed (HTTP %d)", resp.StatusCode)
	}

	return nil
}

// transitionIssue attempts to move an issue to the named status.
func (c *Connector) transitionIssue(ctx context.Context, ticketID, targetStatus string) error {
	// First, get available transitions.
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", c.cfg.InstanceURL, ticketID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var transitions struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"transitions"`
	}
	if err := json.Unmarshal(body, &transitions); err != nil {
		return err
	}

	// Find the transition matching the target status.
	for _, t := range transitions.Transitions {
		if strings.EqualFold(t.Name, targetStatus) {
			transPayload, _ := json.Marshal(map[string]any{
				"transition": map[string]string{"id": t.ID},
			})

			transReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(transPayload))
			if err != nil {
				return err
			}
			c.setAuth(transReq)

			transResp, err := c.client.Do(transReq)
			if err != nil {
				return err
			}
			defer transResp.Body.Close()
			io.Copy(io.Discard, transResp.Body)

			if transResp.StatusCode < 200 || transResp.StatusCode >= 300 {
				return fmt.Errorf("transition failed (HTTP %d)", transResp.StatusCode)
			}
			return nil
		}
	}

	return fmt.Errorf("no transition found for status %q", targetStatus)
}

// ---------------------------------------------------------------------------
// DeepLinker
// ---------------------------------------------------------------------------

func (c *Connector) GenerateDeepLink(_ context.Context, entityType, entityID string) (string, error) {
	return fmt.Sprintf("%s/browse/%s", c.cfg.InstanceURL, entityID), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (c *Connector) setAuth(req *http.Request) {
	req.SetBasicAuth(c.cfg.Email, c.cfg.APIToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}

func truncateBody(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "..."
}
