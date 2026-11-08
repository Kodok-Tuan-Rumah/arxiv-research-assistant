package middleware

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/malikimayzar/arxiv-research-assistant/internal/logging"
)

var log = logging.NewLogger("arxiv-go-backend-middleware", logging.INFO)

// internalServiceToken lazily reads the shared service token from the
// environment. It is read once per request so rotation without restart works.
func internalServiceToken() string {
	return strings.TrimSpace(os.Getenv("INTERNAL_SERVICE_TOKEN"))
}

// InternalServiceAuth rejects requests that do not present a valid
// X-Internal-Service-Token. Health, ready, and metrics endpoints are exempt so
// load balancers and the gateway readiness probe can still probe the service.
// When INTERNAL_SERVICE_TOKEN is not configured, all requests are rejected
// (fail-closed) to avoid an open backend.
func InternalServiceAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()
		if path == "/health" || path == "/ready" || path == "/metrics" {
			return c.Next()
		}

		expected := internalServiceToken()
		if expected == "" {
			log.Warning("INTERNAL_SERVICE_TOKEN not configured; rejecting request",
				logging.Fields{"path": path})
			return fiber.NewError(fiber.StatusServiceUnavailable, "service not configured")
		}

		got := strings.TrimSpace(c.Get("X-Internal-Service-Token"))
		if got == "" || got != expected {
			log.Warning("invalid or missing internal service token",
				logging.Fields{"path": path})
			return fiber.NewError(fiber.StatusUnauthorized, "invalid internal service token")
		}

		return c.Next()
	}
}
