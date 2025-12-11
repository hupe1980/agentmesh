package graph

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// ManagedValueOption configures metadata or lifecycle hooks for a managed value.
type ManagedValueOption func(*managedValueConfig)

type managedValueConfig struct {
	descriptor checkpoint.ManagedValueDescriptor
	rehydrate  func(context.Context) error
}

// WithManagedValueRequired marks the managed value as required when resuming from
// checkpoints. Missing required values during restore will abort execution.
func WithManagedValueRequired() ManagedValueOption {
	return func(cfg *managedValueConfig) {
		cfg.descriptor.Required = true
	}
}

// WithManagedValueRehydrator registers a callback that rebuilds the managed
// value after checkpoint restores. The callback runs before execution resumes
// whenever the checkpoint lists this managed value. Use it to re-open network
// connections or refresh credentials.
func WithManagedValueRehydrator(fn func(context.Context) error) ManagedValueOption {
	return func(cfg *managedValueConfig) {
		cfg.rehydrate = fn
	}
}

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

	// Descriptor returns metadata stored in checkpoints for validation.
	Descriptor() checkpoint.ManagedValueDescriptor

	// Rehydrate refreshes the managed value after checkpoint restore. Default is
	// no-op. Implementations should be idempotent.
	Rehydrate(ctx context.Context) error
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
//	func myNode(ctx context.Context, view graph.ReadOnlyScope) (*graph.Command, error) {
//	    config := graph.GetManaged(ctx, view, configMV)
//	    // use config...
//	    return graph.Set(resultKey, result).End()
//	}
func GetManaged[T any](ctx context.Context, scope ReadOnlyScope, mv ManagedValueAccessor[T]) T {
	registry := scope.ManagedValues()
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
	name       string
	mu         sync.RWMutex
	value      T
	descriptor checkpoint.ManagedValueDescriptor
	rehydrate  func(context.Context) error
}

// NewManagedValue creates a managed value with a static value.
//
// Example:
//
//	configMV := graph.NewManagedValue("runtime_config", &Config{APIKey: "sk_live_..."})
//	timeoutMV := graph.NewManagedValue("timeout", 30*time.Second)
func NewManagedValue[T any](name string, value T, opts ...ManagedValueOption) *StaticManagedValue[T] {
	cfg := managedValueConfig{
		descriptor: checkpoint.ManagedValueDescriptor{
			Name: name,
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &StaticManagedValue[T]{
		name:       name,
		value:      value,
		descriptor: cfg.descriptor,
		rehydrate:  cfg.rehydrate,
	}
}

// Name returns the unique identifier for this managed value.
func (m *StaticManagedValue[T]) Name() string {
	return m.name
}

// Descriptor returns the checkpoint metadata for this managed value.
func (m *StaticManagedValue[T]) Descriptor() checkpoint.ManagedValueDescriptor {
	return m.descriptor
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

// Rehydrate executes the optional rehydration callback registered via options.
func (m *StaticManagedValue[T]) Rehydrate(ctx context.Context) error {
	if m.rehydrate == nil {
		return nil
	}
	return m.rehydrate(ctx)
}

// ManagedValueProvider computes its value dynamically with optional caching.
// Use this for derived state, expensive computations, or values that need refresh.
type ManagedValueProvider[T any] struct {
	name       string
	provider   func(ctx context.Context) (T, error)
	cacheTTL   time.Duration
	descriptor checkpoint.ManagedValueDescriptor
	rehydrate  func(context.Context) error

	mu        sync.RWMutex
	cached    T
	expiresAt time.Time
	hasCache  bool
}

// ManagedValueProviderOption configures a ManagedValueProvider.
type ManagedValueProviderOption func(*managedValueProviderConfig)

type managedValueProviderConfig struct {
	cacheTTL time.Duration
	options  []ManagedValueOption
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

// WithProviderManagedValueOptions applies generic managed value options when
// constructing a ManagedValueProvider (e.g., mark as required, register rehydrators).
func WithProviderManagedValueOptions(opts ...ManagedValueOption) ManagedValueProviderOption {
	return func(cfg *managedValueProviderConfig) {
		cfg.options = append(cfg.options, opts...)
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

	mvCfg := managedValueConfig{
		descriptor: checkpoint.ManagedValueDescriptor{
			Name: name,
		},
	}
	for _, opt := range cfg.options {
		opt(&mvCfg)
	}

	return &ManagedValueProvider[T]{
		name:       name,
		provider:   provider,
		cacheTTL:   cfg.cacheTTL,
		hasCache:   cfg.cacheTTL > 0,
		descriptor: mvCfg.descriptor,
		rehydrate:  mvCfg.rehydrate,
	}
}

// Name returns the unique identifier for this managed value.
func (p *ManagedValueProvider[T]) Name() string {
	return p.name
}

// Descriptor returns the checkpoint metadata for this managed value.
func (p *ManagedValueProvider[T]) Descriptor() checkpoint.ManagedValueDescriptor {
	return p.descriptor
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

// Rehydrate executes the optional rehydration callback for provider values.
func (p *ManagedValueProvider[T]) Rehydrate(ctx context.Context) error {
	if p.rehydrate == nil {
		return nil
	}
	return p.rehydrate(ctx)
}

// ManagedValueRegistry holds managed values for a graph execution (internal).
type ManagedValueRegistry struct {
	mu     sync.RWMutex
	values map[string]ManagedValue
}

// NewManagedValueRegistry creates a new registry for managed values.
func NewManagedValueRegistry() *ManagedValueRegistry {
	return &ManagedValueRegistry{
		values: make(map[string]ManagedValue),
	}
}

// register adds a managed value to the registry.
func (r *ManagedValueRegistry) register(mv ManagedValue) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[mv.Name()] = mv
}

// getByName retrieves a managed value by name.
func (r *ManagedValueRegistry) getByName(name string) ManagedValue {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.values[name]
}

// descriptors returns a sorted slice of managed value descriptors for storing in checkpoints.
func (r *ManagedValueRegistry) descriptors() []checkpoint.ManagedValueDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.values) == 0 {
		return nil
	}
	desc := make([]checkpoint.ManagedValueDescriptor, 0, len(r.values))
	for _, mv := range r.values {
		d := mv.Descriptor()
		if d.Name == "" {
			d.Name = mv.Name()
		}
		desc = append(desc, d)
	}
	sort.Slice(desc, func(i, j int) bool { return desc[i].Name < desc[j].Name })
	return desc
}

// ensureAndRehydrate validates that the registry satisfies the checkpoint descriptors
// and invokes rehydrators for matching managed values.
func (r *ManagedValueRegistry) ensureAndRehydrate(ctx context.Context, descriptors []checkpoint.ManagedValueDescriptor) error {
	if len(descriptors) == 0 {
		return nil
	}
	if r == nil {
		names := make([]string, len(descriptors))
		for i, d := range descriptors {
			names[i] = d.Name
		}
		sort.Strings(names)
		return &ManagedValueError{MissingValues: names, IsRequired: false}
	}

	var missing []string
	for _, desc := range descriptors {
		mv := r.getByName(desc.Name)
		if mv == nil {
			if desc.Required {
				missing = append(missing, desc.Name)
			}
			continue
		}

		if err := mv.Rehydrate(ctx); err != nil {
			return &RehydrateError{Name: desc.Name, Cause: err}
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return &ManagedValueError{MissingValues: missing, IsRequired: true}
	}

	return nil
}
