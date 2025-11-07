package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 5*time.Second)
	require.NotNil(t, cb)
	assert.Equal(t, StateClosed, cb.State())
	assert.Equal(t, int32(3), cb.failureThreshold)
	assert.Equal(t, int32(2), cb.successThreshold)
	assert.Equal(t, 5*time.Second, cb.timeout)
}

func TestCircuitBreakerState_String(t *testing.T) {
	tests := []struct {
		state    CircuitBreakerState
		expected string
	}{
		{StateClosed, "CLOSED"},
		{StateOpen, "OPEN"},
		{StateHalfOpen, "HALF_OPEN"},
		{CircuitBreakerState(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestCircuitBreaker_SuccessfulCalls(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)
	ctx := context.Background()

	// All successful calls should pass through
	for i := 0; i < 5; i++ {
		err := cb.Call(ctx, func(ctx context.Context) error {
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, StateClosed, cb.State())
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)
	ctx := context.Background()
	testErr := errors.New("test error")

	// First 2 failures - circuit should remain closed
	for i := 0; i < 2; i++ {
		err := cb.Call(ctx, func(ctx context.Context) error {
			return testErr
		})
		assert.ErrorIs(t, err, testErr)
		assert.Equal(t, StateClosed, cb.State(), "should remain closed after %d failures", i+1)
	}

	// Third failure - circuit should open
	err := cb.Call(ctx, func(ctx context.Context) error {
		return testErr
	})
	assert.ErrorIs(t, err, testErr)
	assert.Equal(t, StateOpen, cb.State(), "should open after threshold failures")
}

func TestCircuitBreaker_BlocksWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 1*time.Second)
	ctx := context.Background()

	// Trigger failures to open circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	assert.Equal(t, StateOpen, cb.State())

	// Subsequent calls should fail fast without executing the function
	called := false
	err := cb.Call(ctx, func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.ErrorIs(t, err, ErrCircuitBreakerOpen)
	assert.False(t, called, "function should not be called when circuit is open")
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 100*time.Millisecond)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
	}
	assert.Equal(t, StateOpen, cb.State())

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Next call should transition to half-open
	called := false
	err := cb.Call(ctx, func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "function should be called in half-open state")
}

func TestCircuitBreaker_HalfOpenToClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	// Wait and transition to half-open
	time.Sleep(75 * time.Millisecond)

	// First success in half-open
	err := cb.Call(ctx, func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, StateHalfOpen, cb.State(), "should remain half-open after 1 success")

	// Second success should close the circuit
	err = cb.Call(ctx, func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State(), "should close after threshold successes")
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
	}

	// Wait and transition to half-open
	time.Sleep(75 * time.Millisecond)

	// First call succeeds - transitions to half-open
	_ = cb.Call(ctx, func(ctx context.Context) error {
		return nil
	})
	assert.Equal(t, StateHalfOpen, cb.State())

	// Failure in half-open should reopen the circuit
	err := cb.Call(ctx, func(ctx context.Context) error {
		return errors.New("fail again")
	})
	assert.Error(t, err)
	assert.Equal(t, StateOpen, cb.State(), "should reopen on failure in half-open state")
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 100*time.Millisecond)
	ctx := context.Background()

	// Open the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
	}
	assert.Equal(t, StateOpen, cb.State())

	// Reset should return to closed state
	cb.Reset()
	assert.Equal(t, StateClosed, cb.State())

	// Should accept calls normally
	err := cb.Call(ctx, func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)
	ctx := context.Background()

	// Two failures
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
	}
	assert.Equal(t, StateClosed, cb.State())

	// One success should reset failure count
	err := cb.Call(ctx, func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, StateClosed, cb.State())

	// Two more failures should NOT open (count was reset)
	for i := 0; i < 2; i++ {
		_ = cb.Call(ctx, func(ctx context.Context) error {
			return errors.New("fail")
		})
		assert.Equal(t, StateClosed, cb.State())
	}
}

func TestCircuitBreaker_ConcurrentCalls(t *testing.T) {
	cb := NewCircuitBreaker(10, 2, 100*time.Millisecond)
	ctx := context.Background()

	// Run many concurrent successful calls
	concurrency := 100
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			err := cb.Call(ctx, func(ctx context.Context) error {
				time.Sleep(1 * time.Millisecond)
				return nil
			})
			assert.NoError(t, err)
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < concurrency; i++ {
		<-done
	}

	assert.Equal(t, StateClosed, cb.State())
}

func TestCircuitBreaker_ContextCancellation(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Circuit breaker should still work with cancelled context
	err := cb.Call(ctx, func(ctx context.Context) error {
		return ctx.Err()
	})

	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
