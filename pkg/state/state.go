package state

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"sync"
)

// State is the central state container with type-safe operations.
// Designed for BSP execution: synchronous updates, concurrent reads.
type State struct {
	// Internal: Direct mutation protected by RWMutex
	data       map[string]any
	mu         sync.RWMutex
	version    uint64
	registered map[string]reflect.Type
}

// NewState creates a new mutable state container.
func NewState() *State {
	return &State{
		data:       make(map[string]any),
		registered: make(map[string]reflect.Type),
	}
}

// Register adds a key to the registry with type validation.
// Must be called before using the key (typically at graph construction time).
func Register[T any](s *State, key Key[T]) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var zero T
	expected := reflect.TypeOf(zero)
	
	// Handle interface types
	if expected == nil {
		expected = reflect.TypeOf((*T)(nil)).Elem()
	}

	if existing, ok := s.registered[key.name]; ok {
		if existing != expected {
			return fmt.Errorf("key %q already registered with different type: %v vs %v",
				key.name, existing, expected)
		}
		return nil // Already registered with same type
	}

	s.registered[key.name] = expected
	// Initialize with zero value
	s.data[key.name] = key.zero
	return nil
}

// Get retrieves a typed value - COMPILE-TIME TYPE SAFETY.
// Concurrent-safe: Multiple vertices can read during BSP superstep.
func Get[T any](s *State, key Key[T]) T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.data[key.name]
	if !ok {
		return key.zero
	}
	return val.(T) // Safe: key registration enforces type
}

// Set updates or creates a typed value.
// BSP-safe: Called between supersteps when no concurrent reads.
// Returns error if key validation fails or context cancelled.
func Set[T any](ctx context.Context, s *State, key Key[T], value T) error {
	// Check context first
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate key is registered
	expectedType, ok := s.registered[key.name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrKeyNotRegistered, key.name)
	}

	valueType := reflect.TypeOf(value)
	if valueType != expectedType {
		return fmt.Errorf("%w: key %q expected %v, got %v",
			ErrTypeMismatch, key.name, expectedType, valueType)
	}

	s.data[key.name] = value
	s.version++
	return nil
}

// Append adds to a list (type-safe).
// BSP-safe: Called between supersteps.
// Returns error if list doesn't exist or context cancelled.
func Append[T any](ctx context.Context, s *State, key ListKey[T], value T) error {
	// Check context first
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate key is registered
	if _, ok := s.registered[key.name]; !ok {
		return fmt.Errorf("%w: %s", ErrKeyNotRegistered, key.name)
	}

	existing := s.data[key.name]
	var list []T
	if existing != nil {
		var ok bool
		list, ok = existing.([]T)
		if !ok {
			return fmt.Errorf("%w: %s", ErrKeyNotList, key.name)
		}
	}

	list = append(list, value)

	// Enforce max size
	if key.maxSize > 0 && len(list) > key.maxSize {
		list = list[len(list)-key.maxSize:]
	}

	s.data[key.name] = list
	s.version++
	return nil
}

// ApplyUpdates applies a batch of updates atomically.
// BSP Usage: Called between supersteps to apply all vertex results.
func (s *State) ApplyUpdates(ctx context.Context, updates map[string]any) error {
	// Check context first
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate all keys first (all-or-nothing)
	for key, value := range updates {
		if expectedType, ok := s.registered[key]; ok {
			valueType := reflect.TypeOf(value)
			if valueType != expectedType {
				return fmt.Errorf("%w: key %q expected %v, got %v",
					ErrTypeMismatch, key, expectedType, valueType)
			}
		} else {
			return fmt.Errorf("%w: %s", ErrKeyNotRegistered, key)
		}
	}

	// Apply all updates
	for key, value := range updates {
		s.data[key] = value
	}
	s.version++

	return nil
}

// Snapshot returns immutable point-in-time view.
// BSP Usage: Called at start of superstep to give vertices consistent view.
func (s *State) Snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create immutable copy
	data := make(map[string]any, len(s.data))
	for k, v := range s.data {
		data[k] = v
	}

	return &Snapshot{
		data:    data,
		version: s.version,
	}
}

// Stream provides zero-allocation iteration over a list.
// Concurrent-safe: Acquires read lock for iteration.
func Stream[T any](s *State, key ListKey[T]) iter.Seq[T] {
	return func(yield func(T) bool) {
		list := Get(s, key.Key)
		for _, item := range list {
			if !yield(item) {
				return
			}
		}
	}
}

// Version returns monotonic version counter.
func (s *State) Version() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// MustSet panics on error (for initialization code only).
func MustSet[T any](ctx context.Context, s *State, key Key[T], value T) {
	if err := Set(ctx, s, key, value); err != nil {
		panic(fmt.Sprintf("state.MustSet: %v", err))
	}
}
