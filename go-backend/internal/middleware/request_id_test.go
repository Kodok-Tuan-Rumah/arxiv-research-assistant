package middleware_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
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
