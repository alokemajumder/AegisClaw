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
	"github.com/alokemajumder/AegisClaw/internal/connector"
	"github.com/alokemajumder/AegisClaw/internal/database"
	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/internal/grpcutil"
	aegisnats "github.com/alokemajumder/AegisClaw/internal/nats"
	"github.com/alokemajumder/AegisClaw/internal/observability"
	"github.com/alokemajumder/AegisClaw/internal/orchestrator"
	"github.com/alokemajumder/AegisClaw/internal/playbook"
	"github.com/alokemajumder/AegisClaw/pkg/agentsdk"
	"github.com/alokemajumder/AegisClaw/pkg/connectorsdk"

	// Connector implementations
	"github.com/alokemajumder/AegisClaw/connectors/edr/defender"
	"github.com/alokemajumder/AegisClaw/connectors/itsm/servicenow"
	"github.com/alokemajumder/AegisClaw/connectors/notifications/slack"
	"github.com/alokemajumder/AegisClaw/connectors/notifications/teams"
	"github.com/alokemajumder/AegisClaw/connectors/siem/sentinel"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger("orchestrator", cfg.Observability.LogLevel)
	slog.SetDefault(logger)

	shutdown, err := observability.Setup(ctx, "orchestrator", cfg.Observability)
	if err != nil {
		logger.Warn("observability setup failed (continuing without tracing)", "error", err)
	} else {
		defer shutdown(ctx)
	}

	// Database
	pool, err := database.New(ctx, cfg.Database, logger)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// NATS
	nc, err := aegisnats.New(ctx, cfg.NATS, logger)
	if err != nil {
		logger.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()

	if err := nc.SetupStreams(ctx); err != nil {
		logger.Error("failed to set up JetStream streams", "error", err)
		os.Exit(1)
	}

	// Repositories
	runRepo := repository.NewRunRepo(pool)
	stepRepo := repository.NewRunStepRepo(pool)
	findingRepo := repository.NewFindingRepo(pool)
	engRepo := repository.NewEngagementRepo(pool)
	connInstanceRepo := repository.NewConnectorInstanceRepo(pool)
	coverageRepo := repository.NewCoverageRepo(pool)

	// Evidence store (optional — continues without if MinIO unavailable)
	var evidenceStore *evidence.Store
	evStore, err := evidence.NewStore(ctx, cfg.MinIO, logger)
	if err != nil {
		logger.Warn("evidence store unavailable (continuing without)", "error", err)
	} else {
		evidenceStore = evStore
	}

	// Connector registry + service
	connRegistry := connectorsdk.NewRegistry()
	_ = connRegistry.Register("sentinel", func() connectorsdk.Connector { return sentinel.New() })
	_ = connRegistry.Register("defender", func() connectorsdk.Connector { return defender.New() })
	_ = connRegistry.Register("servicenow", func() connectorsdk.Connector { return servicenow.New() })
	_ = connRegistry.Register("teams", func() connectorsdk.Connector { return teams.New() })
	_ = connRegistry.Register("slack", func() connectorsdk.Connector { return slack.New() })
	connectorSvc := connector.NewService(connRegistry, connInstanceRepo, logger)
	defer connectorSvc.Close()

	// Playbook loader and executor
	pbLoader := playbook.NewLoader(logger)
	pbExecutor := playbook.NewExecutor(connectorSvc, logger)

	// Kill switch
	killSwitch := orchestrator.NewKillSwitch()

	// Agent dependencies
	agentDeps := agentsdk.AgentDeps{
		Logger:                logger,
		DB:                    pool,
		EvidenceStore:         evidenceStore,
		ConnectorSvc:          connectorSvc,
		PlaybookLoader:        pbLoader,
		PlaybookExecutor:      pbExecutor,
		ReceiptHMACKey:        []byte(cfg.Auth.ReceiptHMACKey),
		ConnectorInstanceRepo: connInstanceRepo,
		PlaybookDir:           cfg.Server.PlaybookDir,
	}

	// Agent registry (initializes all agents with deps)
	agentReg := orchestrator.NewAgentRegistry(logger, agentDeps)

	// Run engine
	engine := orchestrator.NewRunEngine(agentReg, runRepo, stepRepo, findingRepo, engRepo, connectorSvc, coverageRepo, killSwitch, logger)

	// Consumer
	consumer := aegisnats.NewConsumer(nc.JetStream, logger)

	// Orchestrator
	orch := orchestrator.NewOrchestrator(engine, consumer, runRepo, engRepo, killSwitch, logger)
	if err := orch.Start(ctx); err != nil {
		logger.Error("failed to start orchestrator", "error", err)
		os.Exit(1)
	}

	// Start connector health loop in background
	go connectorSvc.StartHealthLoop(ctx, 5*time.Minute)

	// gRPC server
	grpcPort := cfg.Server.GRPCBasePort
	grpcAddr := fmt.Sprintf(":%d", grpcPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Error("failed to listen", "addr", grpcAddr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpcutil.ServerOptions(logger)...)

	go func() {
		logger.Info("orchestrator gRPC starting", "addr", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	// Health check endpoints
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"healthy","service":"orchestrator"}`)
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ctx := r.Context()
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"orchestrator","error":"database: %s"}`, err.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"ready","service":"orchestrator"}`)
	})
	healthServer := &http.Server{
		Addr:         ":10090",
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

	logger.Info("orchestrator started", "grpc_addr", grpcAddr, "nats_url", cfg.NATS.URL)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down orchestrator")

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		logger.Info("gRPC server stopped gracefully")
	case <-time.After(10 * time.Second):
		logger.Warn("gRPC graceful stop timed out, forcing stop")
		grpcServer.Stop()
	}

	healthServer.Shutdown(context.Background())
	cancel()
	logger.Info("orchestrator stopped")
}
