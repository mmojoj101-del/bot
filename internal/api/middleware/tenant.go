package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// TenantContext extracts tenant context from the JWT claims or X-Tenant-ID header.
func TenantContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Tenant might already be set by JWT auth
		tenantID := c.Locals("tenant_id")
		if tenantID == nil || tenantID.(string) == "" {
			// Try header override (for super admins)
			if headerTenant := c.Get("X-Tenant-ID"); headerTenant != "" {
				c.Locals("tenant_id", headerTenant)
			}
		}
		return c.Next()
	}
}
