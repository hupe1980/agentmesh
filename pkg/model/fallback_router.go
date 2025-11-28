package model

import (
	"context"
	"time"
)

// FallbackRouter tries models in order until one succeeds.
// It implements a circuit breaker pattern to avoid repeatedly trying failed models.
type FallbackRouter struct {
	models           []Model
	breakers         []*CircuitBreaker
	failureThreshold int
	resetTimeout     time.Duration
}

// FallbackRouterOption configures a FallbackRouter.
type FallbackRouterOption func(*FallbackRouter)

// WithFailureThreshold sets the number of failures before tripping the circuit.
func WithFailureThreshold(threshold int) FallbackRouterOption {
	return func(r *FallbackRouter) {
		r.failureThreshold = threshold
	}
}

// WithResetTimeout sets the duration before a tripped circuit resets.
func WithResetTimeout(timeout time.Duration) FallbackRouterOption {
	return func(r *FallbackRouter) {
		r.resetTimeout = timeout
	}
}

// NewFallbackRouter creates a new fallback router with circuit breakers.
// Models are tried in order; the first available (non-tripped) model is returned.
func NewFallbackRouter(models []Model, opts ...FallbackRouterOption) *FallbackRouter {
	r := &FallbackRouter{
		models:           models,
		failureThreshold: 3,
		resetTimeout:     30 * time.Second,
	}
	for _, opt := range opts {
		opt(r)
	}

	// Create circuit breakers for each model
	r.breakers = make([]*CircuitBreaker, len(models))
	for i := range models {
		r.breakers[i] = NewCircuitBreaker(r.failureThreshold, r.resetTimeout)
	}

	return r
}

// Route returns the first available model (with non-open circuit).
func (r *FallbackRouter) Route(ctx context.Context, req *Request) (Model, error) {
	for i, model := range r.models {
		if !r.breakers[i].IsOpen() {
			return model, nil
		}
	}
	return nil, ErrNoModelAvailable
}

// RecordSuccess records a successful request for the given model.
func (r *FallbackRouter) RecordSuccess(m Model) {
	for i, model := range r.models {
		if model == m {
			r.breakers[i].RecordSuccess()
			return
		}
	}
}

// RecordFailure records a failed request for the given model.
func (r *FallbackRouter) RecordFailure(m Model) {
	for i, model := range r.models {
		if model == m {
			r.breakers[i].RecordFailure()
			return
		}
	}
}

// CircuitState returns the circuit breaker state for the given model.
func (r *FallbackRouter) CircuitState(m Model) CircuitState {
	for i, model := range r.models {
		if model == m {
			return r.breakers[i].State()
		}
	}
	return CircuitClosed
}

// ResetCircuit resets the circuit breaker for the given model.
func (r *FallbackRouter) ResetCircuit(m Model) {
	for i, model := range r.models {
		if model == m {
			r.breakers[i].Reset()
			return
		}
	}
}

// ResetAllCircuits resets all circuit breakers.
func (r *FallbackRouter) ResetAllCircuits() {
	for _, breaker := range r.breakers {
		breaker.Reset()
	}
}

// AvailableModels returns models with non-open circuits.
func (r *FallbackRouter) AvailableModels() []Model {
	var available []Model
	for i, model := range r.models {
		if !r.breakers[i].IsOpen() {
			available = append(available, model)
		}
	}
	return available
}

// FallbackRoutedModel extends RoutedModel to automatically record successes/failures.
// It wraps FallbackRouter and updates circuit breakers based on generation results.
type FallbackRoutedModel struct {
	*RoutedModel
	fallbackRouter *FallbackRouter
}

// NewFallbackRoutedModel creates a RoutedModel that automatically manages circuit breakers.
func NewFallbackRoutedModel(router *FallbackRouter, opts ...RoutedModelOption) *FallbackRoutedModel {
	rm := &FallbackRoutedModel{
		fallbackRouter: router,
	}

	// Wrap the router to track successes
	wrappedOpts := append([]RoutedModelOption{}, opts...)

	rm.RoutedModel = NewRoutedModel(router, wrappedOpts...)

	return rm
}

// RecordSuccess forwards to the fallback router.
func (m *FallbackRoutedModel) RecordSuccess(model Model) {
	m.fallbackRouter.RecordSuccess(model)
}

// RecordFailure forwards to the fallback router.
func (m *FallbackRoutedModel) RecordFailure(model Model) {
	m.fallbackRouter.RecordFailure(model)
}

// PriorityRouter routes to models based on priority order with health checking.
// Unlike FallbackRouter, it doesn't use circuit breakers but allows explicit health checks.
type PriorityRouter struct {
	models      []Model
	healthCheck func(ctx context.Context, m Model) bool
}

// PriorityRouterOption configures a PriorityRouter.
type PriorityRouterOption func(*PriorityRouter)

// WithHealthCheck sets a function to check if a model is healthy.
func WithHealthCheck(fn func(ctx context.Context, m Model) bool) PriorityRouterOption {
	return func(r *PriorityRouter) {
		r.healthCheck = fn
	}
}

// NewPriorityRouter creates a new priority-based router.
// Models are tried in order; the first healthy model is selected.
func NewPriorityRouter(models []Model, opts ...PriorityRouterOption) *PriorityRouter {
	r := &PriorityRouter{
		models: models,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Route returns the first healthy model in priority order.
func (r *PriorityRouter) Route(ctx context.Context, req *Request) (Model, error) {
	for _, model := range r.models {
		if r.healthCheck != nil {
			if !r.healthCheck(ctx, model) {
				continue
			}
		}
		return model, nil
	}
	return nil, ErrNoModelAvailable
}
