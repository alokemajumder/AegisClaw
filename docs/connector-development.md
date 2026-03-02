# Connector Development Guide

## Overview

AegisClaw uses a **plugin-based connector system** that allows you to integrate with any security platform. Connectors are managed through a settings-driven registry — you configure them via the UI/API without code changes to the platform.

To add support for a new platform, you implement the `Connector` interface from the Connector SDK.

## Quick Start

### 1. Create a new connector package

```
connectors/
  siem/
    myplatform/
      connector.go      # Main connector implementation
      connector_test.go  # Tests
      schema.go          # Config schema
```

### 2. Implement the Connector interface

```go
package myplatform

import (
    "context"
    "encoding/json"

    sdk "github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

type Connector struct {
    config MyPlatformConfig
    client *http.Client
}

type MyPlatformConfig struct {
    BaseURL   string `json:"base_url"`
    TenantID  string `json:"tenant_id"`
}

func New() sdk.Connector {
    return &Connector{}
}

func (c *Connector) Type() string              { return "myplatform" }
func (c *Connector) Category() sdk.Category    { return sdk.CategorySIEM }
func (c *Connector) Version() string           { return "1.0.0" }
func (c *Connector) Capabilities() []sdk.Capability {
    return []sdk.Capability{sdk.CapQueryEvents, sdk.CapDeepLink}
}

func (c *Connector) Init(ctx context.Context, cfg sdk.ConnectorConfig) error {
    var platformCfg MyPlatformConfig
    if err := json.Unmarshal(cfg.Config, &platformCfg); err != nil {
        return fmt.Errorf("parsing config: %w", err)
    }
    c.config = platformCfg
    c.client = &http.Client{Timeout: 30 * time.Second}
    return nil
}

func (c *Connector) Close() error {
    c.client.CloseIdleConnections()
    return nil
}

func (c *Connector) HealthCheck(ctx context.Context) (*sdk.HealthStatus, error) {
    start := time.Now()
    // Make a lightweight API call to verify connectivity
    resp, err := c.client.Get(c.config.BaseURL + "/api/health")
    if err != nil {
        return &sdk.HealthStatus{
            Status:    "unhealthy",
            Message:   err.Error(),
            Latency:   time.Since(start),
            CheckedAt: time.Now(),
        }, nil
    }
    defer resp.Body.Close()

    return &sdk.HealthStatus{
        Status:    "healthy",
        Latency:   time.Since(start),
        CheckedAt: time.Now(),
    }, nil
}

func (c *Connector) ValidateCredentials(ctx context.Context) error {
    // Verify credentials are valid
    return nil
}

func (c *Connector) ConfigSchema() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "base_url": {"type": "string", "description": "Platform API base URL"},
            "tenant_id": {"type": "string", "description": "Tenant identifier"}
        },
        "required": ["base_url", "tenant_id"]
    }`)
}
```

### 3. Implement capability interfaces

If your connector supports event querying:

```go
func (c *Connector) QueryEvents(ctx context.Context, query sdk.EventQuery) (*sdk.EventResult, error) {
    // Translate the generic query into platform-specific API call
    // Normalize results into sdk.Event structs
    return &sdk.EventResult{
        Events:     events,
        TotalCount: len(events),
    }, nil
}
```

### 4. Register the connector

In `cmd/connector-service/main.go`, register your factory:

```go
registry.Register("myplatform", func() sdk.Connector { return myplatform.New() })
```

### 5. Add to the connector registry database

Add an INSERT statement to the migration or seed script:

```sql
INSERT INTO connector_registry (connector_type, category, display_name, description, version, config_schema, capabilities, status)
VALUES ('myplatform', 'siem', 'My Platform', 'Description here', '1.0.0', '{"type":"object",...}', '{"query_events","deep_link"}', 'available');
```

## Connector SDK Interfaces

### Core Interface (Required)

| Method | Description |
|--------|-------------|
| `Type()` | Returns the connector type identifier |
| `Category()` | Returns the category (siem, edr, itsm, identity, notification, cloud) |
| `Version()` | Returns the connector version |
| `Capabilities()` | Returns list of supported capabilities |
| `Init(ctx, config)` | Initialize with configuration and credentials |
| `Close()` | Cleanup resources |
| `HealthCheck(ctx)` | Check platform connectivity and status |
| `ValidateCredentials(ctx)` | Verify credentials are valid |
| `ConfigSchema()` | Return JSON Schema for config validation |

### Capability Interfaces (Implement as needed)

| Interface | Capabilities | Methods |
|-----------|-------------|---------|
| `EventQuerier` | `query_events` | `QueryEvents(ctx, query)` |
| `TicketManager` | `create_ticket`, `update_ticket` | `CreateTicket(ctx, ticket)`, `UpdateTicket(ctx, id, update)` |
| `Notifier` | `send_notification` | `SendNotification(ctx, notif)` |
| `AssetFetcher` | `fetch_assets` | `FetchAssets(ctx, filter)` |
| `DeepLinker` | `deep_link` | `GenerateDeepLink(ctx, entityType, entityID)` |

## Configuration

Connectors receive their configuration via `sdk.ConnectorConfig`:

```go
type ConnectorConfig struct {
    Config        json.RawMessage       // Platform-specific config (from connector_instances.config)
    AuthMethod    string                // api_key, oauth2, service_principal, certificate
    Secrets       map[string]string     // Resolved secrets from vault
    RateLimit     RateLimitConfig       // Per-instance rate limiting
    Retry         RetryConfig           // Retry behavior
    FieldMappings map[string]string     // Custom field mapping overrides
}
```

## Best Practices

1. **Always use context**: Pass `ctx` to all external calls for timeout/cancellation
2. **Normalize data**: Map platform-specific fields to SDK normalized schemas
3. **Handle pagination**: External APIs often paginate — handle this transparently
4. **Rate limit awareness**: Respect both AegisClaw rate limits and platform API limits
5. **Error handling**: Return meaningful error messages that help debugging
6. **Health checks should be lightweight**: Don't run expensive queries for health checks
7. **Deep links**: Generate URLs that link directly to the entity in the platform's UI
8. **Test with real APIs**: Integration tests should verify actual API compatibility
