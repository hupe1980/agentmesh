package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// StateClosed allows all requests through.
	StateClosed CircuitState = iota
	// StateOpen rejects all requests.
	StateOpen
	// StateHalfOpen allows limited requests to test recovery.
	StateHalfOpen
)

// CircuitBreakerMiddleware implements circuit breaker pattern for tool execution.
// Prevents cascading failures by stopping execution when error rate is high.
type CircuitBreakerMiddleware struct {
	maxFailures  int
	resetTimeout time.Duration
	mu           sync.Mutex
	state        CircuitState
	failures     int
	lastFailTime time.Time
}

// NewCircuitBreakerMiddleware creates a new circuit breaker middleware.
func NewCircuitBreakerMiddleware(maxFailures int, resetTimeout time.Duration) *CircuitBreakerMiddleware {
	return &CircuitBreakerMiddleware{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
	}
}

// Wrap wraps the tool executor with circuit breaker logic.
func (m *CircuitBreakerMiddleware) Wrap(next tool.Executor) tool.Executor {
	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		m.mu.Lock()

		if m.state == StateOpen && time.Since(m.lastFailTime) > m.resetTimeout {
			m.state = StateHalfOpen
			m.failures = 0
		}

		// Reject if circuit is open
		if m.state == StateOpen {
			m.mu.Unlock()
			results := make([]tool.ExecutionResult, len(calls))
			for i := range calls {
				results[i] = tool.ExecutionResult{
					Error: fmt.Errorf("circuit breaker is open"),
				}
			}
			return results, nil
		}

		m.mu.Unlock()

		// Execute
		results, err := next.Execute(ctx, calls)
		if err != nil {
			m.recordFailure()
			return results, err
		}

		hasErrors := false
		for _, result := range results {
			if result.Error != nil {
				hasErrors = true
				break
			}
		}

		if hasErrors {
			m.recordFailure()
		} else {
			m.recordSuccess()
		}

		return results, nil
	})
}

// recordFailure records a failure and potentially opens the circuit.
func (m *CircuitBreakerMiddleware) recordFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failures++
	m.lastFailTime = time.Now()

	if m.failures >= m.maxFailures {
		m.state = StateOpen
	}
}

// recordSuccess records a success and potentially closes the circuit.
func (m *CircuitBreakerMiddleware) recordSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == StateHalfOpen {
		m.state = StateClosed
		m.failures = 0
	}
}

// State returns the current circuit state.
func (m *CircuitBreakerMiddleware) State() CircuitState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Reset resets the circuit breaker to closed state.
func (m *CircuitBreakerMiddleware) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = StateClosed
	m.failures = 0
}

// String returns a string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}
