package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/malikimayzar/arxiv-research-assistant/internal/api"
)

type fakeQdrantOK struct{}

func (fakeQdrantOK) Health(context.Context) error { return nil }

type fakeQdrantFail struct{}

func (fakeQdrantFail) Health(context.Context) error { return context.DeadlineExceeded }

func TestHealthHandler_NoDB(t *testing.T) {
	app := fiber.New()
	app.Get("/health", api.HealthHandler(nil, nil, nil))

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result api.HealthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", result.Status)
	}

	if result.Services["postgres"] != "error" {
		t.Errorf("expected postgres error, got '%s'", result.Services["postgres"])
	}
}

func TestHealthHandler_QdrantHealthy(t *testing.T) {
	app := fiber.New()
	app.Get("/health", api.HealthHandler(nil, nil, fakeQdrantOK{}))

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	var result api.HealthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// db and ml are nil → overall degraded, but qdrant specifically must be "ok"
	if result.Status != "degraded" {
		t.Errorf("expected status 'degraded' (nil db/ml), got '%s'", result.Status)
	}
	if result.Services["qdrant"] != "ok" {
		t.Errorf("expected qdrant 'ok', got '%s'", result.Services["qdrant"])
	}
}

func TestHealthHandler_QdrantUnhealthy(t *testing.T) {
	app := fiber.New()
	app.Get("/health", api.HealthHandler(nil, nil, fakeQdrantFail{}))

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	var result api.HealthResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", result.Status)
	}
	if result.Services["qdrant"] != "error" {
		t.Errorf("expected qdrant 'error', got '%s'", result.Services["qdrant"])
	}
}
