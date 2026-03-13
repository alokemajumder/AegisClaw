package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alokemajumder/AegisClaw/internal/circuitbreaker"
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
	breakers   map[uuid.UUID]*circuitbreaker.CircuitBreaker
}

// NewService creates a new ConnectorService.
func NewService(registry *connectorsdk.Registry, repo *repository.ConnectorInstanceRepo, logger *slog.Logger) *Service {
	return &Service{
		registry:   registry,
		repo:       repo,
		logger:     logger,
		connectors: make(map[uuid.UUID]connectorsdk.Connector),
		breakers:   make(map[uuid.UUID]*circuitbreaker.CircuitBreaker),
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

	secrets := make(map[string]string)
	if instance.SecretRef != nil && *instance.SecretRef != "" {
		if val := os.Getenv(*instance.SecretRef); val != "" {
			secrets["api_key"] = val
		} else {
			s.logger.Warn("secret_ref environment variable not set", "secret_ref", *instance.SecretRef, "instance_id", instanceID)
		}
	}

	cfg := connectorsdk.ConnectorConfig{
		Config:        instance.Config,
		AuthMethod:    instance.AuthMethod,
		Secrets:       secrets,
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

// getBreaker returns or creates a circuit breaker for the given connector instance.
func (s *Service) getBreaker(instanceID uuid.UUID) *circuitbreaker.CircuitBreaker {
	s.mu.RLock()
	cb, ok := s.breakers[instanceID]
	s.mu.RUnlock()
	if ok {
		return cb
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after acquiring write lock
	if cb, ok = s.breakers[instanceID]; ok {
		return cb
	}
	cb = circuitbreaker.New(
		"connector-"+instanceID.String(),
		5,
		30*time.Second,
		s.logger,
	)
	s.breakers[instanceID] = cb
	return cb
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

	cb := s.getBreaker(instanceID)
	var result *connectorsdk.EventResult
	cbErr := cb.Execute(ctx, func(cbCtx context.Context) error {
		var qErr error
		result, qErr = querier.QueryEvents(cbCtx, query)
		return qErr
	})
	return result, cbErr
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

	cb := s.getBreaker(instanceID)
	var result *connectorsdk.TicketResult
	cbErr := cb.Execute(ctx, func(cbCtx context.Context) error {
		var tErr error
		result, tErr = mgr.CreateTicket(cbCtx, ticket)
		return tErr
	})
	return result, cbErr
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

	cb := s.getBreaker(instanceID)
	return cb.Execute(ctx, func(cbCtx context.Context) error {
		return notifier.SendNotification(cbCtx, notif)
	})
}

// ListByCategory returns connector instances for the given org and category.
func (s *Service) ListByCategory(ctx context.Context, orgID uuid.UUID, category string) ([]uuid.UUID, error) {
	instances, err := s.repo.ListByCategory(ctx, orgID, category)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for _, inst := range instances {
		if inst.Enabled {
			ids = append(ids, inst.ID)
		}
	}
	return ids, nil
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
