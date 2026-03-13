package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/alokemajumder/AegisClaw/internal/config"
	"github.com/alokemajumder/AegisClaw/internal/grpcutil"
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
	grpcServer := grpc.NewServer(grpcutil.ServerOptions(logger)...)

	// Start listening.
	addr := fmt.Sprintf(":%d", listenPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "address", addr, "error", err)
		os.Exit(1)
	}

	// Health check endpoints
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"healthy","service":"evidence-service"}`)
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := store.HealthCheck(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"evidence-service","error":"minio: %s"}`, err.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"ready","service":"evidence-service"}`)
	})
	healthServer := &http.Server{
		Addr:         ":10092",
		Handler:      healthMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("health endpoint starting", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server error", "error", err)
		}
	}()

	// Handle graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", "signal", sig.String())

		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			logger.Warn("gRPC graceful stop timed out, forcing stop")
			grpcServer.Stop()
		}

		healthServer.Shutdown(context.Background())
		cancel()
	}()

	logger.Info("starting service", "service", serviceName, "port", listenPort)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Error("gRPC server exited with error", "error", err)
		os.Exit(1)
	}

	logger.Info("service stopped", "service", serviceName)
}
