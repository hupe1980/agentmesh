package middleware

import (
	"context"
	"iter"
	"sync/atomic"

	"github.com/hupe1980/agentmesh/pkg/model"
)

// TokenCounterMiddleware tracks token usage across model calls.
type TokenCounterMiddleware struct {
	inputTokens  atomic.Int64
	outputTokens atomic.Int64
	totalTokens  atomic.Int64
	callCount    atomic.Int64
}

// NewTokenCounterMiddleware creates a new token counter middleware.
func NewTokenCounterMiddleware() *TokenCounterMiddleware {
	return &TokenCounterMiddleware{}
}

// Wrap wraps the model executor with token counting.
func (m *TokenCounterMiddleware) Wrap(next model.Executor) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			m.callCount.Add(1)

			// Pass through and track token usage
			for resp, err := range next.Generate(ctx, req) {
				if err != nil {
					if !yield(nil, err) {
						return
					}
					continue
				}

				// Track token usage from response
				if resp != nil && resp.Usage != nil {
					m.inputTokens.Add(int64(resp.Usage.PromptTokens))
					m.outputTokens.Add(int64(resp.Usage.CompletionTokens))
					m.totalTokens.Add(int64(resp.Usage.TotalTokens))
				}

				if !yield(resp, nil) {
					return
				}
			}
		}
	})
}

// InputTokens returns the total input tokens used.
func (m *TokenCounterMiddleware) InputTokens() int64 {
	return m.inputTokens.Load()
}

// OutputTokens returns the total output tokens used.
func (m *TokenCounterMiddleware) OutputTokens() int64 {
	return m.outputTokens.Load()
}

// TotalTokens returns the total tokens used.
func (m *TokenCounterMiddleware) TotalTokens() int64 {
	return m.totalTokens.Load()
}

// CallCount returns the number of model calls made.
func (m *TokenCounterMiddleware) CallCount() int64 {
	return m.callCount.Load()
}

// Reset resets all counters to zero.
func (m *TokenCounterMiddleware) Reset() {
	m.inputTokens.Store(0)
	m.outputTokens.Store(0)
	m.totalTokens.Store(0)
	m.callCount.Store(0)
}

// Stats returns a snapshot of current statistics.
func (m *TokenCounterMiddleware) Stats() TokenStats {
	return TokenStats{
		InputTokens:  m.InputTokens(),
		OutputTokens: m.OutputTokens(),
		TotalTokens:  m.TotalTokens(),
		CallCount:    m.CallCount(),
	}
}

// TokenStats represents token usage statistics.
type TokenStats struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CallCount    int64
}
