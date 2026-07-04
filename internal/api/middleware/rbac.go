package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

// RBACMiddleware checks if the authenticated user has the required role(s).
type RBACMiddleware struct {
}

// NewRBACMiddleware creates a new RBAC middleware.
func NewRBACMiddleware() *RBACMiddleware {
	return &RBACMiddleware{}
}

// RequireRole returns middleware that requires at least one of the specified roles.
func (r *RBACMiddleware) RequireRole(roles ...domain.MemberRole) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Super admin bypasses role checks
		if isSuperAdmin, ok := c.Locals("is_super_admin").(bool); ok && isSuperAdmin {
			return c.Next()
		}

		userRole := c.Locals("role")
		if userRole == nil || userRole.(string) == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "access denied",
			})
		}

		roleStr := userRole.(string)
		for _, role := range roles {
			if string(role) == roleStr {
				return c.Next()
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "insufficient permissions",
		})
	}
}

// RequireSuperAdmin returns middleware that requires super admin privileges.
func (r *RBACMiddleware) RequireSuperAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		isSuperAdmin, ok := c.Locals("is_super_admin").(bool)
		if !ok || !isSuperAdmin {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "super admin access required",
			})
		}
		return c.Next()
	}
}
