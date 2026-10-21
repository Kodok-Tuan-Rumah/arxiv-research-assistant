package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
)

// TestMetricsDoesNotRetainMutableFiberStrings guards against a regression where
// Prometheus label values aliased Fiber's request buffer. Once the context was
// reused, a previous method label could change from "POST" to "GETT", causing
// duplicate series and a 500 from /metrics.
func TestMetricsDoesNotRetainMutableFiberStrings(t *testing.T) {
	app := fiber.New()
	app.Use(Metrics())
	app.Post("/query", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	for i := 0; i < 8; i++ {
		req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader("{}"))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("POST /query: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		req = httptest.NewRequest(http.MethodGet, "/health", nil)
		resp, err = app.Test(req)
		if err != nil {
			t.Fatalf("GET /health: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("metrics gather failed: %v", err)
	}

	for _, family := range families {
		if family.GetName() != "http_requests_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() != "method" {
					continue
				}
				if label.GetValue() != http.MethodGet && label.GetValue() != http.MethodPost {
					t.Fatalf("corrupted method label: %q", label.GetValue())
				}
			}
		}
	}
}
