package middleware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

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

	t.Run("caches successful result", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingToolExecutor("result", &callCount))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{"input": "test"}`},
		}

		// First call
		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "result", results[0].Result)
		assert.Equal(t, 1, callCount)
		assert.Equal(t, 1, mw.Size())

		// Second call - should hit cache
		results2, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results2, 1)
		assert.Equal(t, "result", results2[0].Result)
		assert.Equal(t, 1, callCount) // Still 1, not 2
	})

	t.Run("different calls have different cache keys", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingToolExecutor("result", &callCount))

		calls1 := []tool.Call{
			{ID: "1", Name: "tool_a", Arguments: `{"input": "test1"}`},
		}
		calls2 := []tool.Call{
			{ID: "2", Name: "tool_b", Arguments: `{"input": "test2"}`},
		}

		exec.Execute(context.Background(), calls1)
		exec.Execute(context.Background(), calls2)

		assert.Equal(t, 2, callCount)
		assert.Equal(t, 2, mw.Size())
	})

	t.Run("does not cache error results", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(alternatingToolExecutor(&callCount))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{"input": "test"}`},
		}

		// First call returns error
		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Error(t, results[0].Error)
		assert.Equal(t, 1, callCount)
		assert.Equal(t, 0, mw.Size()) // Not cached

		// Second call should execute again
		results2, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		assert.Nil(t, results2[0].Error)
		assert.Equal(t, 2, callCount)
		assert.Equal(t, 1, mw.Size()) // Now cached
	})

	t.Run("handles mixed cached and uncached calls", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingToolExecutor("result", &callCount))

		// Cache first call
		calls1 := []tool.Call{
			{ID: "1", Name: "tool_a", Arguments: `{"input": "cached"}`},
		}
		exec.Execute(context.Background(), calls1)
		assert.Equal(t, 1, callCount)

		// Now make batch with cached and uncached
		calls2 := []tool.Call{
			{ID: "1", Name: "tool_a", Arguments: `{"input": "cached"}`},   // cached
			{ID: "2", Name: "tool_b", Arguments: `{"input": "uncached"}`}, // not cached
		}

		results, err := exec.Execute(context.Background(), calls2)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, 2, callCount) // Only one new call
	})

	t.Run("handles executor error", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		exec := mw.Wrap(erroringToolExecutor(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		_, err := exec.Execute(context.Background(), calls)
		require.Error(t, err)
		assert.Equal(t, 0, mw.Size())
	})
}

func TestCacheMiddleware_Clear(t *testing.T) {
	t.Parallel()

	t.Run("clears all cached entries", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingToolExecutor("result", &callCount))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{"input": "test"}`},
		}

		exec.Execute(context.Background(), calls)
		assert.Equal(t, 1, mw.Size())

		mw.Clear()
		assert.Equal(t, 0, mw.Size())

		// After clear, should call executor again
		exec.Execute(context.Background(), calls)
		assert.Equal(t, 2, callCount)
	})
}

func TestCacheMiddleware_Size(t *testing.T) {
	t.Parallel()

	t.Run("returns correct cache size", func(t *testing.T) {
		t.Parallel()

		mw := NewCacheMiddleware()
		var callCount int
		exec := mw.Wrap(countingToolExecutor("result", &callCount))

		assert.Equal(t, 0, mw.Size())

		for i := 0; i < 5; i++ {
			calls := []tool.Call{
				{ID: string(rune('0' + i)), Name: "tool_" + string(rune('a'+i)), Arguments: `{}`},
			}
			exec.Execute(context.Background(), calls)
		}

		assert.Equal(t, 5, mw.Size())
	})
}

// countingToolExecutor creates a tool executor that counts calls.
func countingToolExecutor(result any, callCount *int) tool.Executor {
	return tool.WrapFunc(func(_ context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		*callCount++
		results := make([]tool.ExecutionResult, len(calls))
		for i, call := range calls {
			results[i] = tool.ExecutionResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Result:     result,
			}
		}
		return results, nil
	})
}

// alternatingToolExecutor returns error on odd calls, success on even calls.
func alternatingToolExecutor(callCount *int) tool.Executor {
	return tool.WrapFunc(func(_ context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		*callCount++
		results := make([]tool.ExecutionResult, len(calls))
		for i, call := range calls {
			if *callCount%2 == 1 {
				results[i] = tool.ExecutionResult{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Error:      assert.AnError,
				}
			} else {
				results[i] = tool.ExecutionResult{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result:     "success",
				}
			}
		}
		return results, nil
	})
}
