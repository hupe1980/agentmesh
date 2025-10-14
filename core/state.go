package core

import (
	"fmt"
	"maps"
)

// State maintains the current value and the pending-commit delta.
type State struct {
	value map[string]any
	delta map[string]any
}

const (
	// AppPrefix is the prefix for application-scoped state keys.
	AppPrefix = "app:"
	// UserPrefix is the prefix for user-scoped state keys.
	UserPrefix = "user:"
	// RunPrefix is the prefix for run-scoped state keys.
	RunPrefix = "run:"
)

// NewState creates a new State with value and delta maps.
func NewState(value, delta map[string]any) *State {
	if value == nil {
		value = make(map[string]any)
	}

	if delta == nil {
		delta = make(map[string]any)
	}

	return &State{
		value: value,
		delta: delta,
	}
}

// Get returns the value for a given key, falling back to default if not present.
func (s *State) Get(key string, defaultVal any) any {
	if v, ok := s.delta[key]; ok {
		return v
	}

	if v, ok := s.value[key]; ok {
		return v
	}

	return defaultVal
}

// Set sets the value for a given key (stored in both value and delta).
func (s *State) Set(key string, val any) {
	s.value[key] = val
	s.delta[key] = val
}

// Contains checks if the key exists in either value or delta.
func (s *State) Contains(key string) bool {
	if _, ok := s.delta[key]; ok {
		return true
	}

	if _, ok := s.value[key]; ok {
		return true
	}

	return false
}

// SetDefault returns the existing value for a key, or sets it to default if missing.
func (s *State) SetDefault(key string, defaultVal any) any {
	if s.Contains(key) {
		return s.Get(key, nil)
	}

	s.Set(key, defaultVal)

	return defaultVal
}

// HasDelta checks if the state has pending deltas.
func (s *State) HasDelta() bool {
	return len(s.delta) > 0
}

// Update applies a delta map to both value and delta.
func (s *State) Update(delta map[string]any) {
	for k, v := range delta {
		s.value[k] = v
		s.delta[k] = v
	}
}

// ToMap returns a merged map of value + delta.
func (s *State) ToMap() map[string]any {
	result := make(map[string]any, len(s.value)+len(s.delta))

	maps.Copy(result, s.value)
	maps.Copy(result, s.delta)

	return result
}

// StateString returns the value for the given key as a string.
func StateString(snap StateSnapshotter, key string) (string, error) {
	val, ok := snap.StateSnapshot()[key]
	if !ok {
		return "", ErrKeyMissing{Key: key}
	}
	str, ok := val.(string)
	if !ok {
		return "", ErrTypeMismatch{Key: key, Expected: "string", ActualType: fmt.Sprintf("%T", val)}
	}
	return str, nil
}

// StateTyped returns the value for the given key as type T.
func StateTyped[T any](snap StateSnapshotter, key string) (T, error) {
	var zero T

	val, ok := snap.StateSnapshot()[key]
	if !ok {
		return zero, ErrKeyMissing{Key: key}
	}

	typed, ok := val.(T)
	if !ok {
		return zero, ErrTypeMismatch{Key: key, Expected: fmt.Sprintf("%T", zero), ActualType: fmt.Sprintf("%T", val)}
	}

	return typed, nil
}
