package connectorsdk

import (
	"context"
	"encoding/json"
	"time"
)

// Category defines the connector category.
type Category string

const (
	CategorySIEM         Category = "siem"
	CategoryEDR          Category = "edr"
	CategoryITSM         Category = "itsm"
	CategoryIdentity     Category = "identity"
	CategoryNotification Category = "notification"
	CategoryCloud        Category = "cloud"
)

// Capability defines what a connector can do.
type Capability string

const (
	CapQueryEvents      Capability = "query_events"
	CapCreateTicket     Capability = "create_ticket"
	CapUpdateTicket     Capability = "update_ticket"
	CapSendNotification Capability = "send_notification"
	CapFetchAssets      Capability = "fetch_assets"
	CapDeepLink         Capability = "deep_link"
)

// Connector is the interface all connectors must implement.
type Connector interface {
	// Metadata
	Type() string
	Category() Category
	Capabilities() []Capability
	Version() string

	// Lifecycle
	Init(ctx context.Context, cfg ConnectorConfig) error
	Close() error

	// Health
	HealthCheck(ctx context.Context) (*HealthStatus, error)
	ValidateCredentials(ctx context.Context) error

	// Schema
	ConfigSchema() json.RawMessage
}

// EventQuerier is implemented by connectors that can query events (SIEM, EDR, Cloud).
type EventQuerier interface {
	QueryEvents(ctx context.Context, query EventQuery) (*EventResult, error)
}

// TicketManager is implemented by connectors that manage tickets (ITSM).
type TicketManager interface {
	CreateTicket(ctx context.Context, ticket TicketRequest) (*TicketResult, error)
	UpdateTicket(ctx context.Context, ticketID string, update TicketUpdate) error
}

// Notifier is implemented by connectors that send notifications.
type Notifier interface {
	SendNotification(ctx context.Context, notif NotificationRequest) error
}

// AssetFetcher is implemented by connectors that can fetch/sync assets.
type AssetFetcher interface {
	FetchAssets(ctx context.Context, filter AssetFilter) (*AssetResult, error)
}

// DeepLinker is implemented by connectors that can generate deep links.
type DeepLinker interface {
	GenerateDeepLink(ctx context.Context, entityType, entityID string) (string, error)
}

// ConnectorConfig holds the configuration for a connector instance.
type ConnectorConfig struct {
	Config     json.RawMessage       `json:"config"`
	AuthMethod string                `json:"auth_method"`
	Secrets    map[string]string     `json:"secrets"`
	RateLimit  RateLimitConfig       `json:"rate_limit"`
	Retry      RetryConfig           `json:"retry"`
	FieldMappings map[string]string  `json:"field_mappings"`
}

// RateLimitConfig for connector rate limiting.
type RateLimitConfig struct {
	RequestsPerSecond int `json:"requests_per_second"`
	Burst             int `json:"burst"`
}

// RetryConfig for connector retry behavior.
type RetryConfig struct {
	MaxRetries int `json:"max_retries"`
	BackoffMs  int `json:"backoff_ms"`
}

// HealthStatus represents the health of a connector.
type HealthStatus struct {
	Status    string    `json:"status"` // healthy, degraded, unhealthy
	Message   string    `json:"message,omitempty"`
	Latency   time.Duration `json:"latency"`
	CheckedAt time.Time `json:"checked_at"`
}

// EventQuery for querying events from SIEM/EDR/Cloud.
type EventQuery struct {
	TimeRange TimeRange         `json:"time_range"`
	Filters   map[string]string `json:"filters,omitempty"`
	Query     string            `json:"query,omitempty"` // Native query language (KQL, SPL, etc.)
	MaxResults int              `json:"max_results,omitempty"`
}

// TimeRange for time-based queries.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// EventResult from a query.
type EventResult struct {
	Events     []Event `json:"events"`
	TotalCount int     `json:"total_count"`
	Truncated  bool    `json:"truncated"`
}

// Event is a normalized security event.
type Event struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	Source      string            `json:"source"`
	Severity    string            `json:"severity,omitempty"`
	Category    string            `json:"category,omitempty"`
	Description string            `json:"description,omitempty"`
	RawData     json.RawMessage   `json:"raw_data,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// TicketRequest for creating ITSM tickets.
type TicketRequest struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Priority    string            `json:"priority"`
	Assignee    string            `json:"assignee,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
}

// TicketResult from ticket creation.
type TicketResult struct {
	TicketID  string `json:"ticket_id"`
	TicketURL string `json:"ticket_url,omitempty"`
	Status    string `json:"status"`
}

// TicketUpdate for updating an existing ticket.
type TicketUpdate struct {
	Status      *string           `json:"status,omitempty"`
	Comment     *string           `json:"comment,omitempty"`
	Priority    *string           `json:"priority,omitempty"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
}

// NotificationRequest for sending notifications.
type NotificationRequest struct {
	Title    string            `json:"title"`
	Message  string            `json:"message"`
	Severity string            `json:"severity,omitempty"`
	Channel  string            `json:"channel,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AssetFilter for querying assets from connectors.
type AssetFilter struct {
	Types     []string          `json:"types,omitempty"`
	MaxResults int              `json:"max_results,omitempty"`
	Filters   map[string]string `json:"filters,omitempty"`
}

// AssetResult from an asset query.
type AssetResult struct {
	Assets     []AssetRecord `json:"assets"`
	TotalCount int           `json:"total_count"`
}

// AssetRecord is a normalized asset record from a connector.
type AssetRecord struct {
	ExternalID  string            `json:"external_id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
