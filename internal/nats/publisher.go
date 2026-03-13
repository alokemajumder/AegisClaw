package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
)

// Publisher provides typed JetStream publishing.
type Publisher struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// NewPublisher creates a new NATS JetStream publisher.
func NewPublisher(js jetstream.JetStream, logger *slog.Logger) *Publisher {
	return &Publisher{js: js, logger: logger}
}

// Publish wraps the payload in an Envelope and publishes to the given subject.
func (p *Publisher) Publish(ctx context.Context, subject string, orgID uuid.UUID, payload any) error {
	env := NewEnvelope(orgID, subject, payload)
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshaling message: %w", err)
	}

	ack, err := p.js.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}

	p.logger.Debug("message published",
		"subject", subject,
		"trace_id", env.TraceID,
		"stream", ack.Stream,
		"seq", ack.Sequence,
	)
	return nil
}
