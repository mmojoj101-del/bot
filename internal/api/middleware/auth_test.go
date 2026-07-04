package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	jwtpkg "github.com/raghna/fury-sms-gateway/internal/pkg/jwt"
)

const testSecret = "test-secret-key-for-unit-tests-min-32-chars!!"

func TestJWTAuth_ValidToken(t *testing.T) {
	app := fiber.New()
	app.Use(JWTAuth(testSecret))

	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	token, err := jwtpkg.GenerateAccessToken(testSecret, "user-1", "tenant-1", "admin", false, 15*time.Minute, "test", "test")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	resp, err := app.Test(getRequestWithBearer("/protected", token))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestJWTAuth_MissingHeader(t *testing.T) {
	app := fiber.New()
	app.Use(JWTAuth(testSecret))
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/protected", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("missing header: expected 401, got %d", resp.StatusCode)
	}
}

func TestJWTAuth_InvalidFormat(t *testing.T) {
	app := fiber.New()
	app.Use(JWTAuth(testSecret))
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	resp, err := app.Test(getRequestWithBearer("/protected", "invalid-token"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("invalid token: expected 401, got %d", resp.StatusCode)
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	app := fiber.New()
	app.Use(JWTAuth(testSecret))
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	token, err := jwtpkg.GenerateAccessToken(testSecret, "user-1", "tenant-1", "admin", false, -1*time.Second, "test", "test")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	resp, err := app.Test(getRequestWithBearer("/protected", token))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("expired: expected 401, got %d", resp.StatusCode)
	}
}

func TestJWTAuth_WrongSecret(t *testing.T) {
	app := fiber.New()
	app.Use(JWTAuth("different-secret-for-testing-purposes!!"))
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	token, err := jwtpkg.GenerateAccessToken(testSecret, "user-1", "tenant-1", "admin", false, 15*time.Minute, "test", "test")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	resp, err := app.Test(getRequestWithBearer("/protected", token))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("wrong secret: expected 401, got %d", resp.StatusCode)
	}
}

func TestJWTAuth_SetsLocals(t *testing.T) {
	app := fiber.New()
	app.Use(JWTAuth(testSecret))
	app.Get("/protected", func(c *fiber.Ctx) error {
		if c.Locals("user_id").(string) != "user-1" {
			t.Fatal("user_id not set correctly")
		}
		if c.Locals("tenant_id").(string) != "tenant-1" {
			t.Fatal("tenant_id not set correctly")
		}
		if c.Locals("role").(string) != "admin" {
			t.Fatal("role not set correctly")
		}
		if c.Locals("auth_method").(string) != "jwt" {
			t.Fatal("auth_method not set correctly")
		}
		return c.SendString("ok")
	})

	token, err := jwtpkg.GenerateAccessToken(testSecret, "user-1", "tenant-1", "admin", false, 15*time.Minute, "test", "test")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	resp, err := app.Test(getRequestWithBearer("/protected", token))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestJWTAuth_WithoutBearerPrefix(t *testing.T) {
	app := fiber.New()
	app.Use(JWTAuth(testSecret))
	app.Get("/protected", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	token, err := jwtpkg.GenerateAccessToken(testSecret, "user-1", "tenant-1", "admin", false, 15*time.Minute, "test", "test")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// Send without "Bearer " prefix
	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", token)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("no Bearer: expected 401, got %d", resp.StatusCode)
	}
}

// getRequestWithBearer creates a GET request with Bearer auth header.
func getRequestWithBearer(path, token string) *http.Request {
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}
