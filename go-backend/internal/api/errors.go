package api

import "github.com/gofiber/fiber/v2"

type ErrorResponse struct {
	Status    int    `json:"status"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "internal server error"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	requestID, _ := c.Locals("requestID").(string)

	return c.Status(code).JSON(ErrorResponse{
		Status:    code,
		Message:   message,
		RequestID: requestID,
	})
}