package middleware

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
		},
		[]string{"method", "path"},
	)

	activeGoroutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_goroutines",
			Help: "Number of active goroutines",
		},
	)

	qdrantSearchDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "qdrant_search_duration_seconds",
			Help:    "Qdrant vector search duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
		},
	)

	ollamaGenerationDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "ollama_generation_duration_seconds",
			Help:    "Ollama LLM generation duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300},
		},
	)

	failureModeTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "failure_mode_total",
			Help: "Total queries per failure mode",
		},
		[]string{"mode"},
	)

	postgresConnectionsActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "postgres_connections_active",
			Help: "Active PostgreSQL connections",
		},
	)
)

func Metrics() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Response().StatusCode())
		path := c.Route().Path
		httpRequestsTotal.WithLabelValues(c.Method(), path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Method(), path).Observe(duration)
		return err
	}
}

func SetActiveGoroutines(n float64)              { activeGoroutines.Set(n) }
func ObserveQdrantSearch(seconds float64)        { qdrantSearchDuration.Observe(seconds) }
func ObserveOllamaGeneration(seconds float64)    { ollamaGenerationDuration.Observe(seconds) }
func IncFailureMode(mode string)                 { failureModeTotal.WithLabelValues(mode).Inc() }
func SetPostgresConnections(n float64)           { postgresConnectionsActive.Set(n) }
