package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/alokemajumder/AegisClaw/internal/api"
	"github.com/alokemajumder/AegisClaw/internal/auth"
	"github.com/alokemajumder/AegisClaw/internal/config"
	"github.com/alokemajumder/AegisClaw/internal/connector"
	"github.com/alokemajumder/AegisClaw/internal/database"
	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/internal/metrics"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
	"github.com/alokemajumder/AegisClaw/internal/observability"
	"github.com/alokemajumder/AegisClaw/internal/reporting"
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

	logger := observability.NewLogger("api-gateway", cfg.Observability.LogLevel)
	slog.SetDefault(logger)

	shutdown, err := observability.Setup(ctx, "api-gateway", cfg.Observability)
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

	// NATS (optional — API gateway works without it but can't publish events)
	var publisher *natspkg.Publisher
	natsClient, err := natspkg.New(ctx, cfg.NATS, logger)
	if err != nil {
		logger.Warn("NATS connection failed (running without async messaging)", "error", err)
	} else {
		defer natsClient.Close()
		publisher = natspkg.NewPublisher(natsClient.JetStream, logger)
	}

	// Auth — validate JWT secret
	if cfg.Auth.JWTSecret == "" {
		logger.Error("AEGISCLAW_AUTH_JWT_SECRET is not set — refusing to start with empty JWT secret")
		os.Exit(1)
	}
	if cfg.Auth.JWTSecret == "dev-secret-change-in-production" {
		if cfg.Server.Environment == "production" {
			logger.Error("refusing to start: default development JWT secret is not allowed in production — set AEGISCLAW_AUTH_JWT_SECRET")
			os.Exit(1)
		}
		logger.Warn("WARNING: Using default development JWT secret. Set AEGISCLAW_AUTH_JWT_SECRET for production!")
	}
	tokenSvc := auth.NewTokenService(ctx, cfg.Auth)

	// Persistent token blacklist
	tokenBlacklistRepo := repository.NewTokenBlacklistRepo(pool)
	tokenSvc.SetBlacklistStore(tokenBlacklistRepo)

	// Handler with all dependencies
	h := api.NewHandler(pool, tokenSvc, publisher, logger)

	// Persistent login lockout
	loginAttemptRepo := repository.NewLoginAttemptRepo(pool)
	h.LockoutStore = loginAttemptRepo

	// Store NATS client on handler for health checks
	if natsClient != nil {
		h.NATSClient = natsClient
	}

	// Connector registry + service
	connRegistry := connectorsdk.NewRegistry()
	_ = connRegistry.Register("sentinel", func() connectorsdk.Connector { return sentinel.New() })
	_ = connRegistry.Register("defender", func() connectorsdk.Connector { return defender.New() })
	_ = connRegistry.Register("servicenow", func() connectorsdk.Connector { return servicenow.New() })
	_ = connRegistry.Register("teams", func() connectorsdk.Connector { return teams.New() })
	_ = connRegistry.Register("slack", func() connectorsdk.Connector { return slack.New() })
	connInstanceRepo := repository.NewConnectorInstanceRepo(pool)
	connectorSvc := connector.NewService(connRegistry, connInstanceRepo, logger)
	defer connectorSvc.Close()
	h.ConnectorSvc = connectorSvc

	// Evidence store (optional — log warning if unavailable)
	evStore, err := evidence.NewStore(ctx, cfg.MinIO, logger)
	if err != nil {
		logger.Warn("evidence store unavailable (continuing without)", "error", err)
	} else {
		h.EvidenceStore = evStore
	}

	// Reporting service (requires evidence store for storage, but works without)
	h.ReportSvc = reporting.NewService(
		h.Findings, h.Runs, h.Coverage, h.Assets, h.Reports,
		evStore, // may be nil — service handles it
		logger,
	)

	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(httprate.LimitByIP(100, time.Minute))
	// Prometheus metrics
	r.Use(metrics.Middleware)
	// Security response headers
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			next.ServeHTTP(w, r)
		})
	})
	corsOrigins := cfg.Server.CORSOrigins
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"http://localhost:3000"}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check (unauthenticated)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","service":"api-gateway"}`)
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"api-gateway","error":"database: %s"}`, err.Error())
			return
		}
		fmt.Fprintf(w, `{"status":"ready","service":"api-gateway"}`)
	})
	// /metrics served on internal metrics port (not public API) — see below

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Auth (unauthenticated) — stricter rate limit on login/refresh
		r.Route("/auth", func(r chi.Router) {
			r.With(httprate.LimitByIP(10, time.Minute)).Post("/login", h.Login)
			r.With(httprate.LimitByIP(10, time.Minute)).Post("/refresh", h.Refresh)
			r.With(tokenSvc.Middleware).Get("/me", h.Me)
			r.With(tokenSvc.Middleware).Post("/logout", h.Logout)
		})

		// Role middleware helpers
		requireOperator := auth.RequireRole("admin", "operator")
		requireApprover := auth.RequireRole("admin", "operator", "approver")

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(tokenSvc.Middleware)

			// Dashboard (all authenticated users)
			r.Get("/dashboard/summary", h.DashboardSummary)
			r.Get("/dashboard/activity", h.DashboardActivity)
			r.Get("/dashboard/health", h.DashboardHealth)

			// Assets
			r.Route("/assets", func(r chi.Router) {
				r.Get("/", h.ListAssets)
				r.Get("/{assetID}", h.GetAsset)
				r.Get("/{assetID}/findings", h.ListAssetFindings)

				r.With(requireOperator).Post("/", h.CreateAsset)
				r.With(requireOperator).Put("/{assetID}", h.UpdateAsset)
				r.With(requireOperator).Delete("/{assetID}", h.DeleteAsset)
			})

			// Engagements
			r.Route("/engagements", func(r chi.Router) {
				r.Get("/", h.ListEngagements)
				r.Get("/{engagementID}", h.GetEngagement)
				r.Get("/{engagementID}/runs", h.ListEngagementRuns)

				r.With(requireOperator).Post("/", h.CreateEngagement)
				r.With(requireOperator).Put("/{engagementID}", h.UpdateEngagement)
				r.With(requireOperator).Delete("/{engagementID}", h.DeleteEngagement)
				r.With(requireOperator).Post("/{engagementID}/activate", h.ActivateEngagement)
				r.With(requireOperator).Post("/{engagementID}/pause", h.PauseEngagement)
				r.With(requireOperator).Post("/{engagementID}/runs", h.TriggerRun)
			})

			// Runs
			r.Route("/runs", func(r chi.Router) {
				r.Get("/", h.ListRuns)
				r.Get("/{runID}", h.GetRun)
				r.Get("/{runID}/steps", h.ListRunSteps)
				r.Get("/{runID}/receipt", h.GetRunReceipt)

				r.With(requireOperator).Post("/{runID}/kill", h.KillRun)
				r.With(requireOperator).Post("/{runID}/pause", h.PauseRun)
				r.With(requireOperator).Post("/{runID}/resume", h.ResumeRun)
			})

			// Findings
			r.Route("/findings", func(r chi.Router) {
				r.Get("/", h.ListFindings)
				r.Get("/{findingID}", h.GetFinding)

				r.With(requireOperator).Put("/{findingID}", h.UpdateFinding)
				r.With(requireOperator).Post("/{findingID}/ticket", h.CreateFindingTicket)
				r.With(requireOperator).Post("/{findingID}/retest", h.RetestFinding)
			})

			// Connectors
			r.Route("/connectors", func(r chi.Router) {
				r.Get("/registry", h.ListConnectorRegistry)
				r.Get("/registry/{connectorType}", h.GetConnectorType)
				r.Get("/", h.ListConnectorInstances)
				r.Get("/{connectorID}", h.GetConnectorInstance)
				r.Get("/{connectorID}/health", h.GetConnectorHealth)

				r.With(requireOperator).Post("/", h.CreateConnectorInstance)
				r.With(requireOperator).Put("/{connectorID}", h.UpdateConnectorInstance)
				r.With(requireOperator).Delete("/{connectorID}", h.DeleteConnectorInstance)
				r.With(requireOperator).Patch("/{connectorID}/enable", h.ToggleConnector)
				r.With(requireOperator).Post("/{connectorID}/test", h.TestConnector)
				r.With(requireOperator).Post("/{connectorID}/health/check", h.TriggerHealthCheck)
			})

			// Approvals
			r.Route("/approvals", func(r chi.Router) {
				r.Get("/", h.ListApprovals)
				r.Get("/{approvalID}", h.GetApproval)

				r.With(requireApprover).Post("/{approvalID}/approve", h.ApproveRequest)
				r.With(requireApprover).Post("/{approvalID}/deny", h.DenyRequest)
			})

			// Reports
			r.Route("/reports", func(r chi.Router) {
				r.Get("/", h.ListReports)
				r.Get("/{reportID}", h.GetReport)
				r.Get("/{reportID}/download", h.DownloadReport)

				r.With(requireOperator).Post("/generate", h.GenerateReport)
			})

			// Coverage (all authenticated users)
			r.Get("/coverage", h.GetCoverage)
			r.Get("/coverage/gaps", h.GetCoverageGaps)

			// Admin (admin-only)
			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireRole("admin"))
				r.Get("/users", h.ListUsers)
				r.Post("/users", h.CreateUser)
				r.Put("/users/{userID}", h.UpdateUser)
				r.Get("/audit-log", h.QueryAuditLog)
				r.Get("/system/health", h.SystemHealth)
				r.Post("/system/kill-switch", h.KillSwitch)
			})
		})
	})

	// Internal metrics server (unauthenticated but not on the public API port)
	metricsPort := cfg.Observability.MetricsPort
	if metricsPort == 0 {
		metricsPort = 9102
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metrics.Handler())
	metricsSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", metricsPort),
		Handler:      metricsMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	go func() {
		logger.Info("metrics server starting", "addr", metricsSrv.Addr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
		}
	}()

	addr := fmt.Sprintf(":%d", cfg.Server.APIPort)
	srv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("api-gateway starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-sigCh
	logger.Info("shutting down api-gateway")

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", "error", err)
	}

	logger.Info("api-gateway stopped")
}
