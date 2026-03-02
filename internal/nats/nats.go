package nats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/alokemajumder/AegisClaw/internal/config"
)

// Stream definitions for the platform.
const (
	StreamRuns       = "RUNS"
	StreamAgents     = "AGENTS"
	StreamEvidence   = "EVIDENCE"
	StreamConnectors = "CONNECTORS"
	StreamApprovals  = "APPROVALS"
)

// Client wraps the NATS connection and JetStream context.
type Client struct {
	Conn      *nats.Conn
	JetStream jetstream.JetStream
	logger    *slog.Logger
}

// New creates a new NATS client and establishes a connection.
func New(ctx context.Context, cfg config.NATSConfig, logger *slog.Logger) (*Client, error) {
	opts := []nats.Option{
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.DisconnectErrHandler(func(conn *nats.Conn, err error) {
			if err != nil {
				logger.Warn("nats disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(conn *nats.Conn) {
			logger.Info("nats reconnected", "url", conn.ConnectedUrl())
		}),
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("creating jetstream context: %w", err)
	}

	logger.Info("nats connection established", "url", cfg.URL)

	return &Client{
		Conn:      nc,
		JetStream: js,
		logger:    logger,
	}, nil
}

// SetupStreams creates all required JetStream streams.
func (c *Client) SetupStreams(ctx context.Context) error {
	streams := []jetstream.StreamConfig{
		{
			Name:        StreamRuns,
			Subjects:    []string{"runs.>"},
			Retention:   jetstream.LimitsPolicy,
			MaxAge:      30 * 24 * time.Hour, // 30 days
			Storage:     jetstream.FileStorage,
			Replicas:    1,
			Description: "Run lifecycle events",
		},
		{
			Name:        StreamAgents,
			Subjects:    []string{"agents.>"},
			Retention:   jetstream.WorkQueuePolicy,
			MaxAge:      24 * time.Hour,
			Storage:     jetstream.FileStorage,
			Replicas:    1,
			Description: "Agent task dispatch and results",
		},
		{
			Name:        StreamEvidence,
			Subjects:    []string{"evidence.>"},
			Retention:   jetstream.LimitsPolicy,
			MaxAge:      90 * 24 * time.Hour, // 90 days
			Storage:     jetstream.FileStorage,
			Replicas:    1,
			Description: "Evidence submission and indexing events",
		},
		{
			Name:        StreamConnectors,
			Subjects:    []string{"connectors.>"},
			Retention:   jetstream.WorkQueuePolicy,
			MaxAge:      24 * time.Hour,
			Storage:     jetstream.FileStorage,
			Replicas:    1,
			Description: "Connector execution requests and responses",
		},
		{
			Name:        StreamApprovals,
			Subjects:    []string{"approvals.>"},
			Retention:   jetstream.LimitsPolicy,
			MaxAge:      7 * 24 * time.Hour, // 7 days
			Storage:     jetstream.FileStorage,
			Replicas:    1,
			Description: "Approval requests and decisions",
		},
	}

	for _, cfg := range streams {
		_, err := c.JetStream.CreateOrUpdateStream(ctx, cfg)
		if err != nil {
			return fmt.Errorf("creating stream %s: %w", cfg.Name, err)
		}
		c.logger.Info("jetstream stream ready", "stream", cfg.Name)
	}

	return nil
}

// Close gracefully closes the NATS connection.
func (c *Client) Close() {
	if c.Conn != nil {
		c.Conn.Drain()
	}
}

// HealthCheck verifies the NATS connection is alive.
func (c *Client) HealthCheck() error {
	if !c.Conn.IsConnected() {
		return fmt.Errorf("nats is not connected")
	}
	return nil
}
