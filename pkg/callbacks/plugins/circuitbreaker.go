package plugins

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// CircuitState represents the current state of the circuit breaker.
type CircuitState int

const (
	// CircuitClosed means requests are flowing normally
	CircuitClosed CircuitState = iota
	// CircuitOpen means the circuit is open and requests are blocked
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing if the service has recovered
	CircuitHalfOpen
)

// CircuitBreakerPlugin implements the circuit breaker pattern to prevent
// cascading failures. It opens the circuit after a threshold of failures
// and periodically allows test requests to check if the service has recovered.
type CircuitBreakerPlugin struct {
	callbacks.NoopPlugin

	maxFailures   int
	resetTimeout  time.Duration
	halfOpenLimit int

	mu               sync.Mutex
	state            CircuitState
	failures         atomic.Int64
	lastFailureTime  time.Time
	halfOpenAttempts int
}

// NewCircuitBreakerPlugin creates a circuit breaker plugin.
// maxFailures is the number of consecutive failures before opening the circuit.
// resetTimeout is how long to wait before transitioning from open to half-open.
// halfOpenLimit is the number of test requests allowed in half-open state.
func NewCircuitBreakerPlugin(maxFailures int, resetTimeout time.Duration, halfOpenLimit int) *CircuitBreakerPlugin {
	return &CircuitBreakerPlugin{
		maxFailures:   maxFailures,
		resetTimeout:  resetTimeout,
		halfOpenLimit: halfOpenLimit,
		state:         CircuitClosed,
	}
}

// BeforeModel checks circuit breaker state before model invocation.
func (p *CircuitBreakerPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.state {
	case CircuitOpen:
		// Check if enough time has passed to try half-open
		if time.Since(p.lastFailureTime) > p.resetTimeout {
			p.state = CircuitHalfOpen
			p.halfOpenAttempts = 0
		} else {
			return nil, fmt.Errorf("circuit breaker is open (failures: %d)", p.failures.Load())
		}

	case CircuitHalfOpen:
		// Limit test requests in half-open state
		if p.halfOpenAttempts >= p.halfOpenLimit {
			return nil, fmt.Errorf("circuit breaker is half-open, test limit reached")
		}
		p.halfOpenAttempts++
	}

	return nil, nil
}

// AfterModel records successful model invocation.
func (p *CircuitBreakerPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Success - reset failures or close circuit
	p.failures.Store(0)

	if p.state == CircuitHalfOpen {
		p.state = CircuitClosed
		p.halfOpenAttempts = 0
	}

	return nil, nil
}

// OnModelError records failed invocation and updates circuit state.
func (p *CircuitBreakerPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Increment failure count
	failures := p.failures.Add(1)
	p.lastFailureTime = time.Now()

	// Check if we should open the circuit
	if failures >= int64(p.maxFailures) {
		p.state = CircuitOpen
	}

	return nil, err
}

// GetState returns the current circuit state.
func (p *CircuitBreakerPlugin) GetState() CircuitState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Reset manually resets the circuit breaker to closed state.
func (p *CircuitBreakerPlugin) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.state = CircuitClosed
	p.failures.Store(0)
	p.halfOpenAttempts = 0
}
