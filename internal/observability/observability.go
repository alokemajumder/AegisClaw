package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/alokemajumder/AegisClaw/internal/config"
)

// Setup initializes the observability stack (tracing, metrics, logging).
func Setup(ctx context.Context, serviceName string, cfg config.ObservabilityConfig) (func(context.Context) error, error) {
	// Set up structured logging
	level := parseLogLevel(cfg.LogLevel)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	})
	logger := slog.New(handler).With("service", serviceName)
	slog.SetDefault(logger)

	// Set up OTEL tracing
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion("0.1.0"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating otel resource: %w", err)
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.TracingEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating otel exporter: %w", err)
	}

	// Configurable sampling rate: 0 = never, 1.0 = always (default).
	sampler := sdktrace.AlwaysSample()
	if cfg.SamplingRate > 0 && cfg.SamplingRate < 1.0 {
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SamplingRate))
	} else if cfg.SamplingRate == 0 {
		sampler = sdktrace.NeverSample()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	logger.Info("observability initialized",
		"tracing_endpoint", cfg.TracingEndpoint,
		"log_level", cfg.LogLevel,
	)

	return tp.Shutdown, nil
}

// NewLogger creates a new structured logger for a service.
func NewLogger(serviceName string, level string) *slog.Logger {
	lvl := parseLogLevel(level)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     lvl,
		AddSource: true,
	})
	return slog.New(handler).With("service", serviceName)
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
