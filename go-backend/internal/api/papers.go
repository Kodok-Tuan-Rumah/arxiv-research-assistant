package api

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
	"github.com/malikimayzar/arxiv-research-assistant/internal/repository"
)

type PapersHandler struct {
	repo repository.PaperRepository
	ml   *client.MLClient
}

func NewPapersHandler(repo repository.PaperRepository, ml *client.MLClient) *PapersHandler {
	return &PapersHandler{repo: repo, ml: ml}
}

func (h *PapersHandler) List(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 20)
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	papers, total, err := h.repo.List(c.Context(), limit, offset)
	if err != nil {
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
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid paper ID")
	}

	paper, err := h.repo.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "paper not found")
	}

	return c.JSON(paper)
}

// Delete removes a paper together with its vectors. Order of operations:
// resolve the arxiv_id → purge vectors in the ML service → delete the PG row.
// Purging first keeps retries safe: a failed purge leaves everything intact,
// and a failed PG delete can simply be retried because the purge is idempotent.
// Aborting with 502 (instead of deleting the PG row anyway) guarantees that a
// deleted paper can never silently keep serving vectors.
func (h *PapersHandler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid paper ID")
	}

	paper, err := h.repo.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "paper not found")
	}

	if err := h.ml.DeletePaper(paper.ArxivID); err != nil {
		fmt.Printf("⚠️  vector purge failed (paper_id=%s, arxiv_id=%s): %v\n", id, paper.ArxivID, err)
		return fiber.NewError(fiber.StatusBadGateway, "failed to purge paper vectors")
	}

	if err := h.repo.Delete(c.Context(), id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to delete paper")
	}

	return c.JSON(fiber.Map{"deleted": true, "id": id})
}
