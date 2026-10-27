package client

import (
	"time"

	"github.com/malikimayzar/arxiv-research-assistant/internal/logging"
	"github.com/sony/gobreaker"
)

func newCircuitBreaker(name string, logger *logging.Logger) *gobreaker.CircuitBreaker {
	return gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Info("Circuit breaker state changed", logging.Fields{
				"breaker": name,
				"from":    from.String(),
				"to":      to.String(),
			})
		},
	})
}
