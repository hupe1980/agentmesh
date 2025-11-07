package policies

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// retryState tracks retry attempts and backoff.
type retryState struct {
	mu            sync.Mutex
	attempts      int
	lastAttemptAt time.Time
}

// RetryConfig configures retry behavior with exponential backoff.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts (0 means no retries).
	MaxAttempts int
	// InitialDelay is the delay before the first retry.
	InitialDelay time.Duration
	// MaxDelay is the maximum backoff delay (caps exponential growth).
	MaxDelay time.Duration
	// BackoffMultiplier is the factor for exponential backoff (typically 2.0).
	BackoffMultiplier float64
	// Jitter adds randomness (0.0-1.0) to prevent thundering herd.
	Jitter float64
}

// DefaultRetryConfig returns a RetryConfig with sensible defaults for most use cases.
// MaxAttempts: 3, InitialDelay: 1s, MaxDelay: 30s, BackoffMultiplier: 2.0, Jitter: 0.1
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            0.1,
	}
}

// ExponentialBackoffRetry returns an OnModelErrorCallback that retries failed requests with exponential backoff.
// It uses closure-based state to track retry attempts and implements the backoff formula:
// delay = min(InitialDelay × (BackoffMultiplier ^ attempt), MaxDelay), with optional jitter.
//
// When max attempts are exceeded, the callback short-circuits with an AI message containing
// the failure details and resets the retry counter for the next error.
//
// The callback sleeps for the calculated backoff duration before returning nil to trigger
// a retry. Jitter is applied to prevent synchronized retries across multiple instances.
//
// Example:
//
//	config := policies.DefaultRetryConfig()
//	config.MaxAttempts = 5
//	manager.RegisterOnModelError(policies.ExponentialBackoffRetry(config))
func ExponentialBackoffRetry(config RetryConfig) callbacks.OnModelErrorCallback {
	state := &retryState{}

	return func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		state.attempts++

		// Check if we've exceeded max attempts
		if state.attempts > config.MaxAttempts {
			state.attempts = 0 // Reset for next error
			return message.NewAIMessageFromText(
				fmt.Sprintf("Failed after %d attempts: %v", config.MaxAttempts, err),
			), nil
		}

		// Calculate exponential backoff delay
		delay := calculateBackoff(state.attempts-1, config)
		state.lastAttemptAt = time.Now()

		// Sleep for the backoff duration
		time.Sleep(delay)

		// Return nil error to trigger retry
		return nil, nil
	}
}

// RetryWithTimeout returns an OnModelErrorCallback that retries with an overall timeout.
// If the total time spent retrying exceeds the timeout duration, the callback gives up
// and short-circuits with an AI message.
//
// The timeout is measured from when the callback is first created, not from each retry attempt.
// This ensures the total retry duration is bounded even if individual retries take time.
//
// Example:
//
//	config := policies.DefaultRetryConfig()
//	manager.RegisterOnModelError(policies.RetryWithTimeout(config, 5*time.Minute))
func RetryWithTimeout(config RetryConfig, timeout time.Duration) callbacks.OnModelErrorCallback {
	state := &retryState{}
	startTime := time.Now()

	return func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		// Check if timeout has elapsed
		if time.Since(startTime) > timeout {
			state.attempts = 0
			return message.NewAIMessageFromText(
				fmt.Sprintf("Retry timeout exceeded after %d attempts: %v", state.attempts, err),
			), nil
		}

		state.attempts++

		// Check max attempts
		if state.attempts > config.MaxAttempts {
			state.attempts = 0
			return message.NewAIMessageFromText(
				fmt.Sprintf("Failed after %d attempts: %v", config.MaxAttempts, err),
			), nil
		}

		// Calculate and apply backoff
		delay := calculateBackoff(state.attempts-1, config)
		state.lastAttemptAt = time.Now()
		time.Sleep(delay)

		return nil, nil
	}
}

// ConditionalRetry returns an OnModelErrorCallback that only retries specific error types.
// The shouldRetry predicate function determines whether each error is retryable.
// Non-retryable errors are propagated immediately without retry attempts.
//
// This is useful for distinguishing between transient errors (network timeouts, rate limits)
// and permanent errors (invalid input, authentication failures).
//
// Example:
//
//	config := policies.DefaultRetryConfig()
//	shouldRetry := func(err error) bool {
//	    return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrTimeout)
//	}
//	manager.RegisterOnModelError(policies.ConditionalRetry(config, shouldRetry))
func ConditionalRetry(config RetryConfig, shouldRetry func(error) bool) callbacks.OnModelErrorCallback {
	state := &retryState{}

	return func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		// Check if error is retryable
		if !shouldRetry(err) {
			return nil, err // Don't retry, propagate error
		}

		state.mu.Lock()
		defer state.mu.Unlock()

		state.attempts++

		if state.attempts > config.MaxAttempts {
			state.attempts = 0
			return message.NewAIMessageFromText(
				fmt.Sprintf("Failed after %d attempts: %v", config.MaxAttempts, err),
			), nil
		}

		delay := calculateBackoff(state.attempts-1, config)
		state.lastAttemptAt = time.Now()
		time.Sleep(delay)

		return nil, nil
	}
}

// calculateBackoff computes the exponential backoff delay for a given attempt number.
// It applies the formula: delay = min(InitialDelay × (BackoffMultiplier ^ attempt), MaxDelay)
// and then reduces the delay by a random jitter amount to prevent thundering herd effects.
//
// The jitter calculation uses nanosecond timestamp randomness to vary the delay by up to
// config.Jitter percent of the calculated delay.
func calculateBackoff(attempt int, config RetryConfig) time.Duration {
	// Exponential backoff: delay = initialDelay * (multiplier ^ attempt)
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffMultiplier, float64(attempt))

	// Apply max delay cap
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// Apply jitter to prevent thundering herd
	// Jitter reduces delay by up to config.Jitter percent
	if config.Jitter > 0 {
		jitterAmount := delay * config.Jitter * (1.0 - (float64(time.Now().UnixNano()%1000) / 1000.0))
		delay -= jitterAmount
	}

	return time.Duration(delay)
}
