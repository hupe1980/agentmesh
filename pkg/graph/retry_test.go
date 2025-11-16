package graph

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type APIError struct {
	StatusCode int
	Msg        string
}

func (e *APIError) Error() string {
	return e.Msg
}

func TestNewRetryPolicy(t *testing.T) {
	builder := NewRetryPolicy()
	policy := builder.Build()

	assert.Equal(t, 3, policy.MaxAttempts, "default should be 3 attempts")
	assert.NotNil(t, policy.Backoff, "should have default backoff")
	assert.Nil(t, policy.Retryable, "default should retry all errors")
}

func TestRetryPolicyBuilder_WithMaxAttempts(t *testing.T) {
	policy := NewRetryPolicy().
		WithMaxAttempts(5).
		Build()

	assert.Equal(t, 5, policy.MaxAttempts)
}

func TestRetryPolicyBuilder_WithNoRetries(t *testing.T) {
	policy := NewRetryPolicy().
		WithNoRetries().
		Build()

	assert.Equal(t, 1, policy.MaxAttempts)
}

func TestRetryPolicyBuilder_WithExponentialBackoff(t *testing.T) {
	policy := NewRetryPolicy().
		WithExponentialBackoff(time.Second, 2.0).
		Build()

	require.NotNil(t, policy.Backoff)

	assert.Equal(t, time.Second, policy.Backoff(1))   // 1s * 2^0 = 1s
	assert.Equal(t, 2*time.Second, policy.Backoff(2)) // 1s * 2^1 = 2s
	assert.Equal(t, 4*time.Second, policy.Backoff(3)) // 1s * 2^2 = 4s
	assert.Equal(t, 8*time.Second, policy.Backoff(4)) // 1s * 2^3 = 8s
}

func TestRetryPolicyBuilder_WithLinearBackoff(t *testing.T) {
	policy := NewRetryPolicy().
		WithLinearBackoff(time.Second).
		Build()

	require.NotNil(t, policy.Backoff)

	assert.Equal(t, time.Second, policy.Backoff(1))     // 1s * 1 = 1s
	assert.Equal(t, 2*time.Second, policy.Backoff(2))   // 1s * 2 = 2s
	assert.Equal(t, 3*time.Second, policy.Backoff(3))   // 1s * 3 = 3s
	assert.Equal(t, 10*time.Second, policy.Backoff(10)) // 1s * 10 = 10s
}

func TestRetryPolicyBuilder_WithConstantBackoff(t *testing.T) {
	policy := NewRetryPolicy().
		WithConstantBackoff(500 * time.Millisecond).
		Build()

	require.NotNil(t, policy.Backoff)

	assert.Equal(t, 500*time.Millisecond, policy.Backoff(1))
	assert.Equal(t, 500*time.Millisecond, policy.Backoff(2))
	assert.Equal(t, 500*time.Millisecond, policy.Backoff(5))
	assert.Equal(t, 500*time.Millisecond, policy.Backoff(100))
}

func TestRetryPolicyBuilder_WithCustomBackoff(t *testing.T) {
	// Fibonacci backoff: 1, 1, 2, 3, 5, 8, ...
	fibonacci := func(attempt int) time.Duration {
		if attempt <= 0 {
			return 0
		}
		if attempt <= 2 {
			return time.Second
		}
		a, b := time.Second, time.Second
		for i := 2; i < attempt; i++ {
			a, b = b, a+b
		}
		return b
	}

	policy := NewRetryPolicy().
		WithCustomBackoff(fibonacci).
		Build()

	require.NotNil(t, policy.Backoff)

	assert.Equal(t, time.Second, policy.Backoff(1))   // 1s
	assert.Equal(t, time.Second, policy.Backoff(2))   // 1s
	assert.Equal(t, 2*time.Second, policy.Backoff(3)) // 2s
	assert.Equal(t, 3*time.Second, policy.Backoff(4)) // 3s
	assert.Equal(t, 5*time.Second, policy.Backoff(5)) // 5s
}

func TestRetryPolicyBuilder_WithRetryableErrors(t *testing.T) {
	ErrTransient := errors.New("transient")
	ErrTimeout := errors.New("timeout")
	ErrPermanent := errors.New("permanent")

	policy := NewRetryPolicy().
		WithRetryableErrors(ErrTransient, ErrTimeout).
		Build()

	require.NotNil(t, policy.Retryable)

	assert.True(t, policy.Retryable(ErrTransient))
	assert.True(t, policy.Retryable(ErrTimeout))
	assert.False(t, policy.Retryable(ErrPermanent))
}

func TestRetryPolicyBuilder_WithRetryableErrors_WrappedErrors(t *testing.T) {
	ErrTransient := errors.New("transient")
	wrappedErr := errors.Join(ErrTransient, errors.New("additional context"))

	policy := NewRetryPolicy().
		WithRetryableErrors(ErrTransient).
		Build()

	require.NotNil(t, policy.Retryable)

	// Should work with wrapped errors (errors.Is)
	assert.True(t, policy.Retryable(wrappedErr))
}

func TestRetryPolicyBuilder_WithNonRetryableErrors(t *testing.T) {
	ErrInvalidInput := errors.New("invalid input")
	ErrUnauthorized := errors.New("unauthorized")
	ErrTransient := errors.New("transient")

	policy := NewRetryPolicy().
		WithNonRetryableErrors(ErrInvalidInput, ErrUnauthorized).
		Build()

	require.NotNil(t, policy.Retryable)

	assert.False(t, policy.Retryable(ErrInvalidInput))
	assert.False(t, policy.Retryable(ErrUnauthorized))
	assert.True(t, policy.Retryable(ErrTransient))
}

