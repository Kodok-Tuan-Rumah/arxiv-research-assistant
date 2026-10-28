package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
)

// HealthResponse describes the health of the API and its dependencies.
type HealthResponse struct {
	Status   string            `json:"status"`
	Version  string            `json:"version"`
	Services map[string]string `json:"services"`
}

// qdrantClient is a small interface so the health check can be unit-tested
type qdrantClient interface {
	Health(ctx context.Context) error
}

// QdrantClient checks Qdrant readiness via its /healthz endpoint.
type QdrantClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewQdrantClient creates a Qdrant health-check client.
func NewQdrantClient(baseURL string) *QdrantClient {
	return &QdrantClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// Health checks Qdrant readiness via its /healthz endpoint.
func (q *QdrantClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := q.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &statusError{code: resp.StatusCode}
	}
	return nil
}

type statusError struct {
	code int
}

func (e *statusError) Error() string {
	return "qdrant returned status " + strconv.Itoa(e.code)
}

func HealthHandler(db *sqlx.DB, ml *client.MLClient, qdrant qdrantClient) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
		defer cancel()

		services := map[string]string{
			"postgres": "ok",
			"qdrant":   "ok",
			"ml":       "ok",
		}

		overallStatus := "ok"

		if db == nil || db.PingContext(ctx) != nil {
			services["postgres"] = "error"
			overallStatus = "degraded"
		}

		if qdrant == nil || qdrant.Health(ctx) != nil {
			services["qdrant"] = "error"
			overallStatus = "degraded"
		}

		if ml == nil || ml.Ping() != nil {
			services["ml"] = "error"
			overallStatus = "degraded"
		}

		return c.JSON(HealthResponse{
			Status:   overallStatus,
			Version:  "0.1.0",
			Services: services,
		})
	}
}
