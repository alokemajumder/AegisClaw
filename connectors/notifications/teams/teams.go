package teams

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
	WebhookURL  string `json:"webhook_url"`
	ChannelName string `json:"channel_name"`
}

func New() *Connector {
	return &Connector{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Connector) Type() string                    { return "teams" }
func (c *Connector) Category() connectorsdk.Category { return connectorsdk.CategoryNotification }
func (c *Connector) Version() string                 { return "1.0.0" }
func (c *Connector) Capabilities() []connectorsdk.Capability {
	return []connectorsdk.Capability{connectorsdk.CapSendNotification}
}

func (c *Connector) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"webhook_url":{"type":"string"},"channel_name":{"type":"string"}},"required":["webhook_url"]}`)
}

func (c *Connector) Init(_ context.Context, cfg connectorsdk.ConnectorConfig) error {
	if err := json.Unmarshal(cfg.Config, &c.cfg); err != nil {
		return fmt.Errorf("parsing teams config: %w", err)
	}
	return nil
}

func (c *Connector) Close() error { return nil }

func (c *Connector) HealthCheck(ctx context.Context) (*connectorsdk.HealthStatus, error) {
	if c.cfg.WebhookURL == "" {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   "Teams webhook URL not configured",
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
			Message:   fmt.Sprintf("Teams webhook unreachable: %v", err),
			Latency:   time.Since(start),
			CheckedAt: time.Now().UTC(),
		}, nil
	}
	defer resp.Body.Close()
	latency := time.Since(start)

	// Teams webhooks may return 405 for HEAD but that still proves reachability.
	if resp.StatusCode >= 500 {
		return &connectorsdk.HealthStatus{
			Status:    "unhealthy",
			Message:   fmt.Sprintf("Teams webhook returned server error: %d", resp.StatusCode),
			Latency:   latency,
			CheckedAt: time.Now().UTC(),
		}, nil
	}

	status := "healthy"
	msg := "Teams webhook reachable"
	if latency > 5*time.Second {
		status = "degraded"
		msg = fmt.Sprintf("Teams webhook reachable but slow (%s)", latency)
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
		return fmt.Errorf("validating teams credentials: webhook URL is empty")
	}

	parsed, err := url.ParseRequestURI(c.cfg.WebhookURL)
	if err != nil {
		return fmt.Errorf("validating teams credentials: invalid webhook URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("validating teams credentials: webhook URL must use HTTPS")
	}

	// Microsoft Teams webhook URLs are hosted on office.com or webhook.office.com
	if !strings.Contains(parsed.Host, "office.com") && !strings.Contains(parsed.Host, "microsoft.com") {
		return fmt.Errorf("validating teams credentials: webhook URL host %q does not appear to be a Microsoft Teams endpoint", parsed.Host)
	}

	// Verify the webhook endpoint is reachable
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.cfg.WebhookURL, nil)
	if err != nil {
		return fmt.Errorf("validating teams credentials: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("validating teams credentials: webhook unreachable: %w", err)
	}
	defer resp.Body.Close()

	// 405 Method Not Allowed is expected for HEAD on some webhook endpoints;
	// anything below 500 confirms the endpoint exists.
	if resp.StatusCode >= 500 {
		return fmt.Errorf("validating teams credentials: webhook returned server error %d", resp.StatusCode)
	}

	return nil
}

func (c *Connector) SendNotification(ctx context.Context, notif connectorsdk.NotificationRequest) error {
	card := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": severityColor(notif.Severity),
		"summary":    notif.Title,
		"sections": []map[string]any{
			{
				"activityTitle": notif.Title,
				"text":          notif.Message,
				"facts": []map[string]string{
					{"name": "Severity", "value": notif.Severity},
					{"name": "Source", "value": "AegisClaw"},
				},
			},
		},
	}

	data, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshaling teams card: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating teams request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending teams notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("teams webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func severityColor(severity string) string {
	switch severity {
	case "critical":
		return "FF0000"
	case "high":
		return "FF6600"
	case "medium":
		return "FFCC00"
	case "low":
		return "00CC00"
	default:
		return "0078D7"
	}
}
