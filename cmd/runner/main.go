package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/alokemajumder/AegisClaw/internal/config"
	aegisnats "github.com/alokemajumder/AegisClaw/internal/nats"
	"github.com/alokemajumder/AegisClaw/internal/observability"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Set up structured logging
	logger := observability.NewLogger("runner", cfg.Observability.LogLevel)
	slog.SetDefault(logger)

	// Set up observability (tracing, metrics)
	shutdown, err := observability.Setup(ctx, "runner", cfg.Observability)
	if err != nil {
		logger.Warn("observability setup failed (continuing without tracing)", "error", err)
	} else {
		defer shutdown(ctx)
	}

	// Connect to NATS to receive execution tasks
	nc, err := aegisnats.New(ctx, cfg.NATS, logger)
	if err != nil {
		logger.Error("failed to connect to nats", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	// Set up gRPC server
	grpcPort := cfg.Server.GRPCBasePort + 1 // 9091 by default (base 9090 + 1)
	grpcAddr := fmt.Sprintf(":%d", grpcPort)

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Error("failed to listen on grpc port", "addr", grpcAddr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	// TODO: Register runner gRPC services once proto definitions are available.

	go func() {
		logger.Info("runner grpc server starting", "addr", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("grpc server error", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("runner started",
		"grpc_addr", grpcAddr,
		"nats_url", cfg.NATS.URL,
	)

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down runner")

	// Graceful shutdown: stop accepting new RPCs and wait for in-flight ones
	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	shutdownTimeout := 10 * time.Second
	select {
	case <-stopped:
		logger.Info("grpc server stopped gracefully")
	case <-time.After(shutdownTimeout):
		logger.Warn("grpc graceful stop timed out, forcing stop")
		grpcServer.Stop()
	}

	cancel()
	logger.Info("runner stopped")
}
