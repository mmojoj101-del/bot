package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RequestID adds a unique request ID to each request.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rid := c.Get("X-Request-ID")
		if rid == "" {
			rid = uuid.New().String()
		}
		c.Set("X-Request-ID", rid)
		c.Locals("request_id", rid)
		return c.Next()
	}
}
