package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

func TestNewCircuitBreakerMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates circuit breaker in closed state", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(3, 5*time.Second)
		require.NotNil(t, mw)
		assert.Equal(t, StateClosed, mw.State())
	})
}

func TestCircuitBreakerMiddleware_Wrap(t *testing.T) {
	t.Parallel()

	t.Run("allows requests when closed", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(3, 5*time.Second)
		exec := mw.Wrap(mockToolExecutor("success"))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "success", results[0].Result)
		assert.Equal(t, StateClosed, mw.State())
	})

	t.Run("opens after max failures", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(3, 5*time.Second)
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		// 3 failures should open the circuit
		for i := 0; i < 3; i++ {
			exec.Execute(context.Background(), calls)
		}

		assert.Equal(t, StateOpen, mw.State())
	})

	t.Run("rejects requests when open", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(1, 1*time.Hour) // Long timeout
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		// Trigger open state
		exec.Execute(context.Background(), calls)
		assert.Equal(t, StateOpen, mw.State())

		// Next request should be rejected without calling executor
		exec2 := mw.Wrap(mockToolExecutor("success"))
		results, err := exec2.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.ErrorIs(t, results[0].Error, ErrCircuitBreakerOpen)
	})

	t.Run("transitions to half-open after timeout", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(1, 10*time.Millisecond)
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		// Trigger open state
		exec.Execute(context.Background(), calls)
		assert.Equal(t, StateOpen, mw.State())

		// Wait for reset timeout
		time.Sleep(20 * time.Millisecond)

		// Next request should allow through and transition to half-open
		exec2 := mw.Wrap(mockToolExecutor("success"))
		results, err := exec2.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Nil(t, results[0].Error)
		assert.Equal(t, StateClosed, mw.State()) // Success closes it
	})

	t.Run("closes on success in half-open state", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(1, 10*time.Millisecond)
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		// Trigger open state
		exec.Execute(context.Background(), calls)
		time.Sleep(15 * time.Millisecond)

		// Success should close the circuit
		exec2 := mw.Wrap(mockToolExecutor("success"))
		exec2.Execute(context.Background(), calls)
		assert.Equal(t, StateClosed, mw.State())
	})

	t.Run("handles executor error", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(3, 5*time.Second)
		exec := mw.Wrap(erroringToolExecutor(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		_, err := exec.Execute(context.Background(), calls)
		require.Error(t, err)
		// Executor error should record failure
	})

	t.Run("handles multiple calls in batch", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(1, 1*time.Hour)
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		// Trigger open state
		exec.Execute(context.Background(), []tool.Call{{ID: "1", Name: "test", Arguments: `{}`}})

		// Try multiple calls when open
		calls := []tool.Call{
			{ID: "1", Name: "tool_a", Arguments: `{}`},
			{ID: "2", Name: "tool_b", Arguments: `{}`},
		}

		results, err := exec.Execute(context.Background(), calls)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.ErrorIs(t, results[0].Error, ErrCircuitBreakerOpen)
		assert.ErrorIs(t, results[1].Error, ErrCircuitBreakerOpen)
	})
}

func TestCircuitBreakerMiddleware_State(t *testing.T) {
	t.Parallel()

	t.Run("returns current state", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(3, 5*time.Second)
		assert.Equal(t, StateClosed, mw.State())
	})
}

func TestCircuitBreakerMiddleware_Reset(t *testing.T) {
	t.Parallel()

	t.Run("resets to closed state", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(1, 1*time.Hour)
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		// Trigger open state
		exec.Execute(context.Background(), calls)
		assert.Equal(t, StateOpen, mw.State())

		// Reset should close
		mw.Reset()
		assert.Equal(t, StateClosed, mw.State())
	})

	t.Run("clears failure count", func(t *testing.T) {
		t.Parallel()

		mw := NewCircuitBreakerMiddleware(3, 5*time.Second)
		exec := mw.Wrap(toolExecutorWithResultError(assert.AnError))

		calls := []tool.Call{
			{ID: "1", Name: "test_tool", Arguments: `{}`},
		}

		// Record 2 failures (not enough to open)
		exec.Execute(context.Background(), calls)
		exec.Execute(context.Background(), calls)
		assert.Equal(t, StateClosed, mw.State())

		// Reset
		mw.Reset()

		// Now 3 more failures needed to open
		exec.Execute(context.Background(), calls)
		exec.Execute(context.Background(), calls)
		assert.Equal(t, StateClosed, mw.State())

		exec.Execute(context.Background(), calls)
		assert.Equal(t, StateOpen, mw.State())
	})
}

func TestCircuitState_String(t *testing.T) {
	t.Parallel()

	t.Run("closed state", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "closed", StateClosed.String())
	})

	t.Run("open state", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "open", StateOpen.String())
	})

	t.Run("half-open state", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "half-open", StateHalfOpen.String())
	})

	t.Run("unknown state", func(t *testing.T) {
		t.Parallel()
		unknown := CircuitState(99)
		assert.Contains(t, unknown.String(), "unknown")
	})
}
