package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

type Connector struct {
	cfg    Config
	client *http.Client
}

type Config struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel"`
	BotToken   string `json:"bot_token"`
}

func New() *Connector {
	return &Connector{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Connector) Type() string                    { return "slack" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryNotification }
func (c *Connector) Version() string                 { return "1.0.0" }
func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{connectorsdk.CapSendNotification}
}

func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"webhook_url":{"type":"string"},"channel":{"type":"string"},"bot_token":{"type":"string"}},"required":["webhook_url"]}`)
}

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing slack config: %w", err)
	}
	return nil
}

func (c *Connector) Close() error { return nil }

func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	if c.cfg.WebhookURL == "" {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   "Slack webhook URL not configured",
			Latency:   0,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.cfg.WebhookURL, nil)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("failed to create health check request: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("Slack webhook unreachable: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	// Slack webhooks may return 405 for HEAD but that still proves reachability.
	if resp.StatusCode >= 500 {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("Slack webhook returned server error: %d", resp.StatusCode),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	status := "healthy"
	msg := "Slack webhook reachable"
	if latency > 5*time.Second {
		status = "degraded"
		msg = fmt.Sprintf("Slack webhook reachable but slow (%s)", latency)
	}

	return &connectorsdk.HealthStatus{
		Status:    status,
		Message:   msg,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func (c *Connector) ValidateCredentials(ctx context.Context) error {
	if c.cfg.WebhookURL == "" {
		return fmt.Errorf("validating slack credentials: webhook URL is empty")
	}

	parsed, err := url.ParseRequestURI(c.cfg.WebhookURL)
	if err != nil {
		return fmt.Errorf("validating slack credentials: invalid webhook URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("validating slack credentials: webhook URL must use HTTPS")
	}

	// Slack webhook URLs are hosted on hooks.slack.com
	if !strings.Contains(parsed.Host, "hooks.slack.com") {
		return fmt.Errorf("validating slack credentials: webhook URL host %q does not appear to be a Slack endpoint", parsed.Host)
	}

	// Verify the webhook endpoint is reachable
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.cfg.WebhookURL, nil)
	if err != nil {
		return fmt.Errorf("validating slack credentials: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("validating slack credentials: webhook unreachable: %w", err)
	}
	defer resp.Body.Close()

	// 405 Method Not Allowed is expected for HEAD on webhook endpoints;
	// anything below 500 confirms the endpoint exists.
	if resp.StatusCode >= 500 {
		return fmt.Errorf("validating slack credentials: webhook returned server error %d", resp.StatusCode)
	}

	return nil
}

func (c *Connector) SendNotification(ctx context.Context, notif connectorsdk.NotificationRequest) error {
	emoji := severityEmoji(notif.Severity)
	payload := map[string]any{
		"blocks": []map[string]any{
			{
				"type": "header",
				"text": map[string]string{"type": "plain_text", "text": emoji + " " + notif.Title},
			},
			{
				"type": "section",
				"text": map[string]string{"type": "mrkdwn", "text": notif.Message},
			},
			{
				"type": "context",
				"elements": []map[string]string{
					{"type": "mrkdwn", "text": fmt.Sprintf("*Severity:* %s | *Source:* AegisClaw", notif.Severity)},
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func severityEmoji(severity string) string {
	switch severity {
	case "critical":
		return ":rotating_light:"
	case "high":
		return ":warning:"
	case "medium":
		return ":large_orange_diamond:"
	case "low":
		return ":information_source:"
	default:
		return ":mag:"
	}
}
