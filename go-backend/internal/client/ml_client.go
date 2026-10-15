package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ml service returned status %d", resp.StatusCode)
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

	return result.(*QueryMLResponse), nil
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
