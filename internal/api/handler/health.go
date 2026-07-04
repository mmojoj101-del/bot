package handler

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	db    *pgxpool.Pool
	rdb   *redis.Client
	migrated func() bool // returns true if migrations are applied
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(db *pgxpool.Pool, rdb *redis.Client, migrated func() bool) *HealthHandler {
	return &HealthHandler{
		db:        db,
		rdb:       rdb,
		migrated:  migrated,
	}
}

// Health returns 200 OK (liveness check).
func (h *HealthHandler) Health(c *fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": "ok",
	})
}

// Readiness returns 200 OK if the service is ready (readiness check).
func (h *HealthHandler) Readiness(c *fiber.Ctx) error {
	// Check database
	if err := h.db.Ping(c.Context()); err != nil {
		slog.Warn("readiness check failed: database", "error", err)
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not ready",
			"reason": "database unavailable",
		})
	}

	// Check Redis
	if err := h.rdb.Ping(c.Context()).Err(); err != nil {
		slog.Warn("readiness check failed: redis", "error", err)
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not ready",
			"reason": "redis unavailable",
		})
	}

	// Check migrations
	if h.migrated != nil && !h.migrated() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "not ready",
			"reason": "migrations pending",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status": "ready",
	})
}

// MetricsHandler returns the Prometheus metrics handler for Fiber.
func MetricsHandler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.Handler())
}
