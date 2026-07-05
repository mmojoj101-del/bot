package handler

import (
	"log/slog"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// Build information set by ldflags in main.
var (
	Version   = "0.1.0"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// WorkerHealthChecker provides detailed health info for all workers.
type WorkerHealthChecker interface {
	// AllHealthy returns true only if every worker is healthy.
	AllHealthy() bool
	// Details returns a map of worker-type → health detail (for JSON encoding).
	Details() map[string]map[string]interface{}
}

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	db       *pgxpool.Pool
	rdb      *redis.Client
	workers  WorkerHealthChecker
}

// NewHealthHandler creates a new health handler with worker-aware readiness.
func NewHealthHandler(db *pgxpool.Pool, rdb *redis.Client, workers WorkerHealthChecker) *HealthHandler {
	return &HealthHandler{
		db:      db,
		rdb:     rdb,
		workers: workers,
	}
}

// startTime records when the handler was instantiated (≈ process start).
var startTime = time.Now()

// Health returns 200 OK (liveness). Always responds — for load balancers.
func (h *HealthHandler) Health(c *fiber.Ctx) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":      "ok",
		"service":     "fury-sms-gateway",
		"version":     Version,
		"git_commit":  GitCommit,
		"build_date":  BuildDate,
		"uptime_sec":  time.Since(startTime).Seconds(),
		"time":        time.Now().UTC().Format(time.RFC3339),
		"goroutines":  runtime.NumGoroutine(),
		"memory_kb":   m.Alloc / 1024,
		"gc_pauses":   m.NumGC,
	})
}

// readinessResult holds the readiness check outcome.
type readinessResult struct {
	Status   string                            `json:"status"`
	Checks   map[string]checkResult            `json:"checks,omitempty"`
	Workers  map[string]map[string]interface{} `json:"workers,omitempty"`
}

type checkResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Readiness returns 200 only if DB, Redis, and workers are all healthy.
// Includes detailed per-check results for debugging.
func (h *HealthHandler) Readiness(c *fiber.Ctx) error {
	result := readinessResult{
		Status: "ready",
		Checks: make(map[string]checkResult),
	}

	// 1. Database check
	if err := h.db.Ping(c.Context()); err != nil {
		slog.Warn("readiness: database failed", "error", err)
		result.Checks["database"] = checkResult{Status: "unhealthy", Error: err.Error()}
	} else {
		result.Checks["database"] = checkResult{Status: "healthy"}
	}

	// 2. Redis check
	if h.rdb != nil {
		if err := h.rdb.Ping(c.Context()).Err(); err != nil {
			slog.Warn("readiness: redis failed", "error", err)
			result.Checks["redis"] = checkResult{Status: "unhealthy", Error: err.Error()}
		} else {
			result.Checks["redis"] = checkResult{Status: "healthy"}
		}
	} else {
		result.Checks["redis"] = checkResult{Status: "degraded", Error: "not connected (cached mode)"}
	}

	// 3. Workers check
	if h.workers != nil {
		if h.workers.AllHealthy() {
			result.Checks["workers"] = checkResult{Status: "healthy"}
		} else {
			result.Checks["workers"] = checkResult{Status: "degraded"}
		}
		result.Workers = h.workers.Details()
	}

	// Determine overall status: all must be healthy (redis can be degraded)
	for name, check := range result.Checks {
		if check.Status == "unhealthy" {
			result.Status = "not ready"
			slog.Warn("readiness check failed", "check", name, "reason", check.Error)
		}
	}

	if result.Status == "ready" {
		return c.Status(fiber.StatusOK).JSON(result)
	}
	return c.Status(fiber.StatusServiceUnavailable).JSON(result)
}

// MetricsHandler returns the Prometheus metrics handler for Fiber.
func MetricsHandler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.Handler())
}
