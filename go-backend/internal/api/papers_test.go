package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/malikimayzar/arxiv-research-assistant/internal/api"
	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
	"github.com/malikimayzar/arxiv-research-assistant/internal/logging"
	"github.com/malikimayzar/arxiv-research-assistant/internal/repository"
)

const testPaperID = "6f1a1234-1111-2222-3333-444455556666"

type recordingLog struct {
	mu     sync.Mutex
	events []string
}

func (r *recordingLog) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordingLog) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type fakePaperRepo struct {
	paper  *repository.Paper
	getErr error
	log    *recordingLog
}

func (f *fakePaperRepo) List(_ context.Context, _, _ int) ([]repository.Paper, int, error) {
	return []repository.Paper{}, 0, nil
}

func (f *fakePaperRepo) GetByID(_ context.Context, _ uuid.UUID) (*repository.Paper, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.paper, nil
}

func (f *fakePaperRepo) GetByArxivID(_ context.Context, _ string) (*repository.Paper, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.paper, nil
}

func (f *fakePaperRepo) Delete(_ context.Context, _ uuid.UUID) error {
	if f.log != nil {
		f.log.add("pg-delete")
	}
	return nil
}

func newFakePaper() *repository.Paper {
	id, _ := uuid.Parse(testPaperID)
	return &repository.Paper{ID: id, ArxivID: "2312.10997", Title: "test paper"}
}

func newDeleteApp(repo repository.PaperRepository, ml *client.MLClient, logger *logging.Logger) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: api.ErrorHandler})
	app.Delete("/papers/:id", api.NewPapersHandler(repo, ml, logger).Delete)
	return app
}

func newSourceApp(repo repository.PaperRepository, ml *client.MLClient, logger *logging.Logger) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: api.ErrorHandler})
	app.Get("/papers/source/:arxiv_id", api.NewPapersHandler(repo, ml, logger).Source)
	return app
}

func TestPaperSource_ReturnsMetadata(t *testing.T) {
	published := time.Date(2017, 6, 12, 0, 0, 0, 0, time.UTC)
	id, _ := uuid.Parse(testPaperID)
	repo := &fakePaperRepo{
		paper: &repository.Paper{
			ID:        id,
			ArxivID:   "1706.03762",
			Title:     "Attention Is All You Need",
			Authors:   []string{"Ashish Vaswani", "Noam Shazeer"},
			Published: &published,
		},
	}
	logger := logging.NewLogger("test", logging.INFO)
	app := newSourceApp(repo, client.NewMLClient("http://127.0.0.1:1", logger), logger)

	for _, tc := range []struct {
		name string
		path string
	}{
		{"raw id", "/papers/source/1706.03762"},
		{"arxiv prefix", "/papers/source/arxiv:1706.03762"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}

			var body struct {
				SourceID string   `json:"source_id"`
				Title    string   `json:"title"`
				Authors  []string `json:"authors"`
				Year     int      `json:"year"`
				URL      string   `json:"url"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			if body.SourceID != "arxiv:1706.03762" {
				t.Errorf("source_id = %q, want arxiv:1706.03762", body.SourceID)
			}
			if body.Title != "Attention Is All You Need" {
				t.Errorf("title = %q, want Attention Is All You Need", body.Title)
			}
			if len(body.Authors) != 2 || body.Authors[0] != "Ashish Vaswani" || body.Authors[1] != "Noam Shazeer" {
				t.Errorf("authors = %v, want [Ashish Vaswani Noam Shazeer]", body.Authors)
			}
			if body.Year != 2017 {
				t.Errorf("year = %d, want 2017", body.Year)
			}
			if body.URL != "https://arxiv.org/abs/1706.03762" {
				t.Errorf("url = %q, want https://arxiv.org/abs/1706.03762", body.URL)
			}
		})
	}
}

func TestPaperSource_NotFound(t *testing.T) {
	repo := &fakePaperRepo{getErr: repository.ErrNotFound}
	logger := logging.NewLogger("test", logging.INFO)
	app := newSourceApp(repo, client.NewMLClient("http://127.0.0.1:1", logger), logger)

	resp, err := app.Test(httptest.NewRequest("GET", "/papers/source/arxiv:9999.99999", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeletePaper_PurgesVectorsThenDeletesRow(t *testing.T) {
	log := &recordingLog{}
	var gotMethod, gotPath string
	mlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		log.add("ml-purge")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"deleted":true}`))
	}))
	defer mlSrv.Close()

	logger := logging.NewLogger("test", logging.INFO)
	app := newDeleteApp(&fakePaperRepo{paper: newFakePaper(), log: log}, client.NewMLClient(mlSrv.URL, logger), logger)

	resp, err := app.Test(httptest.NewRequest("DELETE", "/papers/"+testPaperID, nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if gotMethod != http.MethodDelete || gotPath != "/papers/2312.10997" {
		t.Errorf("ML call = %s %s, want DELETE /papers/2312.10997", gotMethod, gotPath)
	}
	if events := log.snapshot(); len(events) != 2 || events[0] != "ml-purge" || events[1] != "pg-delete" {
		t.Errorf("expected ML purge before PG delete, got %v", events)
	}
}

func TestDeletePaper_PurgeFailureAborts(t *testing.T) {
	log := &recordingLog{}
	mlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer mlSrv.Close()

	logger := logging.NewLogger("test", logging.INFO)
	app := newDeleteApp(&fakePaperRepo{paper: newFakePaper(), log: log}, client.NewMLClient(mlSrv.URL, logger), logger)

	resp, err := app.Test(httptest.NewRequest("DELETE", "/papers/"+testPaperID, nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}
	if events := log.snapshot(); len(events) != 0 {
		t.Errorf("PG delete must not run after failed purge, got %v", events)
	}
}

func TestDeletePaper_NotFoundSkipsPurge(t *testing.T) {
	var mlCalls int
	mlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mlCalls++
	}))
	defer mlSrv.Close()

	logger := logging.NewLogger("test", logging.INFO)
	app := newDeleteApp(&fakePaperRepo{getErr: errors.New("no rows")}, client.NewMLClient(mlSrv.URL, logger), logger)

	resp, err := app.Test(httptest.NewRequest("DELETE", "/papers/"+testPaperID, nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if mlCalls != 0 {
		t.Errorf("ML service must not be called, got %d calls", mlCalls)
	}
}

func TestDeletePaper_InvalidID(t *testing.T) {
	logger := logging.NewLogger("test", logging.INFO)
	app := newDeleteApp(&fakePaperRepo{paper: newFakePaper()}, client.NewMLClient("http://127.0.0.1:1", logger), logger)

	resp, err := app.Test(httptest.NewRequest("DELETE", "/papers/not-a-uuid", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
