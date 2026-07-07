package main

import (
	"context"
	"encoding/json"
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
	httph "github.com/raghna/fury-sms-gateway/internal/connector/driver/http"
	"github.com/raghna/fury-sms-gateway/internal/connector/rule"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	"github.com/raghna/fury-sms-gateway/internal/event"
	pgrepo "github.com/raghna/fury-sms-gateway/internal/repository/postgres"
	"github.com/raghna/fury-sms-gateway/internal/template"
	"github.com/raghna/fury-sms-gateway/internal/worker"
	"github.com/raghna/fury-sms-gateway/pkg/cache"
	"github.com/raghna/fury-sms-gateway/pkg/database"
)

var (
	version = "0.1.0"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.Log.Level),
	})))

	slog.Info("starting fury-sms-gateway",
		"version", version,
		"commit", commit,
		"build_date", date,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Database ────────────────────────────────────────────────────
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

	// ── Redis ────────────────────────────────────────────────────────
	redisAddr := fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	rdb, err := cache.NewRedisClient(ctx, redisAddr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		slog.Warn("redis connection failed, running without cache", "error", err)
		rdb = nil
	}

	// ── Event Bus ────────────────────────────────────────────────────
	eventBus := event.NewMemoryBus()
	defer eventBus.Close()

	// ── Clock ────────────────────────────────────────────────────────
	clock := domain.RealClock{}

	// ── Repositories ─────────────────────────────────────────────────
	txManager := pgrepo.NewTxManager(db)
	queueRepo := pgrepo.NewQueueRepository(db, txManager)
	msgRepo := pgrepo.NewMessageRepository(db)
	outboxRepo := pgrepo.NewOutboxRepository(db)
	connRepo := pgrepo.NewConnectorRepository(db)

	// ── Prometheus Metrics ───────────────────────────────────────────
	promMetrics := connector.NewPrometheusMetricsRecorder("fury", "sms")

	// ── Circuit Breaker Store ────────────────────────────────────────
	cbStore := connector.NewCircuitBreakerStore(
		connector.WithFailureThreshold(5),
		connector.WithResetTimeout(30*time.Second),
		connector.WithOnStateChange(func(connectorID string, old, new connector.CircuitBreakerState) {
			slog.Warn("circuit breaker state changed",
				"connector_id", connectorID,
				"old_state", old.String(),
				"new_state", new.String(),
			)
			promMetrics.RecordCircuitBreakerStateChange(connectorID, new.String())
		}),
	)

	// ── Driver Registry ─────────────────────────────────────────────
	// Every protocol registers its driver here. Adding SMPP is just:
	//   driverRegistry.Register(smppdriver.NewDriver())
	driverRegistry := connector.NewDriverRegistry()
	driverRegistry.Register(httph.NewDriver())

	// ── Connector Registry (Memory) ──────────────────────────────────
	// Holds pre-initialized GenericConnector instances keyed by ID.
	connRegistry := connector.NewMemoryRegistry()

	// Load connectors from database and initialize GenericConnectors.
	tmplEngine := template.NewEngine()
	rulesEngine := rule.NewEngine()
	loadConnectors(ctx, connRepo, driverRegistry, connRegistry, tmplEngine, rulesEngine, cbStore, promMetrics)

	// ── Retry Policy ─────────────────────────────────────────────────
	retryPolicy := worker.NewDefaultRetryPolicy()

	// ── Workers ──────────────────────────────────────────────────────
	// QueueWorker uses ConnectorRegistry (by connector ID) — no sender map.
	qw := worker.NewQueueWorker(
		ctx,
		queueRepo,
		msgRepo,
		connRegistry, // ConnectorRegistry interface
		retryPolicy,
		promMetrics,
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

	// ── Worker Health Checker ────────────────────────────────────────
	workerHealth := worker.NewHealthChecker(qw, re, ow)

	// ── Router ───────────────────────────────────────────────────────
	app, err := router.New(cfg, db, rdb, eventBus, clock, workerHealth, promMetrics)
	if err != nil {
		slog.Error("failed to create router", "error", err)
		os.Exit(1)
	}

	// ── Start workers ────────────────────────────────────────────────
	ow.Start()
	re.Start()
	qw.Start()

	// ── Start HTTP server ────────────────────────────────────────────
	addr := fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port)
	go func() {
		slog.Info("HTTP server listening", "addr", addr)
		if err := app.Listen(addr); err != nil {
			slog.Error("server error", "error", err)
			cancel()
		}
	}()

	// ── Shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		slog.Info("shutting down gracefully...")
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.GracefulTimeout)
	defer shutdownCancel()

	slog.Info("draining workers...")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	qw.Stop()
	slog.Info("queue worker stopped")

	re.Stop()
	slog.Info("retry engine stopped")

	ow.Stop()
	slog.Info("outbox worker stopped")

	wg.Wait()
	eventBus.Close()

	if rdb != nil {
		if err := rdb.Close(); err != nil {
			slog.Error("redis close error", "error", err)
		}
	}

	db.Close()
	slog.Info("server stopped gracefully")
}

