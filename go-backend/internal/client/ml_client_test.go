package client_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/malikimayzar/arxiv-research-assistant/internal/client"
)

// 404 "No relevant chunks found" adalah jawaban valid — 6x beruntun tidak
// boleh membuka breaker dan tiap panggilan harus mengembalikan StatusError.
func TestQueryBreakerIgnores4xx(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"No relevant chunks found"}`))
	}))
	defer srv.Close()

	ml := client.NewMLClient(srv.URL)

	for i := 0; i < 6; i++ {
		_, err := ml.Query("q", 5, "m", "", "req")
		var statusErr *client.StatusError
		if !errors.As(err, &statusErr) || statusErr.StatusCode != http.StatusNotFound {
			t.Fatalf("call %d: want StatusError 404, got %v", i+1, err)
		}
	}

	if state := ml.CircuitBreakerStates()["query"]; state != "closed" {
		t.Errorf("breaker must stay closed on 4xx, got %q", state)
	}
	if requests != 6 {
		t.Errorf("all 6 requests must reach the service, got %d", requests)
	}
}

func TestQueryBreakerOpensOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ml := client.NewMLClient(srv.URL)

	for i := 0; i < 5; i++ {
		if _, err := ml.Query("q", 5, "m", "", "req"); err == nil {
			t.Fatalf("call %d: expected error", i+1)
		}
	}

	if state := ml.CircuitBreakerStates()["query"]; state != "open" {
		t.Fatalf("breaker must open after 5 consecutive 5xx, got %q", state)
	}

	_, err := ml.Query("q", 5, "m", "", "req")
	if err == nil || !strings.Contains(err.Error(), "circuit open") {
		t.Fatalf("expected circuit-open error after tripping, got %v", err)
	}
}

func TestQueryBreakerOpensOnTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // server mati → transport error

	ml := client.NewMLClient(srv.URL)

	for i := 0; i < 5; i++ {
		_, _ = ml.Query("q", 5, "m", "", "req")
	}

	if state := ml.CircuitBreakerStates()["query"]; state != "open" {
		t.Fatalf("breaker must open after 5 transport errors, got %q", state)
	}
}
