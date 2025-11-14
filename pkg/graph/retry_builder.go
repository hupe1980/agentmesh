package graph

import (
	"errors"
	"math"
	"time"
)

// RetryPolicyBuilder provides a fluent API for constructing retry policies.
// Use NewRetryPolicy() to create a builder, then chain configuration methods.
//
// Example:
//
//	policy := graph.NewRetryPolicy().
//	    WithMaxAttempts(5).
//	    WithExponentialBackoff(time.Second, 2.0).
//	    WithRetryableErrors(ErrTransient, ErrTimeout).
//	    Build()
type RetryPolicyBuilder struct {
	maxAttempts int
	backoff     func(attempt int) time.Duration
	retryable   func(error) bool
}

// NewRetryPolicy creates a new retry policy builder with sensible defaults:
//   - MaxAttempts: 3
//   - Backoff: Exponential (base 1s, multiplier 2.0)
//   - Retryable: All errors are retryable
func NewRetryPolicy() *RetryPolicyBuilder {
	return &RetryPolicyBuilder{
		maxAttempts: 3,
		backoff:     ExponentialBackoff(time.Second, 2.0),
		retryable:   nil, // nil means all errors are retryable
	}
}

// WithMaxAttempts sets the maximum number of execution attempts (including the initial one).
// A value <= 1 means no retries (single attempt only).
func (b *RetryPolicyBuilder) WithMaxAttempts(attempts int) *RetryPolicyBuilder {
	b.maxAttempts = attempts
	return b
}

// WithExponentialBackoff configures exponential backoff with the given base and multiplier.
// Wait time = base * (multiplier ^ attempt).
//
// Example:
//
//	WithExponentialBackoff(time.Second, 2.0) // 1s, 2s, 4s, 8s, ...
func (b *RetryPolicyBuilder) WithExponentialBackoff(base time.Duration, multiplier float64) *RetryPolicyBuilder {
	b.backoff = ExponentialBackoff(base, multiplier)
	return b
}

// WithLinearBackoff configures linear backoff with the given base duration.
// Wait time = base * attempt.
//
// Example:
//
//	WithLinearBackoff(time.Second) // 1s, 2s, 3s, 4s, ...
func (b *RetryPolicyBuilder) WithLinearBackoff(base time.Duration) *RetryPolicyBuilder {
	b.backoff = LinearBackoff(base)
	return b
}

// WithConstantBackoff configures constant backoff with the given wait duration.
// Wait time is always the same between retries.
//
// Example:
//
//	WithConstantBackoff(time.Second) // 1s, 1s, 1s, 1s, ...
func (b *RetryPolicyBuilder) WithConstantBackoff(wait time.Duration) *RetryPolicyBuilder {
	b.backoff = ConstantBackoff(wait)
	return b
}

// WithCustomBackoff allows you to provide a custom backoff function.
// The function receives the attempt number (1-indexed) and returns the wait duration.
func (b *RetryPolicyBuilder) WithCustomBackoff(fn func(attempt int) time.Duration) *RetryPolicyBuilder {
	b.backoff = fn
	return b
}

// WithRetryableErrors configures the policy to only retry specific errors.
// Uses errors.Is() for matching, so wrapped errors are supported.
//
// Example:
//
//	WithRetryableErrors(ErrTransient, ErrTimeout, ErrRateLimit)
func (b *RetryPolicyBuilder) WithRetryableErrors(errs ...error) *RetryPolicyBuilder {
	b.retryable = func(err error) bool {
		for _, retryableErr := range errs {
			if errors.Is(err, retryableErr) {
				return true
			}
		}
		return false
	}
	return b
}

// WithNonRetryableErrors configures the policy to retry all errors EXCEPT the specified ones.
// Uses errors.Is() for matching, so wrapped errors are supported.
//
// Example:
//
//	WithNonRetryableErrors(ErrInvalidInput, ErrUnauthorized)
func (b *RetryPolicyBuilder) WithNonRetryableErrors(errs ...error) *RetryPolicyBuilder {
	b.retryable = func(err error) bool {
		for _, nonRetryableErr := range errs {
			if errors.Is(err, nonRetryableErr) {
				return false
			}
		}
		return true
	}
	return b
}

