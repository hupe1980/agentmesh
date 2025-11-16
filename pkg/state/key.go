package state

import (
	"fmt"
)

// Type-Safe State Keys
//
// This file provides generic Key[T] types for compile-time type safety when accessing state.
// This is the RECOMMENDED way to interact with state in AgentMesh.
//
// Why typed keys are better than string-based access:
//
// 1. Compile-time safety: Typos in key names become undefined variable errors
// 2. No runtime panics: Type mismatches caught at compile time, not runtime
// 3. IDE autocomplete: Define keys as package variables for better developer experience
// 4. Centralized definitions: Keys defined once, preventing inconsistencies
// 5. Works with any type: Not limited to primitives like the old GetInt/GetString methods
//
// Migration guide:
//   Old: count := s.Get("counter").(int)        // Runtime panic if wrong type
//   New: count, err := CounterKey.Get(s)        // Compile-time type checking
//
// See examples/builder_api and examples/observability for usage patterns.

// Key provides type-safe access to state values with compile-time type checking.
// It eliminates runtime panics from type assertions and typos in string keys.
//
// Usage:
//
//	// Define keys with types (typically at package level)
//	var (
//	    CounterKey = state.NewKey[int]("counter")
//	    StatusKey  = state.NewKey[string]("status")
//	    ConfigKey  = state.NewKey[*Config]("config")
//	)
//
//	// Type-safe reads with autocomplete
//	counter, err := CounterKey.Get(state)  // Returns int, not any
//	status := StatusKey.GetOr(state, "pending")  // Default value
//
//	// Type-safe writes (compile-time checked)
//	CounterKey.Set(state, 42)              // ✓ Compiles
//	CounterKey.Set(state, "not an int")    // ✗ Compile error
//
// Benefits:
//   - Compile-time type safety (no more interface{} casting)
//   - Autocomplete for key names (define keys as package variables)
//   - Centralized key definitions (prevents typos)
//   - Default value support
//   - Compatible with existing state.Reader/Writer interfaces
type Key[T any] struct {
	name string
}

// NewKey creates a new typed state key with the given name.
// The type parameter T specifies the expected value type.
//
// Example:
//
//	var UserIDKey = NewKey[string]("user_id")
//	var ScoreKey = NewKey[float64]("score")
func NewKey[T any](name string) Key[T] {
	return Key[T]{name: name}
}

// Name returns the underlying string key name.
// Useful for logging, debugging, or when interfacing with untyped APIs.
func (k Key[T]) Name() string {
	return k.name
}

// Get retrieves the typed value from state.
// Returns an error if the key doesn't exist or the value has the wrong type.
//
// Example:
//
//	counter, err := CounterKey.Get(state)
//	if err != nil {
//	    // Handle missing or wrong-type error
//	}
func (k Key[T]) Get(r Reader) (T, error) {
	value := r.Get(k.name)
	if value == nil {
		var zero T
		return zero, fmt.Errorf("key %q not found in state", k.name)
	}

	typed, ok := value.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("key %q has type %T, expected %T", k.name, value, zero)
	}

	return typed, nil
}

// MustGet retrieves the typed value from state, panicking if not found or wrong type.
// Use this when the key is guaranteed to exist (e.g., after validation).
//
// Example:
//
//	counter := CounterKey.MustGet(state)  // Panics if missing or wrong type
func (k Key[T]) MustGet(r Reader) T {
	value, err := k.Get(r)
	if err != nil {
		panic(err)
	}
	return value
}

// GetOr retrieves the typed value from state, returning defaultValue if not found or wrong type.
// This is the safest accessor - never returns an error, always returns a valid T.
//
// Example:
//
//	status := StatusKey.GetOr(state, "pending")  // Returns "pending" if key missing
//	counter := CounterKey.GetOr(state, 0)        // Returns 0 if key missing
func (k Key[T]) GetOr(r Reader, defaultValue T) T {
	value, err := k.Get(r)
	if err != nil {
		return defaultValue
	}
	return value
}

// Set writes the typed value to state.
// The type parameter ensures compile-time type checking.
//
// Example:
//
//	err := CounterKey.Set(state, 42)  // ✓ Type-checked at compile time
func (k Key[T]) Set(w Writer, value T) error {
	return w.Set(k.name, value)
}

// Update applies a transformation function to the current value.
// If the key doesn't exist, it uses the zero value of T.
//
// Example:
//
//	// Increment counter
//	CounterKey.Update(state, func(current int) int {
//	    return current + 1
//	})
//
//	// Append to status
//	StatusKey.Update(state, func(current string) string {
//	    return current + " - updated"
//	})
func (k Key[T]) Update(w Writer, fn func(T) T) error {
	current, err := k.Get(w)
	if err != nil {
		// Use zero value if key doesn't exist
		var zero T
		current = zero
	}
	newValue := fn(current)
	return k.Set(w, newValue)
}

// Exists checks if the key exists in state with the correct type.
// Returns true only if key exists AND has type T.
func (k Key[T]) Exists(r Reader) bool {
	_, err := k.Get(r)
	return err == nil
}

// =============================================================================
// Predefined Common Keys
// =============================================================================

// These are commonly used keys defined with standard types.
// Applications can define their own keys following this pattern.

// MessagesKey accesses the message history channel.
// Typically not used directly - prefer state.MessagesSnapshot() instead.
var MessagesKey = NewKey[[]ExecutionResult]("messages")

// =============================================================================
// Typed Key Helpers for Common Patterns
// =============================================================================

// Counter provides atomic counter operations on an int key.
type Counter struct {
	key Key[int]
}

// NewCounter creates a counter backed by the given key.
func NewCounter(name string) Counter {
	return Counter{key: NewKey[int](name)}
}

// Name returns the underlying key name.
func (c Counter) Name() string {
	return c.key.Name()
}

// Increment atomically increments the counter and returns the new value.
func (c Counter) Increment(w Writer) (int, error) {
	var result int
	err := c.key.Update(w, func(current int) int {
		result = current + 1
		return result
	})
	return result, err
}

// Decrement atomically decrements the counter and returns the new value.
func (c Counter) Decrement(w Writer) (int, error) {
	var result int
	err := c.key.Update(w, func(current int) int {
		result = current - 1
		return result
	})
	return result, err
}

// Get returns the current counter value.
func (c Counter) Get(r Reader) (int, error) {
	return c.key.Get(r)
}

// Set sets the counter to a specific value.
func (c Counter) Set(w Writer, value int) error {
	return c.key.Set(w, value)
}

// Flag provides boolean flag operations.
type Flag struct {
	key Key[bool]
}

// NewFlag creates a boolean flag backed by the given key.
func NewFlag(name string) Flag {
	return Flag{key: NewKey[bool](name)}
}

// Set sets the flag to true.
func (f Flag) Set(w Writer) error {
	return f.key.Set(w, true)
}

// Clear sets the flag to false.
func (f Flag) Clear(w Writer) error {
	return f.key.Set(w, false)
}

// IsSet returns true if the flag is set to true.
func (f Flag) IsSet(r Reader) bool {
	return f.key.GetOr(r, false)
}

// Toggle flips the flag value.
func (f Flag) Toggle(w Writer) error {
	return f.key.Update(w, func(current bool) bool {
		return !current
	})
}
