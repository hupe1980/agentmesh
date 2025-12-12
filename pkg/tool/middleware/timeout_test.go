package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

func TestNewTimeoutMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates timeout middleware", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(5 * time.Second)
		require.NotNil(t, mw)
		assert.Equal(t, 5*time.Second, mw.timeout)
	})
}

func TestTimeoutMiddleware_Wrap(t *testing.T) {
	t.Parallel()

	t.Run("passes through when execution is fast", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(1 * time.Second)
		exec := mw.Wrap(mockToolExecutor("success"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "success", results[0].Result)
		assert.Nil(t, results[0].Error)
	})

	t.Run("returns timeout error when execution is slow", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(50 * time.Millisecond)
		exec := mw.Wrap(slowToolExecutor(200*time.Millisecond, "result"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Error(t, results[0].Error)
		assert.Contains(t, results[0].Error.Error(), "timeout")
	})

	t.Run("handles multiple calls timeout", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(50 * time.Millisecond)
		exec := mw.Wrap(slowToolExecutor(200*time.Millisecond, "result"))

		calls := []tool.Call{
			{ID: "1", Name: "tool_a", Arguments: `{}`},
			{ID: "2", Name: "tool_b", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Error(t, results[0].Error)
		assert.Error(t, results[1].Error)
	})

	t.Run("respects parent context cancellation", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(5 * time.Second)
		exec := mw.Wrap(slowToolExecutor(1*time.Second, "result"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel immediately
		cancel()

		_, err := exec.Execute(ctx, calls)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("propagates executor error", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(1 * time.Second)
		exec := mw.Wrap(erroringToolExecutor(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		_, err := exec.Execute(context.Background(), calls)
		require.Error(t, err)
	})

	t.Run("includes timeout duration in error message", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(100 * time.Millisecond)
		exec := mw.Wrap(slowToolExecutor(500*time.Millisecond, "result"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Contains(t, results[0].Error.Error(), "100ms")
	})

	t.Run("completes just before timeout", func(t *testing.T) {
		t.Parallel()

		mw := NewTimeoutMiddleware(200 * time.Millisecond)
		exec := mw.Wrap(slowToolExecutor(50*time.Millisecond, "quick"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Nil(t, results[0].Error)
		assert.Equal(t, "quick", results[0].Result)
	})
}

// slowToolExecutor creates a tool executor with a delay.
func slowToolExecutor(delay time.Duration, result any) tool.Executor {
	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		select {
		case <-time.After(delay):
			results := make([]tool.ExecutionResult, len(calls))
			for i, call := range calls {
				results[i] = tool.ExecutionResult{
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Result:     result,
				}
			}
			return results, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
}
