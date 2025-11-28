package model

import (
	"context"
)

// CompositeRouter chains multiple routers together.
// Each router is tried in order; the first to return a model wins.
// This implements the Chain of Responsibility pattern.
type CompositeRouter struct {
	routers  []Router
	fallback Model
}

// CompositeRouterOption configures a CompositeRouter.
type CompositeRouterOption func(*CompositeRouter)

// WithCompositeFallback sets a fallback model when all routers fail.
func WithCompositeFallback(m Model) CompositeRouterOption {
	return func(r *CompositeRouter) {
		r.fallback = m
	}
}

// NewCompositeRouter creates a new composite router.
// Routers are tried in order until one returns a valid model.
//
// Example:
//
//	router := NewCompositeRouter(
//	    NewCapabilityRouter(models),  // First check capabilities
//	    NewCostBasedRouter(cheap, expensive),  // Then optimize for cost
//	    NewFallbackRouter(allModels),  // Finally, fallback chain
//	)
func NewCompositeRouter(routers []Router, opts ...CompositeRouterOption) *CompositeRouter {
	r := &CompositeRouter{
		routers: routers,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Route tries each router in order until one succeeds.
func (r *CompositeRouter) Route(ctx context.Context, req *Request) (Model, error) {
	for _, router := range r.routers {
		model, err := router.Route(ctx, req)
		if err == nil && model != nil {
			return model, nil
		}
		// Continue to next router on error
	}

	if r.fallback != nil {
		return r.fallback, nil
	}

	return nil, ErrNoModelAvailable
}

// Add appends a router to the chain.
func (r *CompositeRouter) Add(router Router) {
	r.routers = append(r.routers, router)
}

// Routers returns the list of routers in the chain.
func (r *CompositeRouter) Routers() []Router {
	return r.routers
}

// ConditionalRouter routes based on a condition function.
// If the condition is true, it uses the primary router; otherwise, the alternative.
type ConditionalRouter struct {
	condition   func(ctx context.Context, req *Request) bool
	primary     Router
	alternative Router
}

// NewConditionalRouter creates a router that chooses based on a condition.
func NewConditionalRouter(
	condition func(ctx context.Context, req *Request) bool,
	primary Router,
	alternative Router,
) *ConditionalRouter {
	return &ConditionalRouter{
		condition:   condition,
		primary:     primary,
		alternative: alternative,
	}
}

// Route evaluates the condition and delegates to the appropriate router.
func (r *ConditionalRouter) Route(ctx context.Context, req *Request) (Model, error) {
	if r.condition(ctx, req) {
		return r.primary.Route(ctx, req)
	}
	return r.alternative.Route(ctx, req)
}

// StaticRouter always returns the same model.
// Useful as a simple fallback or for testing.
type StaticRouter struct {
	model Model
}

// NewStaticRouter creates a router that always returns the given model.
func NewStaticRouter(m Model) *StaticRouter {
	return &StaticRouter{model: m}
}

// Route always returns the static model.
func (r *StaticRouter) Route(ctx context.Context, req *Request) (Model, error) {
	if r.model == nil {
		return nil, ErrNoModelAvailable
	}
	return r.model, nil
}

// RandomRouter randomly selects from a pool of models.
// Useful for load balancing or A/B testing.
type RandomRouter struct {
	models []Model
	rand   func(n int) int
}

// RandomRouterOption configures a RandomRouter.
type RandomRouterOption func(*RandomRouter)

// WithRandomFunc sets a custom random function.
func WithRandomFunc(fn func(n int) int) RandomRouterOption {
	return func(r *RandomRouter) {
		r.rand = fn
	}
}

// NewRandomRouter creates a router that randomly selects a model.
func NewRandomRouter(models []Model, opts ...RandomRouterOption) *RandomRouter {
	r := &RandomRouter{
		models: models,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Route randomly selects a model from the pool.
func (r *RandomRouter) Route(ctx context.Context, req *Request) (Model, error) {
	if len(r.models) == 0 {
		return nil, ErrNoModelAvailable
	}

	var idx int
	if r.rand != nil {
		idx = r.rand(len(r.models))
	} else {
		// Default: first model (for deterministic testing)
		idx = 0
	}

	return r.models[idx], nil
}

// WeightedRouter selects models based on weights.
// Higher weights mean higher probability of selection.
type WeightedRouter struct {
	models      []Model
	weights     []int
	totalWeight int
	rand        func(n int) int
}

// WeightedRouterOption configures a WeightedRouter.
type WeightedRouterOption func(*WeightedRouter)

// WithWeightedRandomFunc sets a custom random function for weighted selection.
func WithWeightedRandomFunc(fn func(n int) int) WeightedRouterOption {
	return func(r *WeightedRouter) {
		r.rand = fn
	}
}

// NewWeightedRouter creates a router that selects based on weights.
// weights[i] corresponds to models[i]. Higher weight = more likely selection.
func NewWeightedRouter(models []Model, weights []int, opts ...WeightedRouterOption) *WeightedRouter {
	if len(models) != len(weights) {
		panic("models and weights must have same length")
	}

	total := 0
	for _, w := range weights {
		total += w
	}

	r := &WeightedRouter{
		models:      models,
		weights:     weights,
		totalWeight: total,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Route selects a model based on weights.
func (r *WeightedRouter) Route(ctx context.Context, req *Request) (Model, error) {
	if len(r.models) == 0 || r.totalWeight == 0 {
		return nil, ErrNoModelAvailable
	}

	var target int
	if r.rand != nil {
		target = r.rand(r.totalWeight)
	} else {
		target = 0 // Default: first model
	}

	cumulative := 0
	for i, weight := range r.weights {
		cumulative += weight
		if target < cumulative {
			return r.models[i], nil
		}
	}

	// Fallback to last model
	return r.models[len(r.models)-1], nil
}

// RouterFunc is an adapter to use a function as a Router.
type RouterFunc func(ctx context.Context, req *Request) (Model, error)

// Route calls the function.
func (f RouterFunc) Route(ctx context.Context, req *Request) (Model, error) {
	return f(ctx, req)
}
