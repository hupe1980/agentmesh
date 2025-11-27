package middleware

import (
	"context"
	"fmt"
	"iter"
	"time"

	"github.com/hupe1980/agentmesh/pkg/model"
)

// RetryMiddleware retries failed model calls with exponential backoff.
type RetryMiddleware struct {
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	multiplier     float64
}

// RetryOption configures retry behavior.
type RetryOption func(*RetryMiddleware)

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(maxRetries int) RetryOption {
	return func(m *RetryMiddleware) {
		m.maxRetries = maxRetries
	}
}

// WithInitialBackoff sets the initial backoff duration.
func WithInitialBackoff(d time.Duration) RetryOption {
	return func(m *RetryMiddleware) {
		m.initialBackoff = d
	}
}

// WithMaxBackoff sets the maximum backoff duration.
func WithMaxBackoff(d time.Duration) RetryOption {
	return func(m *RetryMiddleware) {
		m.maxBackoff = d
	}
}

// WithBackoffMultiplier sets the backoff multiplier.
func WithBackoffMultiplier(mult float64) RetryOption {
	return func(m *RetryMiddleware) {
		m.multiplier = mult
	}
}

// NewRetryMiddleware creates a new retry middleware with exponential backoff.
func NewRetryMiddleware(opts ...RetryOption) *RetryMiddleware {
	m := &RetryMiddleware{
		maxRetries:     3,
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     10 * time.Second,
		multiplier:     2.0,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Wrap wraps the model executor with retry logic.
func (m *RetryMiddleware) Wrap(next model.Executor) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			var lastErr error
			backoff := m.initialBackoff

			for attempt := 0; attempt <= m.maxRetries; attempt++ {
				if attempt > 0 {
					select {
					case <-ctx.Done():
						yield(nil, fmt.Errorf("retry cancelled: %w", ctx.Err()))
						return
					case <-time.After(backoff):
						// Calculate next backoff
						backoff = time.Duration(float64(backoff) * m.multiplier)
						if backoff > m.maxBackoff {
							backoff = m.maxBackoff
						}
					}
				}

				// Try to execute
				success := false
				for resp, err := range next.Generate(ctx, req) {
					if err != nil {
						lastErr = err
						break // Try next attempt
					}
					success = true
					if !yield(resp, nil) {
						return
					}
				}

				if success {
					return // Success, no retry needed
				}
			}

			// All retries exhausted
			yield(nil, fmt.Errorf("max retries exceeded (%d): %w", m.maxRetries, lastErr))
		}
	})
}
