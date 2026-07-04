package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRequestID_GeneratesID(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/test", func(c *fiber.Ctx) error {
		rid := c.Locals("request_id")
		if rid == nil || rid.(string) == "" {
			t.Fatal("request_id should not be empty")
		}
		return c.SendString("ok")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID header should be set")
	}
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/test", func(c *fiber.Ctx) error {
		rid := c.Locals("request_id").(string)
		if rid != "existing-id" {
			t.Fatalf("request_id = %s, want existing-id", rid)
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "existing-id")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.Header.Get("X-Request-ID") != "existing-id" {
		t.Fatalf("X-Request-ID = %s, want existing-id", resp.Header.Get("X-Request-ID"))
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	app := fiber.New()
	var id1, id2 string

	app.Use(RequestID())
	app.Get("/test", func(c *fiber.Ctx) error {
		rid := c.Locals("request_id").(string)
		if id1 == "" {
			id1 = rid
		} else {
			id2 = rid
		}
		return c.SendString("ok")
	})

	app.Test(httptest.NewRequest("GET", "/test", nil))
	app.Test(httptest.NewRequest("GET", "/test", nil))

	if id1 == id2 {
		t.Fatal("each request should have a unique request_id")
	}
}
