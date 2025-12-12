package middleware

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// executorWithUsage creates an executor that returns responses with token usage info.
func executorWithUsage(promptTokens, completionTokens, totalTokens int) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			yield(&model.Response{
				Message: message.NewAIMessageFromText("response"),
				Usage: &model.UsageInfo{
					PromptTokens:     promptTokens,
					CompletionTokens: completionTokens,
					TotalTokens:      totalTokens,
				},
			}, nil)
		}
	})
}

// executorWithoutUsage creates an executor that returns responses without usage info.
func executorWithoutUsage() model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			yield(&model.Response{
				Message: message.NewAIMessageFromText("response"),
				Usage:   nil,
			}, nil)
		}
	})
}

func TestNewTokenCounterMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates token counter middleware with zero values", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		require.NotNil(t, mw)
		assert.Equal(t, int64(0), mw.InputTokens())
		assert.Equal(t, int64(0), mw.OutputTokens())
		assert.Equal(t, int64(0), mw.TotalTokens())
		assert.Equal(t, int64(0), mw.CallCount())
	})
}

func TestTokenCounterMiddleware_Wrap(t *testing.T) {
	t.Parallel()

	t.Run("counts tokens from response", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(10, 20, 30))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		responses, errs := collect(context.Background(), exec, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)

		assert.Equal(t, int64(10), mw.InputTokens())
		assert.Equal(t, int64(20), mw.OutputTokens())
		assert.Equal(t, int64(30), mw.TotalTokens())
		assert.Equal(t, int64(1), mw.CallCount())
	})

	t.Run("accumulates tokens across multiple calls", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(10, 20, 30))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		for i := 0; i < 5; i++ {
			collect(context.Background(), exec, req)
		}

		assert.Equal(t, int64(50), mw.InputTokens())
		assert.Equal(t, int64(100), mw.OutputTokens())
		assert.Equal(t, int64(150), mw.TotalTokens())
		assert.Equal(t, int64(5), mw.CallCount())
	})

	t.Run("handles responses without usage info", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithoutUsage())

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		responses, errs := collect(context.Background(), exec, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)

		assert.Equal(t, int64(0), mw.InputTokens())
		assert.Equal(t, int64(0), mw.OutputTokens())
		assert.Equal(t, int64(0), mw.TotalTokens())
		assert.Equal(t, int64(1), mw.CallCount()) // Call is still counted
	})

	t.Run("handles errors from executor", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		var attempts int
		exec := mw.Wrap(alwaysFailingExecutor(assert.AnError, &attempts))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		_, errs := collect(context.Background(), exec, req)
		require.Len(t, errs, 1)

		assert.Equal(t, int64(0), mw.InputTokens())
		assert.Equal(t, int64(0), mw.OutputTokens())
		assert.Equal(t, int64(0), mw.TotalTokens())
		assert.Equal(t, int64(1), mw.CallCount()) // Call is still counted
	})
}

func TestTokenCounterMiddleware_Reset(t *testing.T) {
	t.Parallel()

	t.Run("resets all counters to zero", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(10, 20, 30))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req)
		assert.Equal(t, int64(1), mw.CallCount())

		mw.Reset()

		assert.Equal(t, int64(0), mw.InputTokens())
		assert.Equal(t, int64(0), mw.OutputTokens())
		assert.Equal(t, int64(0), mw.TotalTokens())
		assert.Equal(t, int64(0), mw.CallCount())
	})
}

func TestTokenCounterMiddleware_Stats(t *testing.T) {
	t.Parallel()

	t.Run("returns current statistics snapshot", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(15, 25, 40))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req)
		collect(context.Background(), exec, req)

		stats := mw.Stats()

		assert.Equal(t, int64(30), stats.InputTokens)
		assert.Equal(t, int64(50), stats.OutputTokens)
		assert.Equal(t, int64(80), stats.TotalTokens)
		assert.Equal(t, int64(2), stats.CallCount)
	})

	t.Run("returns independent snapshot", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(10, 20, 30))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req)
		stats := mw.Stats()

		// Make another call
		collect(context.Background(), exec, req)

		// Original stats should be unchanged
		assert.Equal(t, int64(10), stats.InputTokens)
		assert.Equal(t, int64(20), stats.OutputTokens)
		assert.Equal(t, int64(30), stats.TotalTokens)
		assert.Equal(t, int64(1), stats.CallCount)

		// New stats should reflect both calls
		newStats := mw.Stats()
		assert.Equal(t, int64(20), newStats.InputTokens)
		assert.Equal(t, int64(2), newStats.CallCount)
	})
}

func TestTokenCounterMiddleware_Getters(t *testing.T) {
	t.Parallel()

	t.Run("InputTokens returns prompt tokens", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(100, 50, 150))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req)
		assert.Equal(t, int64(100), mw.InputTokens())
	})

	t.Run("OutputTokens returns completion tokens", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(100, 75, 175))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req)
		assert.Equal(t, int64(75), mw.OutputTokens())
	})

	t.Run("TotalTokens returns total tokens", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(100, 75, 200))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req)
		assert.Equal(t, int64(200), mw.TotalTokens())
	})

	t.Run("CallCount returns number of calls", func(t *testing.T) {
		t.Parallel()

		mw := NewTokenCounterMiddleware()
		exec := mw.Wrap(executorWithUsage(10, 20, 30))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		for i := 0; i < 7; i++ {
			collect(context.Background(), exec, req)
		}
		assert.Equal(t, int64(7), mw.CallCount())
	})
}
