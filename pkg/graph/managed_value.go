package graph

import (
	"context"
	"sync"
	"time"
)

// ManagedValue is the base interface for all managed values.
// This non-generic interface allows managed values to be passed to WithManagedValues.
//
// Managed values represent ephemeral runtime state that is NOT included in checkpoints.
// Use managed values for:
//   - Runtime configuration (API keys, timeouts, feature flags)
//   - Session state (auth tokens, user sessions)
//   - Metrics collectors (runtime statistics)
//   - Resource handles (connections, caches)
//   - Computed values (derived state)
//
// Unlike regular state keys, managed values:
//   - Are NOT persisted to checkpoints
//   - Are lost on process restart
//   - Must be reinitialized at runtime
//   - Are perfect for ephemeral/sensitive data
type ManagedValue interface {
	// Name returns the unique identifier for this managed value.
	Name() string
}

// ManagedValueAccessor is the generic interface for type-safe access to managed values.
type ManagedValueAccessor[T any] interface {
	ManagedValue

	// Get retrieves the current value.
	Get(ctx context.Context) (T, error)

	// Set updates the value (for mutable managed values).
	Set(ctx context.Context, value T) error
}

// GetManaged retrieves a managed value from the view with type safety.
// The managed value must have been passed via WithManagedValues.
//
// This follows the same pattern as Get(view, key) for regular state.
//
// Example:
//
//	var configMV = graph.NewManagedValue("config", defaultConfig)
//
//	func myNode(ctx context.Context, view graph.View) (*graph.Command, error) {
//	    config := graph.GetManaged(ctx, view, configMV)
//	    // use config...
//	    return graph.Set(resultKey, result).End()
//	}
func GetManaged[T any](ctx context.Context, view View, mv ManagedValueAccessor[T]) T {
	registry := view.ManagedValues()
	if registry == nil {
		var zero T
		return zero
	}

	// Look up the registered managed value by name
	stored := registry.getByName(mv.Name())
	if stored == nil {
		var zero T
		return zero
	}

	// Type assert to the correct ManagedValueAccessor type
	typedMV, ok := stored.(ManagedValueAccessor[T])
	if !ok {
		var zero T
		return zero
	}

	val, _ := typedMV.Get(ctx)
	return val
}

// StaticManagedValue provides thread-safe storage for ephemeral values.
// Values are stored in memory and NOT checkpointed.
type StaticManagedValue[T any] struct {
	name  string
	mu    sync.RWMutex
	value T
}

// NewManagedValue creates a managed value with a static value.
//
// Example:
//
//	configMV := graph.NewManagedValue("runtime_config", &Config{APIKey: "sk_live_..."})
//	timeoutMV := graph.NewManagedValue("timeout", 30*time.Second)
func NewManagedValue[T any](name string, value T) *StaticManagedValue[T] {
	return &StaticManagedValue[T]{
		name:  name,
		value: value,
	}
}

// Name returns the unique identifier for this managed value.
func (m *StaticManagedValue[T]) Name() string {
	return m.name
}

// Get retrieves the current value.
func (m *StaticManagedValue[T]) Get(_ context.Context) (T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.value, nil
}

// Set updates the stored value.
func (m *StaticManagedValue[T]) Set(_ context.Context, value T) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.value = value
	return nil
}

// ManagedValueProvider computes its value dynamically with optional caching.
// Use this for derived state, expensive computations, or values that need refresh.
type ManagedValueProvider[T any] struct {
	name     string
	provider func(ctx context.Context) (T, error)
	cacheTTL time.Duration

	mu        sync.RWMutex
	cached    T
	expiresAt time.Time
	hasCache  bool
}

// ManagedValueProviderOption configures a ManagedValueProvider.
type ManagedValueProviderOption func(*managedValueProviderConfig)

type managedValueProviderConfig struct {
	cacheTTL time.Duration
}

// WithCacheTTL enables caching with the specified TTL.
// Without this option, the provider is called on every access.
//
// Example:
//
//	// Cache for 5 minutes
//	mv := graph.NewManagedValueProvider("config", fetchConfig, graph.WithCacheTTL(5*time.Minute))
func WithCacheTTL(ttl time.Duration) ManagedValueProviderOption {
	return func(cfg *managedValueProviderConfig) {
		cfg.cacheTTL = ttl
	}
}

// NewManagedValueProvider creates a managed value that computes its value dynamically.
// By default, the provider is called on every access. Use WithCacheTTL to enable caching.
//
// Example:
//
//	// Always fresh (no caching)
//	currentTimeMV := graph.NewManagedValueProvider("current_time", func(ctx context.Context) (time.Time, error) {
//	    return time.Now(), nil
//	})
//
//	// With caching (refreshes every 5 seconds)
//	cachedTimeMV := graph.NewManagedValueProvider("cached_time", func(ctx context.Context) (time.Time, error) {
//	    return time.Now(), nil
//	}, graph.WithCacheTTL(5*time.Second))
func NewManagedValueProvider[T any](name string, provider func(ctx context.Context) (T, error), opts ...ManagedValueProviderOption) *ManagedValueProvider[T] {
	cfg := &managedValueProviderConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return &ManagedValueProvider[T]{
		name:     name,
		provider: provider,
		cacheTTL: cfg.cacheTTL,
		hasCache: cfg.cacheTTL > 0,
	}
}

// Name returns the unique identifier for this managed value.
func (p *ManagedValueProvider[T]) Name() string {
	return p.name
}

// Get retrieves the value, using cache if enabled and not expired.
func (p *ManagedValueProvider[T]) Get(ctx context.Context) (T, error) {
	// No caching - always compute fresh
	if !p.hasCache {
		return p.provider(ctx)
	}

	// Check cache
	p.mu.RLock()
	if time.Now().Before(p.expiresAt) {
		cached := p.cached
		p.mu.RUnlock()
		return cached, nil
	}
	p.mu.RUnlock()

	// Cache miss or expired - compute fresh
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Now().Before(p.expiresAt) {
		return p.cached, nil
	}

	value, err := p.provider(ctx)
	if err != nil {
		return p.cached, err
	}

	p.cached = value
	p.expiresAt = time.Now().Add(p.cacheTTL)
	return value, nil
}

// Set is a no-op for provider values (they are computed).
func (p *ManagedValueProvider[T]) Set(_ context.Context, _ T) error {
	return nil
}

// Invalidate clears the cache, forcing a refresh on next Get.
// This is a no-op if caching is not enabled.
func (p *ManagedValueProvider[T]) Invalidate() {
	if !p.hasCache {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.expiresAt = time.Time{}
}

// managedValueRegistry holds managed values for a graph execution (internal).
type managedValueRegistry struct {
	mu     sync.RWMutex
	values map[string]ManagedValue
}

// newManagedValueRegistry creates a new registry for managed values.
func newManagedValueRegistry() *managedValueRegistry {
	return &managedValueRegistry{
		values: make(map[string]ManagedValue),
	}
}

// register adds a managed value to the registry.
func (r *managedValueRegistry) register(mv ManagedValue) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[mv.Name()] = mv
}

// getByName retrieves a managed value by name.
func (r *managedValueRegistry) getByName(name string) ManagedValue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.values[name]
}
