package api

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
	"github.com/malikimayzar/arxiv-research-assistant/internal/logging"
	"github.com/malikimayzar/arxiv-research-assistant/internal/repository"
)

type PapersHandler struct {
	repo   repository.PaperRepository
	ml     *client.MLClient
	logger *logging.Logger
}

func NewPapersHandler(repo repository.PaperRepository, ml *client.MLClient, logger *logging.Logger) *PapersHandler {
	return &PapersHandler{repo: repo, ml: ml, logger: logger}
}

func (h *PapersHandler) List(c *fiber.Ctx) error {
	requestID, _ := c.Locals("requestID").(string)
	limit := c.QueryInt("limit", 20)
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	papers, total, err := h.repo.List(c.Context(), limit, offset)
	if err != nil {
		h.logger.Error("failed to fetch papers", logging.Fields{
			"request_id": requestID,
			"error":      err.Error(),
		})
		return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch papers")
	}

	return c.JSON(fiber.Map{
		"papers": papers,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

func (h *PapersHandler) GetByID(c *fiber.Ctx) error {
	requestID, _ := c.Locals("requestID").(string)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		h.logger.Warning("invalid paper ID", logging.Fields{
			"request_id": requestID,
			"paper_id":   c.Params("id"),
		})
		return fiber.NewError(fiber.StatusBadRequest, "invalid paper ID")
	}

	paper, err := h.repo.GetByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.logger.Info("paper not found", logging.Fields{
				"request_id": requestID,
				"paper_id":   id.String(),
			})
			return fiber.NewError(fiber.StatusNotFound, "paper not found")
		}
		h.logger.Error("failed to fetch paper", logging.Fields{
			"request_id": requestID,
			"paper_id":   id.String(),
			"error":      err.Error(),
		})
		return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch paper")
	}

	return c.JSON(paper)
}

func (h *PapersHandler) Delete(c *fiber.Ctx) error {
	requestID, _ := c.Locals("requestID").(string)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		h.logger.Warning("invalid paper ID", logging.Fields{
			"request_id": requestID,
			"paper_id":   c.Params("id"),
		})
		return fiber.NewError(fiber.StatusBadRequest, "invalid paper ID")
	}

	paper, err := h.repo.GetByID(c.Context(), id)
	if err != nil {
		h.logger.Info("paper not found", logging.Fields{
			"request_id": requestID,
			"paper_id":   id.String(),
		})
		return fiber.NewError(fiber.StatusNotFound, "paper not found")
	}

	h.logger.Info("deleting paper", logging.Fields{
		"request_id": requestID,
		"paper_id":   id.String(),
		"arxiv_id":   paper.ArxivID,
	})

	if err := h.ml.DeletePaper(paper.ArxivID); err != nil {
		h.logger.Warning("vector purge failed", logging.Fields{
			"request_id": requestID,
			"paper_id":   id.String(),
			"arxiv_id":   paper.ArxivID,
			"error":      err.Error(),
		})
		return fiber.NewError(fiber.StatusBadGateway, "failed to purge paper vectors")
	}

	if err := h.repo.Delete(c.Context(), id); err != nil {
		h.logger.Error("failed to delete paper", logging.Fields{
			"request_id": requestID,
			"paper_id":   id.String(),
			"error":      err.Error(),
		})
		return fiber.NewError(fiber.StatusInternalServerError, "failed to delete paper")
	}

	h.logger.Info("paper deleted", logging.Fields{
		"request_id": requestID,
		"paper_id":   id.String(),
	})

	return c.JSON(fiber.Map{"deleted": true, "id": id})
}
