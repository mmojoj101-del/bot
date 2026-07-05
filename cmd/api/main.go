package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/raghna/fury-sms-gateway/internal/api/router"
	"github.com/raghna/fury-sms-gateway/internal/config"
	"github.com/raghna/fury-sms-gateway/internal/connector"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
	pgrepo "github.com/raghna/fury-sms-gateway/internal/repository/postgres"
	"github.com/raghna/fury-sms-gateway/internal/worker"
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

	// Initialize worker dependencies
	txManager := pgrepo.NewTxManager(db)
	queueRepo := pgrepo.NewQueueRepository(db, txManager)
	msgRepo := pgrepo.NewMessageRepository(db)
	outboxRepo := pgrepo.NewOutboxRepository(db)

	// Register senders
	senders := map[domain.ConnectorType]domain.Sender{
		connector.NewHTTPSender().Type(): connector.NewHTTPSender(),
	}

	// Retry policy
	retryPolicy := worker.NewDefaultRetryPolicy()

	// Create workers — all share the root ctx so main owns the context tree
	qw := worker.NewQueueWorker(
		ctx,
		queueRepo,
		msgRepo,
		nil, // connRepo — will need a connector repo here
		senders,
		retryPolicy,
		nil, // metrics — use noop for now
		eventBus,
		worker.WithBatchSize(100),
		worker.WithPollInterval(1*time.Second),
	)

	re := worker.NewRetryEngine(
		ctx,
		queueRepo,
		retryPolicy,
		worker.RetryEngineWithBatchSize(100),
		worker.RetryEngineWithPollInterval(5*time.Second),
	)

	ow := worker.NewOutboxWorker(
		ctx,
		outboxRepo,
		eventBus,
		worker.OutboxWorkerWithBatchSize(100),
		worker.OutboxWorkerWithPollInterval(500*time.Millisecond),
	)

	// Worker health check (passes to router for /ready endpoint)
	workerHealth := func() bool {
		return qw.IsHealthy() == nil && re.IsHealthy() == nil && ow.IsHealthy() == nil
	}

	// Create Fiber app
	app, err := router.New(cfg, db, rdb, eventBus, clock, workerHealth)
	if err != nil {
		slog.Error("failed to create router", "error", err)
		os.Exit(1)
	}

	// Start workers in order: outbox → retry → queue
	ow.Start()
	re.Start()
	qw.Start()

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

	// Graceful shutdown (reverse order)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.GracefulTimeout)
	defer shutdownCancel()

	slog.Info("draining workers...")

	// 1. Stop HTTP server first — no new requests
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	// 2. Stop QueueWorker first — no new messages pulled
	qw.Stop()
	slog.Info("queue worker stopped")

	// 3. Stop RetryEngine
	re.Stop()
	slog.Info("retry engine stopped")

	// 4. Stop OutboxWorker — drain pending events
	ow.Stop()
	slog.Info("outbox worker stopped")

	// 5. Wait for HTTP server to finish
	wg.Wait()

	// 6. Close event bus
	eventBus.Close()

	// 7. Close Redis
	if rdb != nil {
		if err := rdb.Close(); err != nil {
			slog.Error("redis close error", "error", err)
		}
	}

	// 8. Close PostgreSQL
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
