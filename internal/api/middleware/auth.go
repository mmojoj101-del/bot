package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
	jwtpkg "github.com/raghna/fury-sms-gateway/internal/pkg/jwt"
)

// AuthContext holds the authenticated user information.
type AuthContext struct {
	UserID       string
	TenantID     string
	Role         string
	IsSuperAdmin bool
}

// JWTAuth returns middleware that validates JWT access tokens.
func JWTAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Get("Authorization")
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing authorization header",
			})
		}

		// Strip "Bearer " prefix
		if len(token) < 7 || token[:7] != "Bearer " {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid authorization format",
			})
		}
		token = token[7:]

		claims, err := jwtpkg.ValidateToken(token, secret)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		c.Locals("user_id", claims.Sub)
		c.Locals("tenant_id", claims.TenantID)
		c.Locals("role", claims.Role)
		c.Locals("is_super_admin", claims.IsSuperAdmin)
		c.Locals("auth_method", "jwt")

		return c.Next()
	}
}

// APIKeyAuth returns middleware that validates API keys.
func APIKeyAuth(secret string, apiKeyService interface {
	ValidateAPIKey(ctx interface{}, key, requestID, ipAddress string) (*domain.APIKey, error)
}) fiber.Handler {
	return func(c *fiber.Ctx) error {
		apiKey := c.Get("X-API-Key")
		if apiKey == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing X-API-Key header",
			})
		}

		rid := c.Locals("request_id").(string)
		key, err := apiKeyService.ValidateAPIKey(nil, apiKey, rid, c.IP())
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid API key",
			})
		}

		c.Locals("user_id", "")
		c.Locals("tenant_id", key.TenantID)
		c.Locals("role", "api_user")
		c.Locals("is_super_admin", false)
		c.Locals("auth_method", "api_key")

		return c.Next()
	}
}
