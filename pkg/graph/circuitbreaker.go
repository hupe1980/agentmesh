package graph

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState int32

const (
	// StateClosed indicates the circuit breaker is allowing requests through normally.
	StateClosed CircuitBreakerState = 0

	// StateOpen indicates the circuit breaker is blocking requests due to failures.
	StateOpen CircuitBreakerState = 1

	// StateHalfOpen indicates the circuit breaker is testing if the service has recovered.
	StateHalfOpen CircuitBreakerState = 2
)

// String returns the string representation of the circuit breaker state.
func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// ErrCircuitBreakerOpen is returned when the circuit breaker is in the open state.
var ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

// CircuitBreaker implements the circuit breaker pattern to prevent cascading failures.
//
// The circuit breaker operates in three states:
//   - Closed: Requests flow normally. Failures increment a counter.
//   - Open: Requests fail fast without calling the protected function.
//   - HalfOpen: A limited number of test requests are allowed to check if the service recovered.
//
// State Transitions:
//   - Closed → Open: When failure count reaches the threshold.
//   - Open → HalfOpen: After the configured timeout duration.
//   - HalfOpen → Closed: When success count reaches the threshold.
//   - HalfOpen → Open: On any failure.
//
// Thread Safety:
// CircuitBreaker is safe for concurrent use by multiple goroutines using atomic operations.
//
// Example:
//
//	cb := graph.NewCircuitBreaker(3, 2, 5*time.Second)
//	err := cb.Call(ctx, func(ctx context.Context) error {
//	    return externalService.DoWork(ctx)
//	})
type CircuitBreaker struct {
	failureThreshold int32
	successThreshold int32
	timeout          time.Duration

	failures  atomic.Int32
	successes atomic.Int32
	state     atomic.Int32 // int32 representation of CircuitBreakerState
	openedAt  atomic.Int64 // UnixNano timestamp
}

// NewCircuitBreaker creates a new circuit breaker with the specified configuration.
//
// Parameters:
//   - failureThreshold: Number of consecutive failures before opening the circuit.
//   - successThreshold: Number of consecutive successes in half-open state before closing.
//   - timeout: Duration to wait before transitioning from open to half-open.
//
// Example:
//
//	// Opens after 5 failures, requires 3 successes to close, waits 10s
//	cb := graph.NewCircuitBreaker(5, 3, 10*time.Second)
func NewCircuitBreaker(failureThreshold, successThreshold int32, timeout time.Duration) *CircuitBreaker {
	if failureThreshold < 0 {
		failureThreshold = 0
	}
	if successThreshold < 0 {
		successThreshold = 0
	}

	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
	}
}

// setState atomically sets the circuit breaker state.
func (cb *CircuitBreaker) setState(state CircuitBreakerState) {
	cb.state.Store(int32(state))
}

// getState atomically gets the circuit breaker state.
func (cb *CircuitBreaker) getState() CircuitBreakerState {
	return CircuitBreakerState(cb.state.Load())
}

// Call executes the provided function through the circuit breaker.
//
// If the circuit is open, it returns ErrCircuitBreakerOpen without calling the function.
// If the timeout has elapsed while open, it transitions to half-open and allows the call.
//
// The function's error result determines whether to record a success or failure:
//   - nil error: Success (may close the circuit if in half-open state)
//   - non-nil error: Failure (may open the circuit)
func (cb *CircuitBreaker) Call(ctx context.Context, fn func(context.Context) error) error {
	state := cb.getState()

	// If circuit is open, check if timeout has passed
	if state == StateOpen {
		openedAt := time.Unix(0, cb.openedAt.Load())
		if time.Since(openedAt) > cb.timeout {
			// Transition to half-open
			cb.setState(StateHalfOpen)
			cb.successes.Store(0)
		} else {
			return ErrCircuitBreakerOpen
		}
	}

	// Execute the function
	err := fn(ctx)

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}

	return err
}

// onFailure handles a failed call and potentially opens the circuit.
func (cb *CircuitBreaker) onFailure() {
	state := cb.getState()
	failures := cb.failures.Add(1)

	if state == StateHalfOpen {
		// Any failure in half-open state reopens the circuit
		cb.setState(StateOpen)
		cb.failures.Store(0)
		cb.openedAt.Store(time.Now().UnixNano())
		return
	}

	if failures >= cb.failureThreshold {
		// Too many failures, open the circuit
		cb.setState(StateOpen)
		cb.openedAt.Store(time.Now().UnixNano())
		cb.failures.Store(0)
	}
}

// onSuccess handles a successful call and potentially closes the circuit.
func (cb *CircuitBreaker) onSuccess() {
	state := cb.getState()
	cb.failures.Store(0) // Reset failure count on success

	if state == StateHalfOpen {
		successes := cb.successes.Add(1)
		if successes >= cb.successThreshold {
			// Enough successes, close the circuit
			cb.setState(StateClosed)
			cb.successes.Store(0)
		}
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitBreakerState {
	return cb.getState()
}

// Reset manually resets the circuit breaker to the closed state.
// This is useful for testing or manual intervention.
func (cb *CircuitBreaker) Reset() {
	cb.setState(StateClosed)
	cb.failures.Store(0)
	cb.successes.Store(0)
	cb.openedAt.Store(0)
}
