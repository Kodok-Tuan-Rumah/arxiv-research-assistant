package client

import (
	"testing"
	"time"

	"github.com/malikimayzar/arxiv-research-assistant/internal/logging"
	"github.com/sony/gobreaker"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	logger := logging.NewLogger("test", logging.INFO)
	// Create breaker with short timeout for testing
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "test-breaker",
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     100 * time.Millisecond, // Short timeout for test
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

	// Initial state should be closed
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("initial state should be closed, got %v", cb.State())
	}

	// Simulate 5 failures to trip the breaker
	for i := 0; i < 5; i++ {
		_, err := cb.Execute(func() (interface{}, error) {
			return nil, gobreaker.ErrOpenState
		})
		if err != gobreaker.ErrOpenState && i < 4 {
			t.Errorf("expected error on attempt %d, got %v", i+1, err)
		}
	}

	// After 5 failures, breaker should be open
	if cb.State() != gobreaker.StateOpen {
		t.Errorf("breaker should be open after 5 failures, got %v", cb.State())
	}

	// Wait for timeout to transition to half-open
	time.Sleep(150 * time.Millisecond)

	// Successful requests in half-open state (MaxRequests = 3)
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(func() (interface{}, error) {
			return "success", nil
		})
		if err != nil {
			t.Errorf("expected success in half-open state, got %v", err)
		}
	}

	// After 3 successful requests, breaker should be closed
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("breaker should be closed after 3 successful requests, got %v", cb.State())
	}
}

func TestCircuitBreaker_FailureThreshold(t *testing.T) {
	logger := logging.NewLogger("test", logging.INFO)
	cb := newCircuitBreaker("test-breaker", logger)

	// 4 failures should not trip the breaker (needs 5 with 60% ratio)
	for i := 0; i < 4; i++ {
		_, _ = cb.Execute(func() (interface{}, error) {
			return nil, gobreaker.ErrOpenState
		})
	}

	if cb.State() != gobreaker.StateClosed {
		t.Errorf("breaker should still be closed after 4 failures, got %v", cb.State())
	}

	// 5th failure should trip it
	_, _ = cb.Execute(func() (interface{}, error) {
		return nil, gobreaker.ErrOpenState
	})

	if cb.State() != gobreaker.StateOpen {
		t.Errorf("breaker should be open after 5 failures, got %v", cb.State())
	}
}

func TestCircuitBreaker_AllowsRequestsWhenClosed(t *testing.T) {
	logger := logging.NewLogger("test", logging.INFO)
	cb := newCircuitBreaker("test-breaker", logger)

	successCount := 0
	for i := 0; i < 10; i++ {
		_, err := cb.Execute(func() (interface{}, error) {
			return "success", nil
		})
		if err == nil {
			successCount++
		}
	}

	if successCount != 10 {
		t.Errorf("expected all 10 requests to succeed, got %d", successCount)
	}
	if cb.State() != gobreaker.StateClosed {
		t.Errorf("breaker should remain closed, got %v", cb.State())
	}
}

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	logger := logging.NewLogger("test", logging.INFO)
	cb := newCircuitBreaker("test-breaker", logger)

	// Trip the breaker
	for i := 0; i < 5; i++ {
		_, _ = cb.Execute(func() (interface{}, error) {
			return nil, gobreaker.ErrOpenState
		})
	}

	if cb.State() != gobreaker.StateOpen {
		t.Errorf("breaker should be open, got %v", cb.State())
	}

	// All requests should be rejected while open
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(func() (interface{}, error) {
			return "should not execute", nil
		})
		if err != gobreaker.ErrOpenState {
			t.Errorf("expected ErrOpenState, got %v", err)
		}
	}
}
