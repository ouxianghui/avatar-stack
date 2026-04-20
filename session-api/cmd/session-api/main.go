package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"avatar-stack/session-api/internal/config"
	"avatar-stack/session-api/internal/httpapi"
	"avatar-stack/session-api/internal/service"
	"avatar-stack/session-api/internal/store"
)

// main wires all runtime dependencies and starts the HTTP server.
//
// Startup flow:
// 1. Load config from environment.
// 2. Build logger + Redis-backed store.
// 3. Build service and HTTP router.
// 4. Start server and wait for SIGINT/SIGTERM.
// 5. Gracefully shutdown.
func main() {
	cfg, err := config.Load()
	if err != nil {
		// Config errors are unrecoverable at runtime, fail fast.
		panic(err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	redisStore, err := store.NewRedisStore(
		cfg.RedisURL,
		cfg.SessionKeyPrefix,
		cfg.SessionTTL,
	)
	if err != nil {
		logger.Error("failed to init redis store", "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := redisStore.Close(); closeErr != nil {
			logger.Error("failed to close redis store", "error", closeErr)
		}
	}()

	sessionService := service.NewSessionService(cfg, redisStore, logger)
	handler := httpapi.NewRouter(cfg, sessionService)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.RequestTimeout,
	}

	go func() {
		logger.Info("session-api started", "addr", cfg.HTTPAddr)
		if serveErr := server.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", "error", serveErr)
			os.Exit(1)
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()

	// Shutdown with timeout to avoid hanging on half-open connections.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown gracefully", "error", err)
		os.Exit(1)
	}

	logger.Info("session-api stopped")
}
