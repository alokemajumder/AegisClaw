package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

// Service manages connector lifecycle: loading, initializing, caching, and health checking.
type Service struct {
	registry   *connectorsdk.Registry
	repo       *repository.ConnectorInstanceRepo
	logger     *slog.Logger
	mu         sync.RWMutex
	connectors map[uuid.UUID]connectorsdk.Connector
}

// NewService creates a new ConnectorService.
func NewService(registry *connectorsdk.Registry, repo *repository.ConnectorInstanceRepo, logger *slog.Logger) *Service {
	return &Service{
		registry:   registry,
		repo:       repo,
		logger:     logger,
		connectors: make(map[uuid.UUID]connectorsdk.Connector),
	}
}

// GetConnector returns a cached or freshly initialized connector for the given instance ID.
func (s *Service) GetConnector(ctx context.Context, instanceID uuid.UUID) (connectorsdk.Connector, error) {
	s.mu.RLock()
	if conn, ok := s.connectors[instanceID]; ok {
		s.mu.RUnlock()
		return conn, nil
	}
	s.mu.RUnlock()

	return s.initConnector(ctx, instanceID)
}

func (s *Service) initConnector(ctx context.Context, instanceID uuid.UUID) (connectorsdk.Connector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock
	if conn, ok := s.connectors[instanceID]; ok {
		return conn, nil
	}

	instance, err := s.repo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("loading connector instance: %w", err)
	}

	conn, err := s.registry.Create(instance.ConnectorType)
	if err != nil {
		return nil, fmt.Errorf("creating connector %s: %w", instance.ConnectorType, err)
	}

	// Build connector config from instance
	var rateCfg connectorsdk.RateLimitConfig
	_ = json.Unmarshal(instance.RateLimitConfig, &rateCfg)
	var retryCfg connectorsdk.RetryConfig
	_ = json.Unmarshal(instance.RetryConfig, &retryCfg)
	var fieldMappings map[string]string
	_ = json.Unmarshal(instance.FieldMappings, &fieldMappings)

	cfg := connectorsdk.ConnectorConfig{
		Config:        instance.Config,
		AuthMethod:    instance.AuthMethod,
		Secrets:       make(map[string]string), // resolved from secret_ref in production
		RateLimit:     rateCfg,
		Retry:         retryCfg,
		FieldMappings: fieldMappings,
	}

	if err := conn.Init(ctx, cfg); err != nil {
		return nil, fmt.Errorf("initializing connector %s: %w", instance.Name, err)
	}

	s.connectors[instanceID] = conn
	s.logger.Info("connector initialized", "id", instanceID, "type", instance.ConnectorType, "name", instance.Name)
	return conn, nil
}

// QueryEvents finds a connector by instance ID and queries events.
func (s *Service) QueryEvents(ctx context.Context, instanceID uuid.UUID, query connectorsdk.EventQuery) (*connectorsdk.EventResult, error) {
	conn, err := s.GetConnector(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	querier, ok := conn.(connectorsdk.EventQuerier)
	if !ok {
		return nil, fmt.Errorf("connector does not support event querying")
	}
	return querier.QueryEvents(ctx, query)
}

// CreateTicket finds an ITSM connector and creates a ticket.
func (s *Service) CreateTicket(ctx context.Context, instanceID uuid.UUID, ticket connectorsdk.TicketRequest) (*connectorsdk.TicketResult, error) {
	conn, err := s.GetConnector(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	mgr, ok := conn.(connectorsdk.TicketManager)
	if !ok {
		return nil, fmt.Errorf("connector does not support ticket management")
	}
	return mgr.CreateTicket(ctx, ticket)
}

// SendNotification sends a notification via the specified connector.
func (s *Service) SendNotification(ctx context.Context, instanceID uuid.UUID, notif connectorsdk.NotificationRequest) error {
	conn, err := s.GetConnector(ctx, instanceID)
	if err != nil {
		return err
	}

	notifier, ok := conn.(connectorsdk.Notifier)
	if !ok {
		return fmt.Errorf("connector does not support notifications")
	}
	return notifier.SendNotification(ctx, notif)
}

// HealthCheckAll runs health checks on all loaded connectors.
func (s *Service) HealthCheckAll(ctx context.Context) {
	s.mu.RLock()
	ids := make([]uuid.UUID, 0, len(s.connectors))
	for id := range s.connectors {
		ids = append(ids, id)
	}
	s.mu.RUnlock()

	for _, id := range ids {
		s.mu.RLock()
		conn, ok := s.connectors[id]
		s.mu.RUnlock()
		if !ok {
			continue
		}

		status, err := conn.HealthCheck(ctx)
		healthStr := "unknown"
		if err != nil {
			healthStr = "unhealthy"
			s.logger.Warn("connector health check failed", "id", id, "error", err)
		} else if status != nil {
			healthStr = status.Status
		}

		if err := s.repo.UpdateHealthStatus(ctx, id, healthStr); err != nil {
			s.logger.Error("updating connector health status", "id", id, "error", err)
		}
	}
}

// StartHealthLoop runs periodic health checks.
func (s *Service) StartHealthLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.HealthCheckAll(ctx)
		}
	}
}

// Close shuts down all loaded connectors.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, conn := range s.connectors {
		if err := conn.Close(); err != nil {
			s.logger.Error("closing connector", "id", id, "error", err)
		}
	}
	s.connectors = make(map[uuid.UUID]connectorsdk.Connector)
}
