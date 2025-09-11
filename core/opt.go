package core

import (
	"encoding/json"
	"fmt"
)

// Opt represents an optional value of type T.
// Zero value = unset (None).
type Opt[T any] struct {
	set   bool
	value T
}

// Some creates an Opt[T] with a value set.
func Some[T any](v T) Opt[T] {
	return Opt[T]{set: true, value: v}
}

// None creates an Opt[T] with no value set.
// Usually not needed; zero value behaves the same.
func None[T any]() Opt[T] {
	var zero T
	return Opt[T]{set: false, value: zero}
}

// IsSet returns true if a value is set.
func (o Opt[T]) IsSet() bool {
	return o.set
}

// Get returns the value and whether it is set.
func (o Opt[T]) Get() (T, bool) {
	return o.value, o.set
}

// Or returns the value if set, otherwise returns defaultVal.
func (o Opt[T]) Or(defaultVal T) T {
	if o.set {
		return o.value
	}
	return defaultVal
}

// Set assigns a new value and marks it as set.
func (o *Opt[T]) Set(v T) {
	o.value = v
	o.set = true
}

// Clear resets the option to None.
func (o *Opt[T]) Clear() {
	var zero T
	o.value = zero
	o.set = false
}

// MarshalJSON encodes the value if set, otherwise null.
func (o Opt[T]) MarshalJSON() ([]byte, error) {
	if !o.set {
		return []byte("null"), nil
	}
	return json.Marshal(o.value)
}

// UnmarshalJSON decodes a value into the option.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		o.Clear()
		return nil
	}
	if err := json.Unmarshal(b, &o.value); err != nil {
		return fmt.Errorf("failed to unmarshal Opt: %w", err)
	}
	o.set = true
	return nil
}

// MergeMap merges two Opt[map[K]V], combining their entries.
func MergeMap[K comparable, V any](dst, src Opt[map[K]V]) Opt[map[K]V] {
	merged := dst.Or(make(map[K]V))
	for k, v := range src.Or(nil) {
		merged[k] = v
	}
	return Map(merged)
}
