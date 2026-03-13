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
	"github.com/alokemajumder/AegisClaw/internal/observability"
	"github.com/alokemajumder/AegisClaw/internal/ollama"
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

	// Parse allowed models from config.
	allowedModels := cfg.Ollama.ModelAllowlist
	if len(allowedModels) == 0 {
		allowedModels = []string{"llama3.2", "mistral", "codellama", "phi3"}
	}

	// Create Ollama client.
	client := ollama.NewClient(
		cfg.Ollama.URL,
		cfg.Ollama.TimeoutSeconds,
		allowedModels,
		logger,
	)

	// Check connectivity.
	if client.IsAvailable(ctx) {
		logger.Info("ollama service is available", "url", cfg.Ollama.URL)
	} else {
		logger.Warn("ollama service is not available (agents will use deterministic fallback)",
			"url", cfg.Ollama.URL)
	}

	logger.Info("ollama configuration",
		"url", cfg.Ollama.URL,
		"default_model", cfg.Ollama.DefaultModel,
		"timeout_seconds", cfg.Ollama.TimeoutSeconds,
		"allowed_models", allowedModels,
	)

	addr := fmt.Sprintf(":%d", 9095)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "addr", addr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpcutil.ServerOptions(logger)...)

	// Health check endpoints
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"healthy","service":"ollama-bridge"}`)
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if client.IsAvailable(context.Background()) {
			fmt.Fprintf(w, `{"status":"ready","service":"ollama-bridge"}`)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"ollama-bridge","error":"ollama unavailable"}`)
		}
	})
	healthServer := &http.Server{
		Addr:         ":10095",
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
		logger.Info("ollama-bridge gRPC server starting", "addr", addr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	<-sigCh
	logger.Info("shutting down ollama-bridge")

	healthServer.Shutdown(context.Background())

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

	logger.Info("ollama-bridge stopped")
}
