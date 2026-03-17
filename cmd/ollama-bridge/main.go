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

	// Create Ollama client (always available as fallback).
	client := ollama.NewClient(
		cfg.Ollama.URL,
		cfg.Ollama.TimeoutSeconds,
		allowedModels,
		logger,
	)

	// NVIDIA NIM — optional high-performance LLM backend.
	// When enabled, NIM is the primary backend with Ollama as fallback.
	var nimClient *ollama.NIMClient
	if cfg.NVIDIANIMM.Enabled {
		nimAPIKey := cfg.NVIDIANIMM.APIKey
		if nimAPIKey == "" && cfg.NVIDIANIMM.APIKeyRef != "" {
			nimAPIKey = os.Getenv(cfg.NVIDIANIMM.APIKeyRef)
		}

		nimModels := cfg.NVIDIANIMM.ModelAllowlist
		if len(nimModels) == 0 {
			nimModels = []string{
				"nvidia/nemotron-4-340b-instruct",
				"nvidia/llama-3.1-nemotron-70b-instruct",
				"meta/llama-3.1-405b-instruct",
				"meta/llama-3.1-70b-instruct",
				"mistralai/mixtral-8x22b-instruct-v0.1",
				"deepseek-ai/deepseek-r1",
			}
		}

		nimClient = ollama.NewNIMClient(
			cfg.NVIDIANIMM.URL,
			nimAPIKey,
			cfg.NVIDIANIMM.TimeoutSeconds,
			nimModels,
			logger,
		)

		if nimClient.IsAvailable(ctx) {
			logger.Info("NVIDIA NIM is available (primary LLM backend)",
				"url", cfg.NVIDIANIMM.URL,
				"default_model", cfg.NVIDIANIMM.DefaultModel,
			)
		} else {
			logger.Warn("NVIDIA NIM enabled but not reachable — falling back to Ollama",
				"url", cfg.NVIDIANIMM.URL,
			)
		}
	}

	// Check Ollama connectivity.
	if client.IsAvailable(ctx) {
		logger.Info("ollama service is available", "url", cfg.Ollama.URL)
	} else {
		if nimClient == nil {
			logger.Warn("ollama service is not available (agents will use deterministic fallback)",
				"url", cfg.Ollama.URL)
		} else {
			logger.Info("ollama unavailable but NIM is configured as primary backend")
		}
	}

	logger.Info("LLM configuration",
		"ollama_url", cfg.Ollama.URL,
		"ollama_default_model", cfg.Ollama.DefaultModel,
		"nvidia_nim_enabled", cfg.NVIDIANIMM.Enabled,
		"nvidia_nim_url", cfg.NVIDIANIMM.URL,
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
		ollamaReady := client.IsAvailable(context.Background())
		nimReady := nimClient != nil && nimClient.IsAvailable(context.Background())
		if ollamaReady || nimReady {
			backend := "ollama"
			if nimReady {
				backend = "nvidia_nim"
			}
			fmt.Fprintf(w, `{"status":"ready","service":"ollama-bridge","backend":"%s"}`, backend)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"ollama-bridge","error":"no LLM backend available"}`)
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
