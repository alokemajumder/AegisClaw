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
	"github.com/alokemajumder/AegisClaw/internal/database"
	"github.com/alokemajumder/AegisClaw/internal/metrics"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
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

	// Handler with all dependencies
	h := api.NewHandler(pool, tokenSvc, publisher, logger)

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
	r.Handle("/metrics", metrics.Handler())

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Auth (unauthenticated) — stricter rate limit on login/refresh
		r.Route("/auth", func(r chi.Router) {
			r.With(httprate.LimitByIP(10, time.Minute)).Post("/login", h.Login)
			r.With(httprate.LimitByIP(10, time.Minute)).Post("/refresh", h.Refresh)
			r.With(tokenSvc.Middleware).Get("/me", h.Me)
			r.With(tokenSvc.Middleware).Post("/logout", h.Logout)
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(tokenSvc.Middleware)

			// Dashboard
			r.Get("/dashboard/summary", h.DashboardSummary)
			r.Get("/dashboard/activity", h.DashboardActivity)
			r.Get("/dashboard/health", h.DashboardHealth)

			// Assets
			r.Route("/assets", func(r chi.Router) {
				r.Get("/", h.ListAssets)
				r.Post("/", h.CreateAsset)
				r.Get("/{assetID}", h.GetAsset)
				r.Put("/{assetID}", h.UpdateAsset)
				r.Delete("/{assetID}", h.DeleteAsset)
				r.Get("/{assetID}/findings", h.ListAssetFindings)
			})

			// Engagements
			r.Route("/engagements", func(r chi.Router) {
				r.Get("/", h.ListEngagements)
				r.Post("/", h.CreateEngagement)
				r.Get("/{engagementID}", h.GetEngagement)
				r.Put("/{engagementID}", h.UpdateEngagement)
				r.Delete("/{engagementID}", h.DeleteEngagement)
				r.Post("/{engagementID}/activate", h.ActivateEngagement)
				r.Post("/{engagementID}/pause", h.PauseEngagement)
				r.Get("/{engagementID}/runs", h.ListEngagementRuns)
				r.Post("/{engagementID}/runs", h.TriggerRun)
			})

			// Runs
			r.Route("/runs", func(r chi.Router) {
				r.Get("/", h.ListRuns)
				r.Get("/{runID}", h.GetRun)
				r.Get("/{runID}/steps", h.ListRunSteps)
				r.Get("/{runID}/receipt", h.GetRunReceipt)
				r.Post("/{runID}/kill", h.KillRun)
				r.Post("/{runID}/pause", h.PauseRun)
				r.Post("/{runID}/resume", h.ResumeRun)
			})

			// Findings
			r.Route("/findings", func(r chi.Router) {
				r.Get("/", h.ListFindings)
				r.Get("/{findingID}", h.GetFinding)
				r.Put("/{findingID}", h.UpdateFinding)
				r.Post("/{findingID}/ticket", h.CreateFindingTicket)
				r.Post("/{findingID}/retest", h.RetestFinding)
			})

			// Connectors
			r.Route("/connectors", func(r chi.Router) {
				r.Get("/registry", h.ListConnectorRegistry)
				r.Get("/registry/{connectorType}", h.GetConnectorType)
				r.Get("/", h.ListConnectorInstances)
				r.Post("/", h.CreateConnectorInstance)
				r.Get("/{connectorID}", h.GetConnectorInstance)
				r.Put("/{connectorID}", h.UpdateConnectorInstance)
				r.Delete("/{connectorID}", h.DeleteConnectorInstance)
				r.Patch("/{connectorID}/enable", h.ToggleConnector)
				r.Post("/{connectorID}/test", h.TestConnector)
				r.Get("/{connectorID}/health", h.GetConnectorHealth)
				r.Post("/{connectorID}/health/check", h.TriggerHealthCheck)
			})

			// Approvals
			r.Route("/approvals", func(r chi.Router) {
				r.Get("/", h.ListApprovals)
				r.Get("/{approvalID}", h.GetApproval)
				r.Post("/{approvalID}/approve", h.ApproveRequest)
				r.Post("/{approvalID}/deny", h.DenyRequest)
			})

			// Reports
			r.Route("/reports", func(r chi.Router) {
				r.Get("/", h.ListReports)
				r.Post("/generate", h.GenerateReport)
				r.Get("/{reportID}", h.GetReport)
				r.Get("/{reportID}/download", h.DownloadReport)
			})

			// Coverage
			r.Get("/coverage", h.GetCoverage)
			r.Get("/coverage/gaps", h.GetCoverageGaps)

			// Admin
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

	logger.Info("api-gateway stopped")
}
