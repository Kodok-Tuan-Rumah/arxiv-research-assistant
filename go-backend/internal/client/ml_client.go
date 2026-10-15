package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sony/gobreaker"
)

type MLClient struct {
	baseURL     string
	httpClient  *http.Client
	queryClient *http.Client
	queryCB     *gobreaker.CircuitBreaker
	embedCB     *gobreaker.CircuitBreaker
	ingestCB    *gobreaker.CircuitBreaker
}

func NewMLClient(baseURL string) *MLClient {
	return &MLClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		queryClient: &http.Client{
			Timeout: 600 * time.Second,
		},
		queryCB:  newCircuitBreaker("ml-query"),
		embedCB:  newCircuitBreaker("ml-embed"),
		ingestCB: newCircuitBreaker("ml-ingest"),
	}
}

type EmbedRequest struct {
	Texts []string `json:"texts"`
}

type EmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Model      string      `json:"model"`
	Count      int         `json:"count"`
}

func (c *MLClient) Embed(texts []string) (*EmbedResponse, error) {
	body, err := json.Marshal(EmbedRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	result, err := c.embedCB.Execute(func() (interface{}, error) {
		resp, err := c.httpClient.Post(
			c.baseURL+"/embed",
			"application/json",
			bytes.NewBuffer(body),
		)
		if err != nil {
			return nil, fmt.Errorf("ml service request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ml service returned status %d", resp.StatusCode)
		}

		var result EmbedResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &result, nil
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return nil, fmt.Errorf("ml service unavailable (circuit open): %w", err)
		}
		return nil, err
	}

	return result.(*EmbedResponse), nil
}

type ChunkRequest struct {
	Text      string `json:"text"`
	ChunkSize int    `json:"chunk_size"`
	Overlap   int    `json:"overlap"`
}

type ChunkResponse struct {
	Chunks []string       `json:"chunks"`
	Stats  map[string]any `json:"stats"`
}

func (c *MLClient) Chunk(text string, chunkSize, overlap int) (*ChunkResponse, error) {
	body, err := json.Marshal(ChunkRequest{
		Text:      text,
		ChunkSize: chunkSize,
		Overlap:   overlap,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		c.baseURL+"/chunk",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, fmt.Errorf("ml service request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ml service returned status %d", resp.StatusCode)
	}

	var result ChunkResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

type QueryRequest struct {
	Query   string `json:"query"`
	TopK    int    `json:"top_k"`
	Model   string `json:"model"`
	ArxivID string `json:"arxiv_id,omitempty"`
}

type Source struct {
	Text    string  `json:"text"`
	Score   float32 `json:"score"`
	ArxivID string  `json:"arxiv_id"`
}

type QueryMLResponse struct {
	Answer       string   `json:"answer"`
	Sources      []Source `json:"sources"`
	RetrievalMs  int      `json:"retrieval_ms"`
	GenerationMs int      `json:"generation_ms"`
	Model        string   `json:"model"`
}

// DeletePaper purges every vector of a paper in the ML service (idempotent).
func (c *MLClient) DeletePaper(arxivID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+"/papers/"+url.PathEscape(arxivID), nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ml service delete failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ml service returned status %d", resp.StatusCode)
	}
	return nil
}

// StatusError represents a 4xx response from the ML service. These are valid
// service answers (the request was understood, the rejection is deliberate),
// so the caller receives them as-is and they never count as breaker failures.
type StatusError struct {
	StatusCode int
	Detail     string
}

func (e *StatusError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("ml service returned status %d: %s", e.StatusCode, e.Detail)
	}
	return fmt.Sprintf("ml service returned status %d", e.StatusCode)
}

// extractDetail pulls FastAPI's `detail` field out of an error body, falling
// back to the trimmed raw body when the shape differs.
func extractDetail(body []byte) string {
	var payload struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Detail != "" {
		return payload.Detail
	}
	return strings.TrimSpace(string(body))
}

func (c *MLClient) Query(query string, topK int, model string, arxivID string, requestID string) (*QueryMLResponse, error) {
	body, err := json.Marshal(QueryRequest{
		Query:   query,
		TopK:    topK,
		Model:   model,
		ArxivID: arxivID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	result, err := c.queryCB.Execute(func() (interface{}, error) {
		req, err := http.NewRequest(http.MethodPost, c.baseURL+"/query", bytes.NewBuffer(body))
		if err != nil {
			return nil, fmt.Errorf("failed to build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if requestID != "" {
			req.Header.Set("X-Request-ID", requestID)
		}

		resp, err := c.queryClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ml service query failed: %w", err)
		}
		defer resp.Body.Close()

		// 5xx = service failure → counts toward the breaker.
		if resp.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("ml service returned status %d", resp.StatusCode)
		}

		// 4xx = deliberate answer (e.g. 404 "No relevant chunks found") → hand
		// it back without tripping the breaker.
		if resp.StatusCode >= http.StatusBadRequest {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return &StatusError{StatusCode: resp.StatusCode, Detail: extractDetail(raw)}, nil
		}

		var result QueryMLResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &result, nil
	})

	if err != nil {
		if err == gobreaker.ErrOpenState {
			return nil, fmt.Errorf("ml service unavailable (circuit open): %w", err)
		}
		return nil, err
	}

	switch v := result.(type) {
	case *QueryMLResponse:
		return v, nil
	case *StatusError:
		return nil, v
	default:
		return nil, fmt.Errorf("unexpected result type %T", result)
	}
}

func (c *MLClient) Ping() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("ml service unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ml service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

// State returns current circuit breaker states — useful for /health endpoint
func (c *MLClient) CircuitBreakerStates() map[string]string {
	stateStr := func(s gobreaker.State) string {
		switch s {
		case gobreaker.StateClosed:
			return "closed"
		case gobreaker.StateHalfOpen:
			return "half-open"
		case gobreaker.StateOpen:
			return "open"
		default:
			return "unknown"
		}
	}
	return map[string]string{
		"query":  stateStr(c.queryCB.State()),
		"embed":  stateStr(c.embedCB.State()),
		"ingest": stateStr(c.ingestCB.State()),
	}
}
