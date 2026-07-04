package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/raghna/fury-sms-gateway/internal/api/router"
	"github.com/raghna/fury-sms-gateway/internal/config"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
	"github.com/raghna/fury-sms-gateway/pkg/cache"
	"github.com/raghna/fury-sms-gateway/pkg/database"
)

var (
	// Build information set by ldflags
	version = "0.1.0"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Configure structured logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Log.Level),
	})))

	slog.Info("starting fury-sms-gateway",
		"version", version,
		"commit", commit,
		"build_date", date,
	)

	// Create context that will be cancelled on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database
	dsn := cfg.Database.URL
	if cfg.Database.Password != "" {
		dsn = cfg.Database.URL
	}
	db, err := database.NewPostgresPool(ctx, dsn, cfg.Database.MaxOpenConns)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize Redis
	redisAddr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	rdb, err := cache.NewRedisClient(ctx, redisAddr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		slog.Warn("redis connection failed, running without cache", "error", err)
		rdb = nil
	}

	// Initialize event bus
	eventBus := event.NewMemoryBus()
	defer eventBus.Close()

	// Clock
	clock := domain.RealClock{}

	// Create Fiber app
	app, err := router.New(cfg, db, rdb, eventBus, clock, func() bool {
		// Migrations check - for now assume migrations are applied
		// In production, this should check the migration version
		return true
	})
	if err != nil {
		slog.Error("failed to create router", "error", err)
		os.Exit(1)
	}

	// Start server in a goroutine
	addr := fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	go func() {
		slog.Info("HTTP server listening", "addr", addr)
		if err := app.Listen(addr); err != nil {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		slog.Info("shutting down gracefully...")
	case <-ctx.Done():
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.GracefulTimeout)
	defer shutdownCancel()

	// 1. Stop HTTP server
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}

	// 2. Drain requests (already handled by ShutdownWithContext)

	// 3. Close event bus
	eventBus.Close()

	// 4. Close Redis
	if rdb != nil {
		if err := rdb.Close(); err != nil {
			slog.Error("redis close error", "error", err)
		}
	}

	// 5. Close PostgreSQL
	db.Close()

	slog.Info("server stopped gracefully")
}

func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
