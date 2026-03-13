package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/nats-io/nats.go/jetstream"
)

// MessageHandler processes a raw NATS message payload.
type MessageHandler func(ctx context.Context, data []byte) error

// Consumer provides durable JetStream consumption.
type Consumer struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// NewConsumer creates a new NATS JetStream consumer.
func NewConsumer(js jetstream.JetStream, logger *slog.Logger) *Consumer {
	return &Consumer{js: js, logger: logger}
}

// Subscribe creates a durable consumer and processes messages.
func (c *Consumer) Subscribe(ctx context.Context, stream, durableName, filterSubject string, handler MessageHandler) (jetstream.ConsumeContext, error) {
	cons, err := c.js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Durable:       durableName,
		FilterSubject: filterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
	})
	if err != nil {
		return nil, fmt.Errorf("creating consumer %s: %w", durableName, err)
	}

	cc, err := cons.Consume(func(msg jetstream.Msg) {
		defer func() {
			if r := recover(); r != nil {
				c.logger.Error("panic recovered in message handler",
					"subject", msg.Subject(),
					"consumer", durableName,
					"panic", r,
					"stack", string(debug.Stack()),
				)
				msg.Nak()
			}
		}()

		c.logger.Debug("message received",
			"subject", msg.Subject(),
			"consumer", durableName,
		)

		if err := handler(ctx, msg.Data()); err != nil {
			c.logger.Error("message handling failed",
				"subject", msg.Subject(),
				"error", err,
			)
			msg.Nak()
			return
		}
		msg.Ack()
	})
	if err != nil {
		return nil, fmt.Errorf("starting consumer %s: %w", durableName, err)
	}

	c.logger.Info("consumer started", "stream", stream, "consumer", durableName, "subject", filterSubject)
	return cc, nil
}

// DecodeEnvelope extracts the typed payload from a NATS message.
func DecodeEnvelope[T any](data []byte) (*Envelope[T], error) {
	var env Envelope[T]
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decoding envelope: %w", err)
	}
	return &env, nil
}
