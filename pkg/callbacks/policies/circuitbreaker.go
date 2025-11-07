package policies

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// StateClosed means the circuit is closed and requests pass through
	StateClosed CircuitState = iota
	// StateOpen means the circuit is open and requests are rejected
	StateOpen
	// StateHalfOpen means the circuit is testing if it should close
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// circuitBreakerState tracks circuit breaker state.
type circuitBreakerState struct {
	mu sync.Mutex

	state        CircuitState
	failureCount int
	lastFailure  time.Time
	openedAt     time.Time
}

// CircuitBreakerConfig configures circuit breaker behavior.
type CircuitBreakerConfig struct {
	// MaxFailures is the number of consecutive failures before opening the circuit.
	MaxFailures int
	// Timeout is the duration to wait in Open state before transitioning to Half-Open.
	Timeout time.Duration
	// FailureWindow is the time window for tracking failures in Closed state.
	FailureWindow time.Duration
}

// DefaultCircuitBreakerConfig returns a CircuitBreakerConfig with sensible defaults.
// MaxFailures: 5, Timeout: 30s, FailureWindow: 1m
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:   5,
		Timeout:       30 * time.Second,
		FailureWindow: 1 * time.Minute,
	}
}

// CircuitBreaker returns three callbacks that implement the circuit breaker pattern.
// The circuit breaker prevents cascading failures by temporarily blocking requests
// when error rates exceed thresholds.
//
// The circuit breaker has three states:
//   - Closed: Normal operation, requests pass through
//   - Open: Too many failures detected, requests are short-circuited with error messages
//   - Half-Open: Testing recovery by allowing one request through
//
// State transitions:
//   - Closed → Open: When failure count reaches MaxFailures
//   - Open → Half-Open: After Timeout duration elapses
//   - Half-Open → Closed: When a request succeeds
//   - Half-Open → Open: When a request fails
//
// Returns:
//   - before: BeforeModelCallback that checks circuit state and blocks if Open
//   - after: AfterModelCallback that resets failure count on success in Half-Open state
//   - onError: OnModelErrorCallback that tracks failures and opens circuit when threshold exceeded
//
// Example:
//
//	config := policies.DefaultCircuitBreakerConfig()
//	before, after, onError := policies.CircuitBreaker(config)
//	manager.RegisterBeforeModel(before)
//	manager.RegisterAfterModel(after)
//	manager.RegisterOnModelError(onError)
func CircuitBreaker(config CircuitBreakerConfig) (
	before callbacks.BeforeModelCallback,
	after callbacks.AfterModelCallback,
	onError callbacks.OnModelErrorCallback,
) {
	state := &circuitBreakerState{
		state: StateClosed,
	}

	// BeforeModel: Check if circuit is open
	before = func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		now := time.Now()

		switch state.state {
		case StateClosed:
			// Normal operation
			return nil, nil

		case StateOpen:
			// Check if timeout has elapsed
			if now.Sub(state.openedAt) >= config.Timeout {
				// Transition to half-open
				state.state = StateHalfOpen
				return nil, nil
			}

			// Circuit is still open
			return message.NewAIMessageFromText(
				fmt.Sprintf("Circuit breaker is open (failed %d times). Try again in %v.",
					state.failureCount,
					config.Timeout-now.Sub(state.openedAt)),
			), nil

		case StateHalfOpen:
			// Allow one request through to test
			return nil, nil

		default:
			return nil, nil
		}
	}

	// AfterModel: Reset on success
	after = func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		now := time.Now()

		switch state.state {
		case StateClosed:
			// Check if failures are outside the window
			if !state.lastFailure.IsZero() && now.Sub(state.lastFailure) > config.FailureWindow {
				state.failureCount = 0
			}

		case StateHalfOpen:
			// Success in half-open state: close the circuit
			state.state = StateClosed
			state.failureCount = 0
		}

		return nil, nil // Keep original response
	}

	// OnModelError: Track failures
	onError = func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		now := time.Now()

		// Increment failure count
		state.failureCount++
		state.lastFailure = now

		// Check if we should open the circuit
		if state.failureCount >= config.MaxFailures {
			state.state = StateOpen
			state.openedAt = now
		}

		return nil, err // Pass through the original error
	}

	return before, after, onError
}

// PerNodeCircuitBreaker returns circuit breaker callbacks for a specific node.
// Each node gets an independent circuit breaker, allowing different failure thresholds
// and recovery policies per node.
//
// The nodeName is included in error messages to identify which circuit breaker opened.
// See CircuitBreaker for details on states and behavior.
//
// Returns:
//   - before: BeforeModelCallback that checks circuit state for this node
//   - after: AfterModelCallback that resets failure count on success
//   - onError: OnModelErrorCallback that tracks failures for this node
//
// Example:
//
//	config := policies.DefaultCircuitBreakerConfig()
//	config.MaxFailures = 3
//	before, after, onError := policies.PerNodeCircuitBreaker("expensive_api", config)
//	manager.RegisterBeforeModel(before)
//	manager.RegisterAfterModel(after)
//	manager.RegisterOnModelError(onError)
func PerNodeCircuitBreaker(nodeName string, config CircuitBreakerConfig) (
	before callbacks.BeforeModelCallback,
	after callbacks.AfterModelCallback,
	onError callbacks.OnModelErrorCallback,
) {
	state := &circuitBreakerState{
		state: StateClosed,
	}

	before = func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		now := time.Now()

		switch state.state {
		case StateClosed:
			return nil, nil

		case StateOpen:
			if now.Sub(state.openedAt) >= config.Timeout {
				state.state = StateHalfOpen
				return nil, nil
			}

			return message.NewAIMessageFromText(
				fmt.Sprintf("Circuit breaker for '%s' is open (failed %d times). Try again in %v.",
					nodeName,
					state.failureCount,
					config.Timeout-now.Sub(state.openedAt)),
			), nil

		case StateHalfOpen:
			return nil, nil

		default:
			return nil, nil
		}
	}

	after = func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		now := time.Now()

		switch state.state {
		case StateClosed:
			if !state.lastFailure.IsZero() && now.Sub(state.lastFailure) > config.FailureWindow {
				state.failureCount = 0
			}

		case StateHalfOpen:
			state.state = StateClosed
			state.failureCount = 0
		}

		return nil, nil
	}

	onError = func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		state.mu.Lock()
		defer state.mu.Unlock()

		now := time.Now()
		state.failureCount++
		state.lastFailure = now

		if state.failureCount >= config.MaxFailures {
			state.state = StateOpen
			state.openedAt = now
		}

		return nil, err
	}

	return before, after, onError
}
