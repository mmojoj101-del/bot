package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Logger is a structured logging middleware.
func Logger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		latency := time.Since(start)
		rid := c.Locals("request_id")
		userID := c.Locals("user_id")
		tenantID := c.Locals("tenant_id")

		attrs := []slog.Attr{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", c.Response().StatusCode()),
			slog.Duration("latency", latency),
			slog.String("ip", c.IP()),
			slog.String("user_agent", c.Get("User-Agent")),
		}

		if rid != nil {
			attrs = append(attrs, slog.String("request_id", rid.(string)))
		}
		if userID != nil {
			attrs = append(attrs, slog.String("user_id", userID.(string)))
		}
		if tenantID != nil {
			attrs = append(attrs, slog.String("tenant_id", tenantID.(string)))
		}

		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}

		slog.LogAttrs(nil, slog.LevelInfo, "http_request", attrs...)

		return err
	}
}
