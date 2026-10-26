package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type QueryLog struct {
	ID           uuid.UUID `json:"id"            db:"id"`
	Query        string    `json:"query"          db:"query"`
	Answer       string    `json:"answer"         db:"answer"`
	RetrievalMs  int       `json:"retrieval_ms"   db:"retrieval_ms"`
	GenerationMs int       `json:"generation_ms"  db:"generation_ms"`
	Faithfulness *float64  `json:"faithfulness,omitempty" db:"faithfulness"`
	FailureMode  *string   `json:"failure_mode,omitempty" db:"failure_mode"`
	Model        string    `json:"model"          db:"model"`
	RequestID    *string   `json:"request_id,omitempty" db:"request_id"`
	CreatedAt    time.Time `json:"created_at"     db:"created_at"`
}

type CreateQueryLogParams struct {
	Query        string
	Answer       string
	RetrievalMs  int
	GenerationMs int
	Faithfulness *float64
	FailureMode  *string
	Model        string
	// RequestID is the X-Request-ID propagated by the gateway. It is the only
	// join key between gateway_query_logs and query_logs.
	RequestID string
}

type QueryLogRepository interface {
	Create(ctx context.Context, params CreateQueryLogParams) (*QueryLog, error)
	List(ctx context.Context, limit int) ([]QueryLog, error)
}

type queryLogRepo struct {
	db *sqlx.DB
}

func NewQueryLogRepository(db *sqlx.DB) QueryLogRepository {
	return &queryLogRepo{db: db}
}

func (r *queryLogRepo) Create(ctx context.Context, params CreateQueryLogParams) (*QueryLog, error) {
	l := &QueryLog{
		ID:           uuid.New(),
		Query:        params.Query,
		Answer:       params.Answer,
		RetrievalMs:  params.RetrievalMs,
		GenerationMs: params.GenerationMs,
		Faithfulness: params.Faithfulness,
		FailureMode:  params.FailureMode,
		Model:        params.Model,
	}
	// Store NULL rather than "" when no id was propagated, so a missing
	// correlation is visible in queries instead of looking like a real id.
	if params.RequestID != "" {
		requestID := params.RequestID
		l.RequestID = &requestID
	}

	err := r.db.QueryRowxContext(ctx, `
		INSERT INTO query_logs (id, query, answer, retrieval_ms, generation_ms, faithfulness, failure_mode, model, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at`,
		l.ID, l.Query, l.Answer,
		l.RetrievalMs, l.GenerationMs,
		l.Faithfulness, l.FailureMode, l.Model, l.RequestID,
	).Scan(&l.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("queryLogRepo.Create: %w", err)
	}

	return l, nil
}

func (r *queryLogRepo) List(ctx context.Context, limit int) ([]QueryLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var logs []QueryLog
	err := r.db.SelectContext(ctx, &logs, `
		SELECT id, query, answer, retrieval_ms, generation_ms,
		       faithfulness, failure_mode, model, request_id, created_at
		FROM query_logs
		ORDER BY created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("queryLogRepo.List: %w", err)
	}

	if logs == nil {
		logs = []QueryLog{}
	}

	return logs, nil
}
