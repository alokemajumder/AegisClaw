package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/alokemajumder/AegisClaw/internal/config"
	"github.com/alokemajumder/AegisClaw/internal/observability"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger("ollama-bridge", cfg.Observability.LogLevel)
	slog.SetDefault(logger)

	shutdown, err := observability.Setup(ctx, "ollama-bridge", cfg.Observability)
	if err != nil {
		logger.Warn("observability setup failed (continuing without tracing)", "error", err)
	} else {
		defer shutdown(ctx)
	}

	logger.Info("ollama configuration",
		"url", cfg.Ollama.URL,
		"default_model", cfg.Ollama.DefaultModel,
		"timeout_seconds", cfg.Ollama.TimeoutSeconds,
	)

	addr := fmt.Sprintf(":%d", 9095)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "addr", addr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()

	// TODO: Register ollama-bridge gRPC service implementation here.

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("ollama-bridge gRPC server starting", "addr", addr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	<-sigCh
	logger.Info("shutting down ollama-bridge")

	grpcServer.GracefulStop()

	logger.Info("ollama-bridge stopped")
}
