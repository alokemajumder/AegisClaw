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
	aegisnats "github.com/alokemajumder/AegisClaw/internal/nats"
	"github.com/alokemajumder/AegisClaw/internal/observability"
	"github.com/alokemajumder/AegisClaw/internal/sandbox"
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

	// Sandbox manager (NemoClaw/OpenShell isolation for agent execution)
	sandboxMgr := sandbox.NewManager(sandbox.Config{
		Enabled:        cfg.Sandbox.Enabled,
		RuntimeURL:     cfg.Sandbox.RuntimeURL,
		PolicyDir:      cfg.Sandbox.PolicyDir,
		TimeoutSeconds: cfg.Sandbox.TimeoutSeconds,
		MaxMemoryMB:    cfg.Sandbox.MaxMemoryMB,
		MaxCPUCores:    cfg.Sandbox.MaxCPUCores,
		NetworkPolicy:  cfg.Sandbox.NetworkPolicy,
		Image:          cfg.Sandbox.Image,
		GPU:            cfg.Sandbox.GPU,
		OllamaURL:      cfg.Ollama.URL,
		Gateway: sandbox.GatewayConfig{
			URL:      cfg.Sandbox.RuntimeURL,
			AuthMode: cfg.Sandbox.AuthMode,
			CertFile: cfg.Sandbox.CertFile,
			KeyFile:  cfg.Sandbox.KeyFile,
			CAFile:   cfg.Sandbox.CAFile,
			Token:    cfg.Sandbox.GatewayToken,
		},
	}, logger)
	if sandboxMgr.IsEnabled() {
		logger.Info("sandbox execution enabled", "runtime_url", cfg.Sandbox.RuntimeURL)
		if err := sandboxMgr.ConnectGateway(ctx); err != nil {
			logger.Warn("OpenShell gateway connection failed (in-process fallback)", "error", err)
		}
	} else {
		logger.Info("sandbox execution disabled (in-process fallback)")
	}
	_ = sandboxMgr // Will be wired to RunEngine when gRPC proto is available

	// Set up gRPC server
	grpcPort := cfg.Server.GRPCBasePort + 1 // 9091 by default (base 9090 + 1)
	grpcAddr := fmt.Sprintf(":%d", grpcPort)

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		logger.Error("failed to listen on grpc port", "addr", grpcAddr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpcutil.ServerOptions(logger)...)
	// TODO: Register runner gRPC services once proto definitions are available.

	go func() {
		logger.Info("runner grpc server starting", "addr", grpcAddr)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("grpc server error", "error", err)
			os.Exit(1)
		}
	}()

	// Health check endpoints
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"healthy","service":"runner"}`)
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !nc.Conn.IsConnected() {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"not_ready","service":"runner","error":"nats: disconnected"}`)
			return
		}
		fmt.Fprintf(w, `{"status":"ready","service":"runner"}`)
	})
	healthServer := &http.Server{
		Addr:         ":10091",
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

	healthServer.Shutdown(context.Background())
	cancel()
	logger.Info("runner stopped")
}
