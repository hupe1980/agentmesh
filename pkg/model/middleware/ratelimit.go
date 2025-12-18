package middleware

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/model"
)

const (
	// defaultTokenAcquirePollingInterval is the interval at which the rate limiter
	// checks for available tokens. A small interval (10ms) provides good responsiveness
	// while minimizing CPU overhead from busy-waiting.
	defaultTokenAcquirePollingInterval = 10 * time.Millisecond
)

// RateLimitMiddleware limits the rate of model calls using a token bucket algorithm.
type RateLimitMiddleware struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration
	lastRefill time.Time
	mu         sync.Mutex
	stopRefill chan struct{}
	refillDone chan struct{}
}

// NewRateLimitMiddleware creates a new rate limit middleware.
// maxTokens is the bucket capacity, refillRate is how often to add one token.
func NewRateLimitMiddleware(maxTokens int, refillRate time.Duration) *RateLimitMiddleware {
	m := &RateLimitMiddleware{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
		stopRefill: make(chan struct{}),
		refillDone: make(chan struct{}),
	}

	// Start refill goroutine
	go m.refillLoop()

	return m
}

// refillLoop periodically adds tokens to the bucket.
func (m *RateLimitMiddleware) refillLoop() {
	defer close(m.refillDone)
	ticker := time.NewTicker(m.refillRate)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopRefill:
			return
		case <-ticker.C:
			m.mu.Lock()
			if m.tokens < m.maxTokens {
				m.tokens++
			}
			m.lastRefill = time.Now()
			m.mu.Unlock()
		}
	}
}

// Wrap wraps the model executor with rate limiting.
func (m *RateLimitMiddleware) Wrap(next model.Executor) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			// Wait for a token
			if err := m.acquire(ctx); err != nil {
				yield(nil, err)
				return
			}

			// Pass through to next executor
			for resp, err := range next.Generate(ctx, req) {
				if !yield(resp, err) {
					return
				}
			}
		}
	})
}

// acquire waits for a token to become available.
func (m *RateLimitMiddleware) acquire(ctx context.Context) error {
	ticker := time.NewTicker(defaultTokenAcquirePollingInterval)
	defer ticker.Stop()

	for {
		m.mu.Lock()
		if m.tokens > 0 {
			m.tokens--
			m.mu.Unlock()
			return nil
		}
		m.mu.Unlock()

		select {
		case <-ctx.Done():
			return fmt.Errorf("rate limit acquire cancelled: %w", ctx.Err())
		case <-ticker.C:
			// Continue waiting
		}
	}
}

// Close stops the refill goroutine.
func (m *RateLimitMiddleware) Close() {
	close(m.stopRefill)
	<-m.refillDone
}

// Available returns the number of available tokens.
func (m *RateLimitMiddleware) Available() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tokens
}
