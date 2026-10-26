package middleware_test

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/malikimayzar/arxiv-research-assistant/internal/middleware"
)

func TestRequestID_SetsResponseHeader(t *testing.T) {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("pong")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/ping", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID response header to be set")
	}
}

// echoRequestID builds an app whose handler echoes the id stored in Locals.
func echoRequestID() *fiber.App {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		id, _ := c.Locals("requestID").(string)
		return c.SendString(id)
	})
	return app
}

// The gateway sends X-Request-ID downstream so a single user request can be
// followed across services. Overwriting it with a fresh UUID breaks that join.
func TestRequestID_HonorsIncomingHeader(t *testing.T) {
	const incoming = "trace-abc-123"

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", incoming)

	resp, err := echoRequestID().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if got := string(body); got != incoming {
		t.Errorf("locals requestID = %q, want %q", got, incoming)
	}
	if got := resp.Header.Get("X-Request-ID"); got != incoming {
		t.Errorf("response X-Request-ID = %q, want %q", got, incoming)
	}
}

func TestRequestID_GeneratesWhenHeaderEmpty(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "")

	resp, err := echoRequestID().Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if _, err := uuid.Parse(string(body)); err != nil {
		t.Errorf("generated requestID %q is not a UUID: %v", body, err)
	}
}
