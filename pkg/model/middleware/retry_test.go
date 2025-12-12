package middleware

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// failingExecutor creates an executor that fails a number of times before succeeding.
func failingExecutor(failCount int, successResponse string, attempts *int) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			*attempts++
			if *attempts <= failCount {
				yield(nil, errors.New("temporary error"))
				return
			}
			yield(&model.Response{
				Message: message.NewAIMessageFromText(successResponse),
			}, nil)
		}
	})
}

// alwaysFailingExecutor creates an executor that always fails.
func alwaysFailingExecutor(err error, attempts *int) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			*attempts++
			yield(nil, err)
		}
	})
}

func TestNewRetryMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates retry middleware with defaults", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware()
		require.NotNil(t, mw)
		assert.Equal(t, 3, mw.maxRetries)
		assert.Equal(t, 100*time.Millisecond, mw.initialBackoff)
		assert.Equal(t, 10*time.Second, mw.maxBackoff)
		assert.Equal(t, 2.0, mw.multiplier)
	})

	t.Run("applies options", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(
			WithMaxRetries(5),
			WithInitialBackoff(50*time.Millisecond),
			WithMaxBackoff(5*time.Second),
			WithBackoffMultiplier(1.5),
		)

		assert.Equal(t, 5, mw.maxRetries)
		assert.Equal(t, 50*time.Millisecond, mw.initialBackoff)
		assert.Equal(t, 5*time.Second, mw.maxBackoff)
		assert.Equal(t, 1.5, mw.multiplier)
	})
}

func TestRetryMiddleware_Wrap(t *testing.T) {
	t.Parallel()

	t.Run("passes through on success", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware()
		var callCount int
		exec := mw.Wrap(countingExecutor("success", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		responses, errs := collect(context.Background(), exec, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, "success", responses[0].Message.String())
		assert.Equal(t, 1, callCount)
	})

	t.Run("retries on failure and succeeds", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(
			WithMaxRetries(3),
			WithInitialBackoff(1*time.Millisecond),
		)
		var attempts int
		exec := mw.Wrap(failingExecutor(2, "eventually succeeded", &attempts))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		responses, errs := collect(context.Background(), exec, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, "eventually succeeded", responses[0].Message.String())
		assert.Equal(t, 3, attempts) // 2 failures + 1 success
	})

	t.Run("returns error after max retries exhausted", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(
			WithMaxRetries(2),
			WithInitialBackoff(1*time.Millisecond),
		)
		var attempts int
		exec := mw.Wrap(alwaysFailingExecutor(errors.New("persistent error"), &attempts))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		_, errs := collect(context.Background(), exec, req)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "max retries exceeded")
		assert.Contains(t, errs[0].Error(), "persistent error")
		assert.Equal(t, 3, attempts) // 1 initial + 2 retries
	})

	t.Run("respects context cancellation during backoff", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(
			WithMaxRetries(5),
			WithInitialBackoff(1*time.Second), // Long backoff
		)
		var attempts int
		exec := mw.Wrap(alwaysFailingExecutor(errors.New("error"), &attempts))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, errs := collect(ctx, exec, req)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "retry cancelled")
		assert.Equal(t, 1, attempts) // Only initial attempt
	})

	t.Run("uses exponential backoff", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(
			WithMaxRetries(3),
			WithInitialBackoff(10*time.Millisecond),
			WithBackoffMultiplier(2.0),
		)
		var attempts int
		exec := mw.Wrap(failingExecutor(2, "success", &attempts))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		start := time.Now()
		responses, errs := collect(context.Background(), exec, req)
		elapsed := time.Since(start)

		require.Empty(t, errs)
		require.Len(t, responses, 1)
		// Should take at least 10ms (first backoff) + 20ms (second backoff) = 30ms
		assert.GreaterOrEqual(t, elapsed, 25*time.Millisecond)
	})

	t.Run("caps backoff at maxBackoff", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(
			WithMaxRetries(3),
			WithInitialBackoff(50*time.Millisecond),
			WithMaxBackoff(60*time.Millisecond),
			WithBackoffMultiplier(10.0), // Would be 500ms without cap
		)
		var attempts int
		exec := mw.Wrap(failingExecutor(2, "success", &attempts))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		start := time.Now()
		collect(context.Background(), exec, req)
		elapsed := time.Since(start)

		// With cap, should be around 50ms + 60ms = 110ms, not 50ms + 500ms
		assert.Less(t, elapsed, 200*time.Millisecond)
	})
}

func TestWithMaxRetries(t *testing.T) {
	t.Parallel()

	t.Run("sets max retries", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(WithMaxRetries(10))
		assert.Equal(t, 10, mw.maxRetries)
	})
}

func TestWithInitialBackoff(t *testing.T) {
	t.Parallel()

	t.Run("sets initial backoff", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(WithInitialBackoff(500 * time.Millisecond))
		assert.Equal(t, 500*time.Millisecond, mw.initialBackoff)
	})
}

func TestWithMaxBackoff(t *testing.T) {
	t.Parallel()

	t.Run("sets max backoff", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(WithMaxBackoff(30 * time.Second))
		assert.Equal(t, 30*time.Second, mw.maxBackoff)
	})
}

func TestWithBackoffMultiplier(t *testing.T) {
	t.Parallel()

	t.Run("sets backoff multiplier", func(t *testing.T) {
		t.Parallel()

		mw := NewRetryMiddleware(WithBackoffMultiplier(3.0))
		assert.Equal(t, 3.0, mw.multiplier)
	})
}