func TestRetryPolicyBuilder_WithRetryableFunc(t *testing.T) {
	policy := NewRetryPolicy().
		WithRetryableFunc(func(err error) bool {
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				return apiErr.StatusCode >= 500
			}
			return false
		}).
		Build()

	require.NotNil(t, policy.Retryable)

	assert.True(t, policy.Retryable(&APIError{StatusCode: 500, Msg: "server error"}))
	assert.True(t, policy.Retryable(&APIError{StatusCode: 503, Msg: "service unavailable"}))
	assert.False(t, policy.Retryable(&APIError{StatusCode: 400, Msg: "bad request"}))
	assert.False(t, policy.Retryable(&APIError{StatusCode: 404, Msg: "not found"}))
	assert.False(t, policy.Retryable(errors.New("other error")))
}

func TestBackoffFunctions(t *testing.T) {
	t.Run("ExponentialBackoff", func(t *testing.T) {
		backoff := ExponentialBackoff(100*time.Millisecond, 2.0)

		assert.Equal(t, time.Duration(0), backoff(0))
		assert.Equal(t, 100*time.Millisecond, backoff(1))
		assert.Equal(t, 200*time.Millisecond, backoff(2))
		assert.Equal(t, 400*time.Millisecond, backoff(3))
		assert.Equal(t, 800*time.Millisecond, backoff(4))
	})

	t.Run("LinearBackoff", func(t *testing.T) {
		backoff := LinearBackoff(100 * time.Millisecond)

		assert.Equal(t, time.Duration(0), backoff(0))
		assert.Equal(t, 100*time.Millisecond, backoff(1))
		assert.Equal(t, 200*time.Millisecond, backoff(2))
		assert.Equal(t, 300*time.Millisecond, backoff(3))
		assert.Equal(t, 1000*time.Millisecond, backoff(10))
	})

	t.Run("ConstantBackoff", func(t *testing.T) {
		backoff := ConstantBackoff(250 * time.Millisecond)

		assert.Equal(t, time.Duration(0), backoff(0))
		assert.Equal(t, 250*time.Millisecond, backoff(1))
		assert.Equal(t, 250*time.Millisecond, backoff(2))
		assert.Equal(t, 250*time.Millisecond, backoff(10))
		assert.Equal(t, 250*time.Millisecond, backoff(100))
	})

	t.Run("CappedExponentialBackoff", func(t *testing.T) {
		backoff := CappedExponentialBackoff(100*time.Millisecond, 2.0, time.Second)

		assert.Equal(t, time.Duration(0), backoff(0))
		assert.Equal(t, 100*time.Millisecond, backoff(1))
		assert.Equal(t, 200*time.Millisecond, backoff(2))
		assert.Equal(t, 400*time.Millisecond, backoff(3))
		assert.Equal(t, 800*time.Millisecond, backoff(4))
		assert.Equal(t, time.Second, backoff(5))  // Capped at 1s
		assert.Equal(t, time.Second, backoff(10)) // Still capped
	})

	t.Run("JitteredExponentialBackoff", func(t *testing.T) {
		backoff := JitteredExponentialBackoff(100*time.Millisecond, 2.0, 0.1)

		// With 10% jitter, values should be within ±10% of expected
		for attempt := 1; attempt <= 4; attempt++ {
			expected := 100 * time.Millisecond * (1 << (attempt - 1))
			actual := backoff(attempt)

			// Allow ±10% jitter
			lower := time.Duration(float64(expected) * 0.9)
			upper := time.Duration(float64(expected) * 1.1)

			assert.GreaterOrEqual(t, actual, lower, "attempt %d should be >= %v", attempt, lower)
			assert.LessOrEqual(t, actual, upper, "attempt %d should be <= %v", attempt, upper)
		}
	})
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	t.Run("AllErrorsRetryable", func(t *testing.T) {
		policy := &RetryPolicy{
			MaxAttempts: 3,
			Retryable:   nil, // nil means all errors retryable
		}

		assert.True(t, policy.ShouldRetry(errors.New("any error")))
		assert.True(t, policy.ShouldRetry(errors.New("another error")))
	})

	t.Run("SelectiveRetry", func(t *testing.T) {
		ErrRetryable := errors.New("retryable")
		policy := &RetryPolicy{
			MaxAttempts: 3,
			Retryable: func(err error) bool {
				return errors.Is(err, ErrRetryable)
			},
		}

		assert.True(t, policy.ShouldRetry(ErrRetryable))
		assert.False(t, policy.ShouldRetry(errors.New("other error")))
	})
}

func TestRetryPolicy_GetBackoffDuration(t *testing.T) {
	t.Run("WithBackoffFunction", func(t *testing.T) {
		policy := &RetryPolicy{
			MaxAttempts: 3,
			Backoff:     ConstantBackoff(time.Second),
		}

		assert.Equal(t, time.Second, policy.GetBackoffDuration(1))
		assert.Equal(t, time.Second, policy.GetBackoffDuration(2))
	})

	t.Run("WithoutBackoffFunction", func(t *testing.T) {
		policy := &RetryPolicy{
			MaxAttempts: 3,
			Backoff:     nil,
		}

		assert.Equal(t, time.Duration(0), policy.GetBackoffDuration(1))
		assert.Equal(t, time.Duration(0), policy.GetBackoffDuration(2))
	})
}
