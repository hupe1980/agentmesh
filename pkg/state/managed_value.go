package state

import (
	"context"
	"fmt"
	"sync"
)

// ManagedValue represents ephemeral runtime state that is NOT included in checkpoints.
// Unlike channels (which are persisted), managed values are runtime-only state such as:
//   - Configuration stores
//   - User sessions and authentication state
//   - Connection pools and resource handles
//   - Runtime metrics collectors
//   - Temporary caches
//
// Type-safe access is provided through the generic Get method.
type ManagedValue[T any] interface {
	// Name returns the unique identifier for this managed value
	Name() string

	// Get retrieves the current value
	Get(ctx context.Context) (T, error)

	// Set updates the value (runtime-managed, not from nodes)
	Set(ctx context.Context, value T) error
}

// SimpleManagedValue is a basic thread-safe implementation of ManagedValue.
// It stores a single value with mutex protection for concurrent access.
type SimpleManagedValue[T any] struct {
	name  string
	mu    sync.RWMutex
	value T
	isSet bool
}

// NewManagedValue creates a new thread-safe managed value with the given name.
func NewManagedValue[T any](name string) *SimpleManagedValue[T] {
	return &SimpleManagedValue[T]{
		name: name,
	}
}

// NewManagedValueWithDefault creates a new managed value initialized with a default value.
func NewManagedValueWithDefault[T any](name string, defaultValue T) *SimpleManagedValue[T] {
	return &SimpleManagedValue[T]{
		name:  name,
		value: defaultValue,
		isSet: true,
	}
}

// Name returns the managed value's identifier.
func (m *SimpleManagedValue[T]) Name() string {
	return m.name
}

// Get retrieves the current value (thread-safe read).
func (m *SimpleManagedValue[T]) Get(ctx context.Context) (T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isSet {
		var zero T
		return zero, fmt.Errorf("managed value %q has not been set", m.name)
	}

	return m.value, nil
}

// Set updates the value (thread-safe write).
func (m *SimpleManagedValue[T]) Set(ctx context.Context, value T) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.value = value
	m.isSet = true

	return nil
}

// ComputedManagedValue is a managed value computed on-demand from a function.
// Useful for derived state that doesn't need storage (e.g., current timestamp, metrics).
type ComputedManagedValue[T any] struct {
	name        string
	computeFunc func(ctx context.Context) (T, error)
}

// NewComputedManagedValue creates a managed value that computes its value on each Get().
func NewComputedManagedValue[T any](name string, computeFunc func(ctx context.Context) (T, error)) *ComputedManagedValue[T] {
	return &ComputedManagedValue[T]{
		name:        name,
		computeFunc: computeFunc,
	}
}

// Name returns the managed value's identifier.
func (c *ComputedManagedValue[T]) Name() string {
	return c.name
}

// Get computes and returns the value.
func (c *ComputedManagedValue[T]) Get(ctx context.Context) (T, error) {
	return c.computeFunc(ctx)
}

// Set is not supported for computed values.
func (c *ComputedManagedValue[T]) Set(ctx context.Context, value T) error {
	return fmt.Errorf("cannot set computed managed value %q", c.name)
}

// CachedManagedValue wraps another ManagedValue with caching.
// The cached value is invalidated after the specified duration.
type CachedManagedValue[T any] struct {
	name        string
	source      ManagedValue[T]
	mu          sync.RWMutex
	cachedValue T
	lastFetch   int64 // Unix timestamp
	cacheTTL    int64 // Seconds
	isSet       bool
}

// NewCachedManagedValue creates a managed value that caches another managed value's result.
func NewCachedManagedValue[T any](name string, source ManagedValue[T], cacheTTLSeconds int64) *CachedManagedValue[T] {
	return &CachedManagedValue[T]{
		name:     name,
		source:   source,
		cacheTTL: cacheTTLSeconds,
	}
}

// Name returns the managed value's identifier.
func (c *CachedManagedValue[T]) Name() string {
	return c.name
}

// Get retrieves the value, using cache if valid.
func (c *CachedManagedValue[T]) Get(ctx context.Context) (T, error) {
	now := ctx.Value("timestamp")
	if now == nil {
		// Fallback: use current time if not in context
		now = int64(0) // This would need actual time implementation
	}
	timestamp := now.(int64)

	// Check cache validity (read lock)
	c.mu.RLock()
	if c.isSet && (timestamp-c.lastFetch) < c.cacheTTL {
		value := c.cachedValue
		c.mu.RUnlock()
		return value, nil
	}
	c.mu.RUnlock()

	// Cache miss or expired - fetch new value (write lock)
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.isSet && (timestamp-c.lastFetch) < c.cacheTTL {
		return c.cachedValue, nil
	}

	// Fetch from source
	value, err := c.source.Get(ctx)
	if err != nil {
		var zero T
		return zero, err
	}

	c.cachedValue = value
	c.lastFetch = timestamp
	c.isSet = true

	return value, nil
}

// Set updates the underlying source and invalidates cache.
func (c *CachedManagedValue[T]) Set(ctx context.Context, value T) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.source.Set(ctx, value); err != nil {
		return err
	}

	// Invalidate cache
	c.isSet = false

	return nil
}

// managedValueAny is a type-erased wrapper for storing ManagedValue[T] in a map.
// This allows the Manager to store managed values of different types in a single map.
type managedValueAny struct {
	name string
	get  func(ctx context.Context) (any, error)
	set  func(ctx context.Context, value any) error
}

// WrapManagedValue wraps a typed ManagedValue[T] into a type-erased managedValueAny.
// This allows heterogeneous managed values to be stored in a single map.
func WrapManagedValue[T any](mv ManagedValue[T]) *managedValueAny {
	return &managedValueAny{
		name: mv.Name(),
		get: func(ctx context.Context) (any, error) {
			return mv.Get(ctx)
		},
		set: func(ctx context.Context, value any) error {
			typed, ok := value.(T)
			if !ok {
				var zero T
				return fmt.Errorf("type mismatch: expected %T, got %T", zero, value)
			}
			return mv.Set(ctx, typed)
		},
	}
}

// Name returns the managed value's identifier.
func (m *managedValueAny) Name() string {
	return m.name
}

// Get retrieves the value (type-erased).
func (m *managedValueAny) Get(ctx context.Context) (any, error) {
	return m.get(ctx)
}

// Set updates the value (type-erased with runtime type checking).
func (m *managedValueAny) Set(ctx context.Context, value any) error {
	return m.set(ctx, value)
}
