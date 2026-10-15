package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID := uuid.New().String()
		c.Set("X-Request-ID", requestID)
		c.Locals("requestID", requestID)
		c.Response().Header.Set("X-Request-ID", requestID)
		return c.Next()
	}
}
