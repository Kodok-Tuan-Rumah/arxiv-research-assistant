package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/malikimayzar/arxiv-research-assistant/internal/api"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
	"github.com/malikimayzar/arxiv-research-assistant/internal/repository"
)

type fakeLogRepo struct{}

func (fakeLogRepo) Create(_ context.Context, _ repository.CreateQueryLogParams) (*repository.QueryLog, error) {
	return &repository.QueryLog{}, nil
}

func (fakeLogRepo) List(_ context.Context, _ int) ([]repository.QueryLog, error) {
	return []repository.QueryLog{}, nil
}

const testRequestID = "test-req-123"

// fixedRequestIDMiddleware simulates middleware.RequestID() with a deterministic id.
func fixedRequestIDMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("requestID", testRequestID)
		return c.Next()
	}
}

func TestQueryHandler_MLFailureIsGenericAndRequestIDForwarded(t *testing.T) {
	var gotRequestID string
	mlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-ID")
		http.Error(w, "super-secret dial tcp 10.0.0.1:8001", http.StatusInternalServerError)
	}))
	defer mlSrv.Close()

	ml := client.NewMLClient(mlSrv.URL)

	app := fiber.New(fiber.Config{ErrorHandler: api.ErrorHandler})
	app.Use(fixedRequestIDMiddleware())
	app.Post("/query", api.QueryHandler(ml, fakeLogRepo{}))

	req := httptest.NewRequest("POST", "/query", strings.NewReader(`{"query":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp api.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errResp.Message != "ml service unavailable" {
		t.Errorf("expected generic message %q, got %q", "ml service unavailable", errResp.Message)
	}
	if errResp.RequestID != testRequestID {
		t.Errorf("expected request_id %q, got %q", testRequestID, errResp.RequestID)
	}

	raw := string(body)
	for _, leak := range []string{"secret", "dial tcp", "returned status"} {
		if strings.Contains(raw, leak) {
			t.Errorf("response leaks internal detail %q: %s", leak, raw)
		}
	}

	if gotRequestID != testRequestID {
		t.Errorf("ML service did not receive request id: got %q", gotRequestID)
	}
}

func TestQueryHandler_SuccessForwardsRequestID(t *testing.T) {
	var gotRequestID string
	mlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answer":"ok","sources":[],"retrieval_ms":1,"generation_ms":2,"model":"test-model"}`))
	}))
	defer mlSrv.Close()

	ml := client.NewMLClient(mlSrv.URL)

	app := fiber.New(fiber.Config{ErrorHandler: api.ErrorHandler})
	app.Use(fixedRequestIDMiddleware())
	app.Post("/query", api.QueryHandler(ml, fakeLogRepo{}))

	req := httptest.NewRequest("POST", "/query", strings.NewReader(`{"query":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var qResp api.QueryResponse
	if err := json.Unmarshal(body, &qResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if qResp.Answer != "ok" {
		t.Errorf("expected answer %q, got %q", "ok", qResp.Answer)
	}
	if gotRequestID != testRequestID {
		t.Errorf("ML service did not receive request id on success path: got %q", gotRequestID)
	}
}

func TestQueryHandler_NoChunksReturns200WithFlag(t *testing.T) {
	mlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"No relevant chunks found"}`))
	}))
	defer mlSrv.Close()

	ml := client.NewMLClient(mlSrv.URL)

	app := fiber.New(fiber.Config{ErrorHandler: api.ErrorHandler})
	app.Use(fixedRequestIDMiddleware())
	app.Post("/query", api.QueryHandler(ml, fakeLogRepo{}))

	req := httptest.NewRequest("POST", "/query", strings.NewReader(`{"query":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var qResp api.QueryResponse
	if err := json.Unmarshal(body, &qResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !qResp.InsufficientContext {
		t.Errorf("expected insufficient_context true, body: %s", body)
	}
	if qResp.Answer != "No relevant chunks found." {
		t.Errorf("expected answer %q, got %q", "No relevant chunks found.", qResp.Answer)
	}
	if len(qResp.Sources) != 0 {
		t.Errorf("expected no sources, got %d", len(qResp.Sources))
	}
}
