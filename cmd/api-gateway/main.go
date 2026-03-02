package main

import (
	"context"
	"encoding/json"
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

	"github.com/alokemajumder/AegisClaw/internal/auth"
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

	logger := observability.NewLogger("api-gateway", cfg.Observability.LogLevel)
	slog.SetDefault(logger)

	shutdown, err := observability.Setup(ctx, "api-gateway", cfg.Observability)
	if err != nil {
		logger.Warn("observability setup failed (continuing without tracing)", "error", err)
	} else {
		defer shutdown(ctx)
	}

	tokenSvc := auth.NewTokenService(cfg.Auth)

	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(httprate.LimitByIP(100, time.Minute))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check (unauthenticated)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "api-gateway"})
	})

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Auth (unauthenticated)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", stub("login"))
			r.Post("/refresh", stub("refresh"))
			r.Get("/me", stub("me"))
		})

		// Authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(tokenSvc.Middleware)

			// Dashboard
			r.Get("/dashboard/summary", stub("dashboard_summary"))
			r.Get("/dashboard/activity", stub("dashboard_activity"))
			r.Get("/dashboard/health", stub("dashboard_health"))

			// Assets
			r.Route("/assets", func(r chi.Router) {
				r.Get("/", stub("list_assets"))
				r.Post("/", stub("create_asset"))
				r.Post("/import", stub("import_assets"))
				r.Get("/{assetID}", stub("get_asset"))
				r.Put("/{assetID}", stub("update_asset"))
				r.Delete("/{assetID}", stub("delete_asset"))
				r.Get("/{assetID}/findings", stub("asset_findings"))
			})

			// Engagements
			r.Route("/engagements", func(r chi.Router) {
				r.Get("/", stub("list_engagements"))
				r.Post("/", stub("create_engagement"))
				r.Get("/{engagementID}", stub("get_engagement"))
				r.Put("/{engagementID}", stub("update_engagement"))
				r.Delete("/{engagementID}", stub("delete_engagement"))
				r.Post("/{engagementID}/activate", stub("activate_engagement"))
				r.Post("/{engagementID}/pause", stub("pause_engagement"))
				r.Get("/{engagementID}/runs", stub("list_engagement_runs"))
				r.Post("/{engagementID}/runs", stub("trigger_run"))
			})

			// Runs
			r.Route("/runs", func(r chi.Router) {
				r.Get("/", stub("list_runs"))
				r.Get("/{runID}", stub("get_run"))
				r.Get("/{runID}/steps", stub("list_run_steps"))
				r.Get("/{runID}/receipt", stub("get_run_receipt"))
				r.Post("/{runID}/kill", stub("kill_run"))
				r.Post("/{runID}/pause", stub("pause_run"))
				r.Post("/{runID}/resume", stub("resume_run"))
			})

			// Findings
			r.Route("/findings", func(r chi.Router) {
				r.Get("/", stub("list_findings"))
				r.Get("/clusters", stub("list_finding_clusters"))
				r.Get("/{findingID}", stub("get_finding"))
				r.Put("/{findingID}", stub("update_finding"))
				r.Post("/{findingID}/ticket", stub("create_finding_ticket"))
				r.Post("/{findingID}/retest", stub("retest_finding"))
			})

			// Connectors (settings-driven management)
			r.Route("/connectors", func(r chi.Router) {
				r.Get("/registry", stub("list_connector_registry"))
				r.Get("/registry/{connectorType}", stub("get_connector_type"))
				r.Get("/settings", stub("get_connector_settings"))
				r.Put("/settings", stub("update_connector_settings"))
				r.Get("/settings/{category}", stub("get_category_settings"))
				r.Put("/settings/{category}", stub("update_category_settings"))
				r.Get("/", stub("list_connector_instances"))
				r.Post("/", stub("create_connector_instance"))
				r.Get("/{connectorID}", stub("get_connector_instance"))
				r.Put("/{connectorID}", stub("update_connector_instance"))
				r.Delete("/{connectorID}", stub("delete_connector_instance"))
				r.Patch("/{connectorID}/enable", stub("toggle_connector"))
				r.Post("/{connectorID}/test", stub("test_connector"))
				r.Get("/{connectorID}/health", stub("get_connector_health"))
				r.Post("/{connectorID}/health/check", stub("trigger_health_check"))
			})

			// Approvals
			r.Route("/approvals", func(r chi.Router) {
				r.Get("/", stub("list_approvals"))
				r.Get("/{approvalID}", stub("get_approval"))
				r.Post("/{approvalID}/approve", stub("approve"))
				r.Post("/{approvalID}/deny", stub("deny"))
			})

			// Reports
			r.Route("/reports", func(r chi.Router) {
				r.Get("/", stub("list_reports"))
				r.Post("/generate", stub("generate_report"))
				r.Get("/{reportID}", stub("get_report"))
				r.Get("/{reportID}/download", stub("download_report"))
			})

			// Evidence
			r.Route("/evidence", func(r chi.Router) {
				r.Get("/", stub("list_evidence"))
				r.Get("/{evidenceID}", stub("get_evidence"))
				r.Get("/{evidenceID}/download", stub("download_evidence"))
			})

			// Policies
			r.Route("/policies", func(r chi.Router) {
				r.Get("/", stub("list_policies"))
				r.Post("/", stub("create_policy"))
				r.Get("/{policyID}", stub("get_policy"))
				r.Put("/{policyID}", stub("update_policy"))
			})

			// Coverage
			r.Get("/coverage", stub("get_coverage_matrix"))
			r.Get("/coverage/gaps", stub("get_coverage_gaps"))
			r.Get("/coverage/drift", stub("get_coverage_drift"))

			// Settings
			r.Get("/settings", stub("get_settings"))
			r.Put("/settings", stub("update_settings"))

			// Admin
			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireRole("admin"))
				r.Get("/users", stub("list_users"))
				r.Post("/users", stub("create_user"))
				r.Put("/users/{userID}", stub("update_user"))
				r.Get("/audit-log", stub("query_audit_log"))
				r.Get("/system/health", handleSystemHealth())
				r.Post("/system/kill-switch", stub("kill_switch"))
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

func stub(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data":    nil,
			"message": fmt.Sprintf("endpoint %q not yet implemented", name),
		})
	}
}

func handleSystemHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"status":  "healthy",
				"service": "api-gateway",
				"version": "0.1.0",
			},
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
