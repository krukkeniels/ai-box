package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Structured JSON logging for audit trail
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := LoadConfig()
	slog.Info("starting aibox-llm-proxy",
		"addr", cfg.ListenAddr,
		"upstream", cfg.Upstream,
		"rate_limit_rpm", cfg.RateLimitRPM,
		"rate_limit_tpm", cfg.RateLimitTPM,
		"sandbox_id", cfg.SandboxID,
	)

	proxy, err := NewLLMProxy(cfg)
	if err != nil {
		slog.Error("failed to create proxy", "err", err)
		os.Exit(1)
	}

	// SIGHUP reloads credentials
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			slog.Info("received SIGHUP, reloading credentials")
			if err := proxy.credentials.Reload(); err != nil {
				slog.Error("credential reload failed", "err", err)
			} else {
				slog.Info("credentials reloaded successfully")
			}
		}
	}()

	// Start proxy in background
	go func() {
		if err := proxy.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("proxy server error", "err", err)
			os.Exit(1)
		}
	}()

	slog.Info("proxy listening", "addr", cfg.ListenAddr)

	// Graceful shutdown on SIGTERM/SIGINT
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	sig := <-quit
	slog.Info("received signal, shutting down", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := proxy.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
		os.Exit(1)
	}
	slog.Info("proxy shut down cleanly")
}
