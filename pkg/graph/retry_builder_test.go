package graph

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

type testAPIError struct {
	StatusCode int
}

func (e *testAPIError) Error() string {
	return "API error"
}

func TestRetryPolicyBuilder_WithRetryableFunc(t *testing.T) {
	policy := NewRetryPolicy().
		WithRetryableFunc(func(err error) bool {
			var apiErr *testAPIError
			if errors.As(err, &apiErr) {
				return apiErr.StatusCode >= 500
			}
			return false
		}).
		Build()

	require.NotNil(t, policy.Retryable)

	assert.True(t, policy.Retryable(&testAPIError{StatusCode: 500}))
	assert.True(t, policy.Retryable(&testAPIError{StatusCode: 503}))
	assert.False(t, policy.Retryable(&testAPIError{StatusCode: 400}))
	assert.False(t, policy.Retryable(&testAPIError{StatusCode: 404}))
	assert.False(t, policy.Retryable(errors.New("other error")))
}

func TestRetryPolicyBuilder_FluentChaining(t *testing.T) {
	ErrTransient := errors.New("transient")

	policy := NewRetryPolicy().
		WithMaxAttempts(5).
		WithExponentialBackoff(time.Second, 2.0).
		WithRetryableErrors(ErrTransient).
		Build()

	assert.Equal(t, 5, policy.MaxAttempts)
	assert.NotNil(t, policy.Backoff)
	assert.NotNil(t, policy.Retryable)

	assert.Equal(t, time.Second, policy.Backoff(1))
	assert.True(t, policy.Retryable(ErrTransient))
	assert.False(t, policy.Retryable(errors.New("other")))
}

// Test backoff helper functions directly

func TestExponentialBackoff(t *testing.T) {
	backoff := ExponentialBackoff(time.Second, 2.0)

	assert.Equal(t, time.Duration(0), backoff(0))
	assert.Equal(t, time.Second, backoff(1))
	assert.Equal(t, 2*time.Second, backoff(2))
	assert.Equal(t, 4*time.Second, backoff(3))
	assert.Equal(t, 8*time.Second, backoff(4))
	assert.Equal(t, 16*time.Second, backoff(5))
}

func TestExponentialBackoff_DifferentMultiplier(t *testing.T) {
	backoff := ExponentialBackoff(time.Second, 3.0)

	assert.Equal(t, time.Second, backoff(1))   // 1s * 3^0 = 1s
	assert.Equal(t, 3*time.Second, backoff(2)) // 1s * 3^1 = 3s
	assert.Equal(t, 9*time.Second, backoff(3)) // 1s * 3^2 = 9s
}

func TestLinearBackoff(t *testing.T) {
	backoff := LinearBackoff(time.Second)

	assert.Equal(t, time.Duration(0), backoff(0))
	assert.Equal(t, time.Second, backoff(1))
	assert.Equal(t, 2*time.Second, backoff(2))
	assert.Equal(t, 3*time.Second, backoff(3))
	assert.Equal(t, 10*time.Second, backoff(10))
}

func TestConstantBackoff(t *testing.T) {
	backoff := ConstantBackoff(500 * time.Millisecond)

	assert.Equal(t, time.Duration(0), backoff(0))
	assert.Equal(t, 500*time.Millisecond, backoff(1))
	assert.Equal(t, 500*time.Millisecond, backoff(2))
	assert.Equal(t, 500*time.Millisecond, backoff(100))
}

func TestJitteredExponentialBackoff(t *testing.T) {
	backoff := JitteredExponentialBackoff(time.Second, 2.0, 0.1)

	// With 10% jitter, wait should be within ±10% of base value
	for attempt := 1; attempt <= 5; attempt++ {
		wait := backoff(attempt)
		baseWait := ExponentialBackoff(time.Second, 2.0)(attempt)

		minWait := float64(baseWait) * 0.9
		maxWait := float64(baseWait) * 1.1

		assert.GreaterOrEqual(t, float64(wait), minWait, "wait should be >= 90%% of base for attempt %d", attempt)
		assert.LessOrEqual(t, float64(wait), maxWait, "wait should be <= 110%% of base for attempt %d", attempt)
	}
}

func TestCappedExponentialBackoff(t *testing.T) {
	maxWait := 10 * time.Second
	backoff := CappedExponentialBackoff(time.Second, 2.0, maxWait)

	assert.Equal(t, time.Duration(0), backoff(0))
	assert.Equal(t, time.Second, backoff(1))   // 1s
	assert.Equal(t, 2*time.Second, backoff(2)) // 2s
	assert.Equal(t, 4*time.Second, backoff(3)) // 4s
	assert.Equal(t, 8*time.Second, backoff(4)) // 8s
	assert.Equal(t, maxWait, backoff(5))       // Would be 16s, but capped at 10s
	assert.Equal(t, maxWait, backoff(6))       // Would be 32s, but capped at 10s
	assert.Equal(t, maxWait, backoff(10))      // Would be 512s, but capped at 10s
}

func TestRetryPolicyBuilder_ReuseBuilder(t *testing.T) {
	// Builder can be reused to create multiple policies
	builder := NewRetryPolicy().WithMaxAttempts(5)

	policy1 := builder.WithLinearBackoff(time.Second).Build()
	policy2 := builder.WithConstantBackoff(500 * time.Millisecond).Build()

	// Both policies should have 5 max attempts
	assert.Equal(t, 5, policy1.MaxAttempts)
	assert.Equal(t, 5, policy2.MaxAttempts)

	// But different backoffs
	assert.Equal(t, time.Second, policy1.Backoff(1))
	assert.Equal(t, 500*time.Millisecond, policy2.Backoff(1))
}
