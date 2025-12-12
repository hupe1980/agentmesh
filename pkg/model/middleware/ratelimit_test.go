package middleware

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

func TestNewRateLimitMiddleware(t *testing.T) {
	t.Run("creates rate limit middleware with correct initial state", func(t *testing.T) {
		mw := NewRateLimitMiddleware(5, 100*time.Millisecond)
		defer mw.Close()

		require.NotNil(t, mw)
		assert.Equal(t, 5, mw.Available())
	})
}

func TestRateLimitMiddleware_Wrap(t *testing.T) {
	t.Run("allows requests when tokens available", func(t *testing.T) {
		mw := NewRateLimitMiddleware(3, 100*time.Millisecond)
		defer mw.Close()

		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		responses, errs := collect(context.Background(), exec, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, 1, callCount)
		assert.Equal(t, 2, mw.Available()) // Started with 3, used 1
	})

	t.Run("consumes token for each request", func(t *testing.T) {
		mw := NewRateLimitMiddleware(5, 100*time.Millisecond)
		defer mw.Close()

		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		for i := 0; i < 3; i++ {
			collect(context.Background(), exec, req)
		}

		assert.Equal(t, 3, callCount)
		assert.Equal(t, 2, mw.Available()) // Started with 5, used 3
	})

	t.Run("waits when no tokens available", func(t *testing.T) {
		mw := NewRateLimitMiddleware(1, 50*time.Millisecond)
		defer mw.Close()

		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		// First request consumes the only token
		collect(context.Background(), exec, req)
		assert.Equal(t, 0, mw.Available())

		// Second request should wait for token refill
		start := time.Now()
		collect(context.Background(), exec, req)
		elapsed := time.Since(start)

		assert.Equal(t, 2, callCount)
		assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond) // Allow some tolerance
	})

	t.Run("respects context cancellation while waiting", func(t *testing.T) {
		mw := NewRateLimitMiddleware(1, 1*time.Second) // Slow refill
		defer mw.Close()

		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		// Consume the only token
		collect(context.Background(), exec, req)

		// Try with a timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, errs := collect(ctx, exec, req)
		require.Len(t, errs, 1)
		assert.ErrorIs(t, errs[0], context.DeadlineExceeded)
	})
}

func TestRateLimitMiddleware_Available(t *testing.T) {
	t.Run("returns current token count", func(t *testing.T) {
		mw := NewRateLimitMiddleware(10, 100*time.Millisecond)
		defer mw.Close()

		assert.Equal(t, 10, mw.Available())
	})
}

func TestRateLimitMiddleware_Close(t *testing.T) {
	t.Run("stops refill goroutine", func(t *testing.T) {
		mw := NewRateLimitMiddleware(5, 10*time.Millisecond)

		// Close should not block
		done := make(chan struct{})
		go func() {
			mw.Close()
			close(done)
		}()

		select {
		case <-done:
			// Good, closed successfully
		case <-time.After(1 * time.Second):
			t.Fatal("Close() blocked for too long")
		}
	})
}

func TestRateLimitMiddleware_RefillLoop(t *testing.T) {
	t.Run("refills tokens over time", func(t *testing.T) {
		mw := NewRateLimitMiddleware(2, 30*time.Millisecond)
		defer mw.Close()

		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		// Consume both tokens
		collect(context.Background(), exec, req)
		collect(context.Background(), exec, req)
		assert.Equal(t, 0, mw.Available())

		// Wait for refill
		time.Sleep(80 * time.Millisecond)
		assert.GreaterOrEqual(t, mw.Available(), 1)
	})

	t.Run("does not exceed max tokens", func(t *testing.T) {
		mw := NewRateLimitMiddleware(3, 20*time.Millisecond)
		defer mw.Close()

		// Wait for multiple refill cycles
		time.Sleep(100 * time.Millisecond)

		// Should not exceed max
		assert.LessOrEqual(t, mw.Available(), 3)
	})
}

// erroringExecutor creates an executor that returns an error.
func erroringExecutor(err error) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			yield(nil, err)
		}
	})
}
