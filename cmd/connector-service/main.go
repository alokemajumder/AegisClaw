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

	"github.com/alokemajumder/AegisClaw/connectors/edr/defender"
	"github.com/alokemajumder/AegisClaw/internal/grpcutil"
	"github.com/alokemajumder/AegisClaw/connectors/itsm/servicenow"
	"github.com/alokemajumder/AegisClaw/connectors/notifications/slack"
	"github.com/alokemajumder/AegisClaw/connectors/notifications/teams"
	"github.com/alokemajumder/AegisClaw/connectors/siem/sentinel"
	"github.com/alokemajumder/AegisClaw/internal/config"
	"github.com/alokemajumder/AegisClaw/internal/connector"
	"github.com/alokemajumder/AegisClaw/internal/database"
	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/observability"
	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger("connector-service", cfg.Observability.LogLevel)
	slog.SetDefault(logger)

	// Database
	pool, err := database.New(ctx, cfg.Database, logger)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Register all connector factories
	registry := connectorsdk.NewRegistry()
	_ = registry.Register("sentinel", func() connectorsdk.Connector { return sentinel.New() })
	_ = registry.Register("defender", func() connectorsdk.Connector { return defender.New() })
	_ = registry.Register("servicenow", func() connectorsdk.Connector { return servicenow.New() })
	_ = registry.Register("teams", func() connectorsdk.Connector { return teams.New() })
	_ = registry.Register("slack", func() connectorsdk.Connector { return slack.New() })

	logger.Info("connector registry initialized", "types", registry.ListTypes())

	// Connector service
	connRepo := repository.NewConnectorInstanceRepo(pool)
	svc := connector.NewService(registry, connRepo, logger)
	defer svc.Close()

	// Start health check loop
	go svc.StartHealthLoop(ctx, 5*time.Minute)

	// gRPC server
	port := cfg.Server.GRPCBasePort + 3 // 9093
	addr := fmt.Sprintf(":%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "port", port, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpcutil.ServerOptions(logger)...)

	// Health check endpoints
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"healthy","service":"connector-service"}`)
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"connector-service","error":"database: %s"}`, err.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"ready","service":"connector-service"}`)
	})
	healthServer := &http.Server{
		Addr:         ":10093",
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

	logger.Info("connector-service starting", "port", port)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Error("gRPC server error", "error", err)
		os.Exit(1)
	}

	logger.Info("connector-service stopped")
}
