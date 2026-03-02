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
	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/internal/observability"
)

const (
	serviceName = "evidence-service"
	listenPort  = 9092
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration.
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Set up structured logger.
	logger := observability.NewLogger(serviceName, cfg.Observability.LogLevel)
	slog.SetDefault(logger)

	// Initialize evidence store (MinIO).
	store, err := evidence.NewStore(ctx, cfg.MinIO, logger)
	if err != nil {
		logger.Error("failed to initialize evidence store", "error", err)
		os.Exit(1)
	}

	// Verify evidence store connectivity.
	if err := store.HealthCheck(ctx); err != nil {
		logger.Error("evidence store health check failed", "error", err)
		os.Exit(1)
	}

	// Create gRPC server.
	grpcServer := grpc.NewServer()

	// Start listening.
	addr := fmt.Sprintf(":%d", listenPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "address", addr, "error", err)
		os.Exit(1)
	}

	// Handle graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", "signal", sig.String())
		grpcServer.GracefulStop()
		cancel()
	}()

	logger.Info("starting service", "service", serviceName, "port", listenPort)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Error("gRPC server exited with error", "error", err)
		os.Exit(1)
	}

	logger.Info("service stopped", "service", serviceName)
}
