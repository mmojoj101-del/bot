package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/raghna/fury-sms-gateway/internal/domain"
)

func TestRequireRole_AdminAllowed(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	app.Get("/admin", func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		c.Locals("is_super_admin", false)
		return c.Next()
	}, rbac.RequireRole(domain.MemberRoleAdmin), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/admin", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("admin: expected 200, got %d", resp.StatusCode)
	}
}

func TestRequireRole_OperatorDeniedForAdminOnly(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	app.Get("/admin", func(c *fiber.Ctx) error {
		c.Locals("role", "operator")
		c.Locals("is_super_admin", false)
		return c.Next()
	}, rbac.RequireRole(domain.MemberRoleAdmin), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/admin", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("operator: expected 403, got %d", resp.StatusCode)
	}
}

func TestRequireRole_MultipleRoles(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	handler := func(c *fiber.Ctx) error {
		c.Locals("role", "api_user")
		c.Locals("is_super_admin", false)
		return c.Next()
	}

	app.Get("/restricted", handler, rbac.RequireRole(domain.MemberRoleAdmin, domain.MemberRoleOperator), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/restricted", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("api_user: expected 403, got %d", resp.StatusCode)
	}
}

func TestRequireRole_OperatorAllowedToOperatorOrAdmin(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	app.Get("/endpoint", func(c *fiber.Ctx) error {
		c.Locals("role", "operator")
		c.Locals("is_super_admin", false)
		return c.Next()
	}, rbac.RequireRole(domain.MemberRoleAdmin, domain.MemberRoleOperator), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/endpoint", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("operator: expected 200, got %d", resp.StatusCode)
	}
}

func TestRequireSuperAdmin_Allowed(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	app.Get("/super", func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		c.Locals("is_super_admin", true)
		return c.Next()
	}, rbac.RequireSuperAdmin(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/super", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("super admin: expected 200, got %d", resp.StatusCode)
	}
}

func TestRequireSuperAdmin_Denied(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	app.Get("/super", func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		c.Locals("is_super_admin", false)
		return c.Next()
	}, rbac.RequireSuperAdmin(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/super", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("non-super: expected 403, got %d", resp.StatusCode)
	}
}

func TestRequireRole_SuperAdminBypass(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	app.Get("/operator-only", func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		c.Locals("is_super_admin", true)
		return c.Next()
	}, rbac.RequireRole(domain.MemberRoleOperator), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/operator-only", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("super admin bypass: expected 200, got %d", resp.StatusCode)
	}
}

func TestRequireRole_NoRoleSet(t *testing.T) {
	app := fiber.New()
	rbac := NewRBACMiddleware()

	app.Get("/test", func(c *fiber.Ctx) error {
		// Don't set role
		return c.Next()
	}, rbac.RequireRole(domain.MemberRoleAdmin), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("no role: expected 403, got %d", resp.StatusCode)
	}
}
