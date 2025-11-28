package model

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"
)

// Router errors
var (
	// ErrNoModelAvailable is returned when no model can handle the request.
	ErrNoModelAvailable = errors.New("model: no model available")

	// ErrRoutingFailed is returned when routing fails.
	ErrRoutingFailed = errors.New("model: routing failed")
)

// Router selects an appropriate model for the given request.
// Implementations can base routing decisions on request content,
// metadata, cost constraints, capabilities, or any other criteria.
type Router interface {
	// Route returns the model to use for the given request.
	// Returns ErrNoModelAvailable if no suitable model can be found.
	Route(ctx context.Context, req *Request) (Model, error)
}

// RoutedModel wraps a Router to implement the Model interface.
// Each Generate call first routes to select the appropriate model,
// making routing transparent to callers.
type RoutedModel struct {
	router       Router
	fallback     Model
	capabilities Capabilities
	onRoute      func(ctx context.Context, req *Request, selected Model)
}

// RoutedModelOption configures a RoutedModel.
type RoutedModelOption func(*RoutedModel)

// WithFallbackModel sets a fallback model to use when routing fails.
func WithFallbackModel(m Model) RoutedModelOption {
	return func(rm *RoutedModel) {
		rm.fallback = m
	}
}

// WithRoutedCapabilities sets the capabilities exposed by the RoutedModel.
// By default, capabilities are empty. Set this to reflect the aggregate
// capabilities of all routable models.
func WithRoutedCapabilities(caps Capabilities) RoutedModelOption {
	return func(rm *RoutedModel) {
		rm.capabilities = caps
	}
}

// WithRouteCallback sets a callback invoked after routing decision.
// Useful for logging, metrics, or debugging routing decisions.
func WithRouteCallback(fn func(ctx context.Context, req *Request, selected Model)) RoutedModelOption {
	return func(rm *RoutedModel) {
		rm.onRoute = fn
	}
}

// NewRoutedModel creates a new RoutedModel that wraps a Router.
func NewRoutedModel(router Router, opts ...RoutedModelOption) *RoutedModel {
	rm := &RoutedModel{
		router: router,
	}
	for _, opt := range opts {
		opt(rm)
	}
	return rm
}

// Generate implements the Model interface by routing to an appropriate model.
func (m *RoutedModel) Generate(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		// Route to select model
		model, err := m.router.Route(ctx, req)
		if err != nil {
			// Try fallback if available
			if m.fallback != nil {
				model = m.fallback
			} else {
				yield(nil, fmt.Errorf("%w: %w", ErrRoutingFailed, err))
				return
			}
		}

		// Invoke callback if set
		if m.onRoute != nil {
			m.onRoute(ctx, req, model)
		}

		// Delegate to selected model's iterator
		for resp, err := range model.Generate(ctx, req) {
			if !yield(resp, err) {
				return
			}
		}
	}
}

// Capabilities returns the aggregate capabilities of routable models.
func (m *RoutedModel) Capabilities() Capabilities {
	return m.capabilities
}

// Router returns the underlying router.
func (m *RoutedModel) Router() Router {
	return m.router
}

// --- Circuit Breaker for Fallback Router ---

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed means the circuit is healthy and requests flow normally.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the circuit has tripped and requests are blocked.
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing if the model has recovered.
	CircuitHalfOpen
)

// CircuitBreaker tracks failures for a model and opens the circuit when threshold is exceeded.
type CircuitBreaker struct {
	mu           sync.Mutex
	failures     int
	successes    int
	lastFailure  time.Time
	state        CircuitState
	threshold    int
	resetTimeout time.Duration
	halfOpenMax  int // number of test requests in half-open state
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: resetTimeout,
		halfOpenMax:  1,
		state:        CircuitClosed,
	}
}

// IsOpen returns true if the circuit is open and requests should be blocked.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitOpen:
		// Check if reset timeout has elapsed
		if time.Since(cb.lastFailure) >= cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.successes = 0
			return false // Allow test request
		}
		return true
	case CircuitHalfOpen:
		return false // Allow test requests
	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.successes++
		if cb.successes >= cb.halfOpenMax {
			cb.state = CircuitClosed
			cb.failures = 0
		}
	} else {
		cb.failures = 0
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen {
		// Immediately trip back to open on failure in half-open state
		cb.state = CircuitOpen
	} else if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
}
