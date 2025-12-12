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

// countingExecutor creates an executor that counts how many times it's called.
func countingExecutor(response string, callCount *int) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			*callCount++
			yield(&model.Response{
				Message: message.NewAIMessageFromText(response),
				Partial: false,
			}, nil)
		}
	})
}

// streamingExecutor creates an executor that yields multiple partial responses.
func streamingExecutor(chunks []string, callCount *int) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			*callCount++
			for i, chunk := range chunks {
				partial := i < len(chunks)-1
				if !yield(&model.Response{
					Message: message.NewAIMessageFromText(chunk),
					Partial: partial,
				}, nil) {
					return
				}
			}
		}
	})
}

func TestNewCacheMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates cache middleware", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		require.NotNil(t, mw)
		assert.Equal(t, 0, mw.Size())
	})
}

func TestCacheMiddleware_Wrap(t *testing.T) {
	t.Parallel()

	t.Run("caches response on first call", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingExecutor("cached response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		responses, errs := collect(context.Background(), exec, req)
		require.Empty(t, errs)
		require.Len(t, responses, 1)
		assert.Equal(t, "cached response", responses[0].Message.String())
		assert.Equal(t, 1, callCount)
		assert.Equal(t, 1, mw.Size())
	})

	t.Run("returns cached response on subsequent calls", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingExecutor("cached response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		// First call
		responses1, errs1 := collect(context.Background(), exec, req)
		require.Empty(t, errs1)
		require.Len(t, responses1, 1)
		assert.Equal(t, 1, callCount)

		// Second call should hit cache
		responses2, errs2 := collect(context.Background(), exec, req)
		require.Empty(t, errs2)
		require.Len(t, responses2, 1)
		assert.Equal(t, "cached response", responses2[0].Message.String())
		assert.Equal(t, 1, callCount) // Still 1, not 2
	})

	t.Run("different requests have different cache keys", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		// Use Instructions field which serializes reliably to differentiate requests
		req1 := &model.Request{
			Instructions: "instruction-1",
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}
		req2 := &model.Request{
			Instructions: "instruction-2",
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req1)
		collect(context.Background(), exec, req2)

		assert.Equal(t, 2, callCount)
		assert.Equal(t, 2, mw.Size())
	})

	t.Run("caches final non-partial response from streaming", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(streamingExecutor([]string{"Hello", " ", "World"}, &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("stream"),
			},
		}

		// First call - streaming
		responses1, errs1 := collect(context.Background(), exec, req)
		require.Empty(t, errs1)
		require.Len(t, responses1, 3)
		assert.Equal(t, 1, callCount)
		assert.Equal(t, 1, mw.Size())

		// Second call should return cached final response
		responses2, errs2 := collect(context.Background(), exec, req)
		require.Empty(t, errs2)
		require.Len(t, responses2, 1)
		assert.Equal(t, "World", responses2[0].Message.String())
		assert.Equal(t, 1, callCount) // Still 1
	})
}

func TestCacheMiddleware_Clear(t *testing.T) {
	t.Parallel()

	t.Run("clears all cached entries", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText("hello"),
			},
		}

		collect(context.Background(), exec, req)
		assert.Equal(t, 1, mw.Size())

		mw.Clear()
		assert.Equal(t, 0, mw.Size())

		// After clear, should call executor again
		collect(context.Background(), exec, req)
		assert.Equal(t, 2, callCount)
	})
}

func TestCacheMiddleware_Size(t *testing.T) {
	t.Parallel()

	t.Run("returns correct cache size", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingExecutor("response", &callCount))

		assert.Equal(t, 0, mw.Size())

		// Use different Instructions to create unique cache keys
		for i := 0; i < 5; i++ {
			req := &model.Request{
				Instructions: "instruction-" + string(rune('0'+i)),
				Messages: []message.Message{
					message.NewHumanMessageFromText("hello"),
				},
			}
			collect(context.Background(), exec, req)
		}

		assert.Equal(t, 5, mw.Size())
	})
}
