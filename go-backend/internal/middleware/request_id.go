package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Keep the id supplied by an upstream caller (the gateway) so one user
		// request can be followed across services. Only mint a new one when the
		// header is absent or empty.
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("X-Request-ID", requestID)
		c.Locals("requestID", requestID)
		c.Response().Header.Set("X-Request-ID", requestID)
		return c.Next()
	}
}
