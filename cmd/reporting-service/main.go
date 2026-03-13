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
	"github.com/alokemajumder/AegisClaw/internal/database"
	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/internal/grpcutil"
	"github.com/alokemajumder/AegisClaw/internal/observability"
	"github.com/alokemajumder/AegisClaw/internal/reporting"
)

const (
	serviceName = "reporting-service"
	listenPort  = 9094
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

	// Connect to database.
	pool, err := database.New(ctx, cfg.Database, logger)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Initialize repositories.
	findings := repository.NewFindingRepo(pool)
	runs := repository.NewRunRepo(pool)
	coverage := repository.NewCoverageRepo(pool)
	assets := repository.NewAssetRepo(pool)
	reports := repository.NewReportRepo(pool)

	// Initialize evidence store (optional).
	var store *evidence.Store
	if cfg.MinIO.Endpoint != "" {
		s, err := evidence.NewStore(ctx, cfg.MinIO, logger)
		if err != nil {
			logger.Warn("evidence store unavailable (continuing without storage)", "error", err)
		} else {
			store = s
		}
	}

	// Create reporting service.
	reportSvc := reporting.NewService(findings, runs, coverage, assets, reports, store, logger)
	_ = reportSvc // Available for gRPC handlers when registered

	logger.Info("reporting service initialized",
		"db", "connected",
		"evidence_store", store != nil,
	)

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
		fmt.Fprintf(w, `{"status":"healthy","service":"reporting-service"}`)
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"reporting-service","error":"database: %s"}`, err.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"ready","service":"reporting-service"}`)
	})
	healthServer := &http.Server{
		Addr:         ":10094",
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
		grpcServer.GracefulStop()
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