// WithRetryableFunc provides a custom function to determine if an error should be retried.
// This gives you full control over retry logic.
//
// Example:
//
//	WithRetryableFunc(func(err error) bool {
//	    var apiErr *APIError
//	    return errors.As(err, &apiErr) && apiErr.StatusCode >= 500
//	})
func (b *RetryPolicyBuilder) WithRetryableFunc(fn func(error) bool) *RetryPolicyBuilder {
	b.retryable = fn
	return b
}

// WithNoRetries disables retries completely (equivalent to WithMaxAttempts(1)).
func (b *RetryPolicyBuilder) WithNoRetries() *RetryPolicyBuilder {
	b.maxAttempts = 1
	return b
}

// Build constructs the final RetryPolicy from the builder configuration.
func (b *RetryPolicyBuilder) Build() *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: b.maxAttempts,
		Backoff:     b.backoff,
		Retryable:   b.retryable,
	}
}

// Backoff Strategy Functions

// ExponentialBackoff returns a backoff function that implements exponential backoff.
// Wait time = base * (multiplier ^ attempt).
//
// Example:
//
//	backoff := ExponentialBackoff(time.Second, 2.0)
//	backoff(1) // 1s
//	backoff(2) // 2s
//	backoff(3) // 4s
func ExponentialBackoff(base time.Duration, multiplier float64) func(int) time.Duration {
	return func(attempt int) time.Duration {
		if attempt <= 0 {
			return 0
		}
		return time.Duration(float64(base) * math.Pow(multiplier, float64(attempt-1)))
	}
}

// LinearBackoff returns a backoff function that implements linear backoff.
// Wait time = base * attempt.
//
// Example:
//
//	backoff := LinearBackoff(time.Second)
//	backoff(1) // 1s
//	backoff(2) // 2s
//	backoff(3) // 3s
func LinearBackoff(base time.Duration) func(int) time.Duration {
	return func(attempt int) time.Duration {
		if attempt <= 0 {
			return 0
		}
		return base * time.Duration(attempt)
	}
}

// ConstantBackoff returns a backoff function that waits a constant duration.
// Wait time is always the same.
//
// Example:
//
//	backoff := ConstantBackoff(time.Second)
//	backoff(1) // 1s
//	backoff(2) // 1s
//	backoff(3) // 1s
func ConstantBackoff(wait time.Duration) func(int) time.Duration {
	return func(attempt int) time.Duration {
		if attempt <= 0 {
			return 0
		}
		return wait
	}
}

// JitteredExponentialBackoff returns an exponential backoff with random jitter.
// This helps avoid thundering herd problems when multiple clients retry simultaneously.
// Wait time = base * (multiplier ^ attempt) * (1 ± jitter).
//
// Example:
//
//	backoff := JitteredExponentialBackoff(time.Second, 2.0, 0.1) // ±10% jitter
func JitteredExponentialBackoff(base time.Duration, multiplier, jitterFactor float64) func(int) time.Duration {
	return func(attempt int) time.Duration {
		if attempt <= 0 {
			return 0
		}
		baseWait := float64(base) * math.Pow(multiplier, float64(attempt-1))
		// Add random jitter: ±jitterFactor
		jitter := baseWait * jitterFactor * (2.0*float64(time.Now().UnixNano()%100)/100.0 - 1.0)
		return time.Duration(baseWait + jitter)
	}
}

// CappedExponentialBackoff returns exponential backoff with a maximum wait time.
// Prevents wait times from growing unbounded.
//
// Example:
//
//	backoff := CappedExponentialBackoff(time.Second, 2.0, 30*time.Second) // Max 30s
func CappedExponentialBackoff(base time.Duration, multiplier float64, maxWait time.Duration) func(int) time.Duration {
	return func(attempt int) time.Duration {
		if attempt <= 0 {
			return 0
		}
		wait := time.Duration(float64(base) * math.Pow(multiplier, float64(attempt-1)))
		if wait > maxWait {
			return maxWait
		}
		return wait
	}
}
