package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ErrNotFound is returned when a requested paper does not exist.
var ErrNotFound = errors.New("paper not found")

type Paper struct {
	ID         uuid.UUID      `json:"id"          db:"id"`
	ArxivID    string         `json:"arxiv_id"    db:"arxiv_id"`
	Title      string         `json:"title"       db:"title"`
	Authors    pq.StringArray `json:"authors"     db:"authors"`
	Abstract   string         `json:"abstract"    db:"abstract"`
	Categories pq.StringArray `json:"categories"  db:"categories"`
	Published  *time.Time     `json:"published"   db:"published"`
	IngestedAt time.Time      `json:"ingested_at" db:"ingested_at"`
	ChunkCount int            `json:"chunk_count" db:"chunk_count"`
}

type PaperRepository interface {
	List(ctx context.Context, limit, offset int) ([]Paper, int, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Paper, error)
	GetByArxivID(ctx context.Context, arxivID string) (*Paper, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type paperRepo struct {
	db *sqlx.DB
}

func NewPaperRepository(db *sqlx.DB) PaperRepository {
	return &paperRepo{db: db}
}

func (r *paperRepo) List(ctx context.Context, limit, offset int) ([]Paper, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var total int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM papers`).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("paperRepo.List count: %w", err)
	}

	var papers []Paper
	err = r.db.SelectContext(ctx, &papers, `
		SELECT p.id, p.arxiv_id, p.title, p.authors, p.abstract,
		       p.categories, p.published, p.ingested_at,
		       COUNT(c.id) AS chunk_count
		FROM papers p
		LEFT JOIN chunks c ON c.paper_id = p.id
		GROUP BY p.id
		ORDER BY p.ingested_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("paperRepo.List: %w", err)
	}

	if papers == nil {
		papers = []Paper{}
	}

	return papers, total, nil
}

func (r *paperRepo) GetByID(ctx context.Context, id uuid.UUID) (*Paper, error) {
	var p Paper
	err := r.db.QueryRowxContext(ctx, `
		SELECT p.id, p.arxiv_id, p.title, p.authors, p.abstract,
		       p.categories, p.published, p.ingested_at,
		       COUNT(c.id) AS chunk_count
		FROM papers p
		LEFT JOIN chunks c ON c.paper_id = p.id
		WHERE p.id = $1
		GROUP BY p.id`, id).StructScan(&p)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("paperRepo.GetByID: %w", err)
	}
	return &p, nil
}

func (r *paperRepo) GetByArxivID(ctx context.Context, arxivID string) (*Paper, error) {
	var p Paper
	err := r.db.QueryRowxContext(ctx, `
		SELECT p.id, p.arxiv_id, p.title, p.authors, p.abstract,
		       p.categories, p.published, p.ingested_at,
		       COUNT(c.id) AS chunk_count
		FROM papers p
		LEFT JOIN chunks c ON c.paper_id = p.id
		WHERE p.arxiv_id = $1
		GROUP BY p.id`, arxivID).StructScan(&p)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("paperRepo.GetByArxivID: %w", err)
	}
	return &p, nil
}

func (r *paperRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM papers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("paperRepo.Delete: %w", err)
	}
	return nil
}