// loadConnectors reads all active connectors from the database and registers
// initialized GenericConnector instances in the MemoryRegistry.
func loadConnectors(
	ctx context.Context,
	connRepo domain.ConnectorRepository,
	driverRegistry connector.DriverRegistry,
	connRegistry *connector.MemoryRegistry,
	tmplEngine *template.Engine,
	rulesEngine *rule.Engine,
	cbStore connector.CircuitBreakerStore,
	metrics domain.MetricsRecorder,
) {
	activeStatus := domain.ConnectorStatusActive
	connectors, err := connRepo.ListByTenant(ctx, domain.ConnectorFilter{
		Status: &activeStatus,
		Page:   domain.Page{Offset: 0, Limit: 1000},
	})
	if err != nil {
		slog.Warn("failed to load connectors from DB, starting with empty registry", "error", err)
		return
	}

	for i := range connectors.Items {
		dc := &connectors.Items[i]
		driver, err := driverRegistry.Get(dc.Type)
		if err != nil {
			slog.Warn("no driver registered for connector", "connector_id", dc.ID, "type", dc.Type)
			continue
		}

		cfg := buildConnectorConfig(dc)
		if cfg == nil {
			continue
		}

		conn := connector.NewGenericConnector(
			dc.ID,
			dc.Type,
			*cfg,
			driver,
			connector.WithTemplateEngine(tmplEngine),
			connector.WithRuleEngine(rulesEngine),
			connector.WithMetricsRecorder(metrics),
			connector.WithCircuitBreakerStore(cbStore),
		)

		if err := connRegistry.Add(conn); err != nil {
			slog.Warn("failed to register connector", "connector_id", dc.ID, "error", err)
		} else {
			slog.Info("connector registered", "connector_id", dc.ID, "type", dc.Type)
		}
	}
}

// buildConnectorConfig converts a domain.Connector to connector.ConnectorConfig.
// This bridges the DB model to the GenericConnector's runtime config.
func buildConnectorConfig(dc *domain.Connector) *connector.ConnectorConfig {
	if dc == nil || len(dc.Config) == 0 {
		return nil
	}

	// Try to decode config as ConnectorConfig first (new format)
	var cfg connector.ConnectorConfig
	if err := json.Unmarshal(dc.Config, &cfg); err == nil {
		return &cfg
	}

	// Fallback: create a minimal config with the raw transport JSON.
	// This ensures legacy connectors still work during migration.
	return &connector.ConnectorConfig{
		Metadata: connector.MetadataConfig{
			ID:       dc.ID,
			Name:     dc.Name,
			Protocol: string(dc.Type),
		},
		Transport: dc.Config,
		Rules: connector.RuleConfig{
			Rules: []rule.Rule{
				{
					Condition: rule.Condition{Field: "status", Operator: "eq", Value: "200"},
					Actions:   []rule.Action{{Type: "accept"}},
				},
			},
		},
		Health: connector.HealthCheckConfig{Enabled: false},
	}
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
