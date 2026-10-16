package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
	"github.com/malikimayzar/arxiv-research-assistant/internal/middleware"
	"github.com/malikimayzar/arxiv-research-assistant/internal/repository"
)

type QueryRequest struct {
	Query   string `json:"query"`
	TopK    int    `json:"top_k"`
	Model   string `json:"model"`
	ArxivID string `json:"arxiv_id,omitempty"`
}

type QueryResponse struct {
	Answer              string          `json:"answer"`
	Sources             []client.Source `json:"sources"`
	RetrievalMs         int             `json:"retrieval_ms"`
	GenerationMs        int             `json:"generation_ms"`
	Model               string          `json:"model"`
	LogID               string          `json:"log_id,omitempty"`
	InsufficientContext bool            `json:"insufficient_context,omitempty"`
}

func QueryHandler(ml *client.MLClient, logRepo repository.QueryLogRepository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		requestID, _ := c.Locals("requestID").(string)

		var req QueryRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if req.Query == "" {
			return fiber.NewError(fiber.StatusBadRequest, "query cannot be empty")
		}
		if req.TopK == 0 {
			req.TopK = 5
		}
		if req.Model == "" {
			req.Model = "phi3:mini"
		}

		result, err := ml.Query(c.Context(), req.Query, req.TopK, req.Model, req.ArxivID, requestID)
		if err != nil {
			// Empty retrieval is a valid outcome, not a failure: answer 200 with
			// insufficient_context so clients can render it without an error box.
			var statusErr *client.StatusError
			if errors.As(err, &statusErr) && statusErr.StatusCode == fiber.StatusNotFound {
				middleware.IncFailureMode("insufficient_ctx")
				return c.JSON(QueryResponse{
					Answer:              "No relevant chunks found.",
					Sources:             []client.Source{},
					Model:               req.Model,
					InsufficientContext: true,
				})
			}

			middleware.IncFailureMode("error")
			fmt.Printf("⚠️  query failed (request_id=%s): %v\n", requestID, err)
			return fiber.NewError(fiber.StatusServiceUnavailable, "ml service unavailable")
		}

		// Observe metrics
		middleware.ObserveQdrantSearch(float64(result.RetrievalMs) / 1000)
		middleware.ObserveOllamaGeneration(float64(result.GenerationMs) / 1000)

		// Log to PostgreSQL async
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			logged, err := logRepo.Create(ctx, repository.CreateQueryLogParams{
				Query:        req.Query,
				Answer:       result.Answer,
				RetrievalMs:  result.RetrievalMs,
				GenerationMs: result.GenerationMs,
				Model:        result.Model,
			})
			if err != nil {
				fmt.Printf("⚠️  query log failed: %v\n", err)
				return
			}
			_ = logged
		}()

		// Track failure mode
		if result.Answer == "Not found in context." {
			middleware.IncFailureMode("insufficient_ctx")
		} else {
			middleware.IncFailureMode("correct")
		}

		return c.JSON(QueryResponse{
			Answer:       result.Answer,
			Sources:      result.Sources,
			RetrievalMs:  result.RetrievalMs,
			GenerationMs: result.GenerationMs,
			Model:        result.Model,
		})
	}
}

func QueryHistoryHandler(logRepo repository.QueryLogRepository) fiber.Handler {
	return func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 20)

		logs, err := logRepo.List(c.Context(), limit)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch history")
		}

		return c.JSON(fiber.Map{
			"logs":  logs,
			"total": len(logs),
		})
	}
}
