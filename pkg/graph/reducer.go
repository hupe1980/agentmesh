package graph

import (
	"cmp"
	"fmt"
	"maps"
)

// Reducer specifies how values are combined for a state key.
// Implementations must be deterministic and ideally commutative for parallel writes.
type Reducer[T any] interface {
	// Zero returns the identity element for T (used when no prior value exists).
	Zero() T

	// Reduce merges an incoming value into the existing state.
	Reduce(existing, incoming T) T
}

// -----------------------------------------------------------------------------
// Built-in Reducers
// -----------------------------------------------------------------------------

// ReplaceReducer always returns the incoming value (last-write-wins).
// This is the default reducer for scalar keys.
type ReplaceReducer[T any] struct{}

// Zero returns the zero value of T.
func (ReplaceReducer[T]) Zero() T {
	var z T
	return z
}

// Reduce returns the incoming value, discarding the existing value.
func (ReplaceReducer[T]) Reduce(_, incoming T) T {
	return incoming
}

// AppendReducer concatenates slices.
// This is the default reducer for list keys.
type AppendReducer[T any] struct{}

// Zero returns nil (empty slice).
func (AppendReducer[T]) Zero() []T {
	return nil
}

// Reduce appends incoming slice to existing slice.
func (AppendReducer[T]) Reduce(existing, incoming []T) []T {
	return append(existing, incoming...)
}

// PrependReducer inserts incoming slice at the front of existing slice.
type PrependReducer[T any] struct{}

// Zero returns nil (empty slice).
func (PrependReducer[T]) Zero() []T {
	return nil
}

// Reduce prepends incoming slice before existing slice.
func (PrependReducer[T]) Reduce(existing, incoming []T) []T {
	return append(incoming, existing...)
}

// SumReducer adds numeric values.
type SumReducer[T ~int | ~int64 | ~float64] struct{}

// Zero returns 0.
func (SumReducer[T]) Zero() T {
	return 0
}

// Reduce adds the incoming value to the existing value.
func (SumReducer[T]) Reduce(existing, incoming T) T {
	return existing + incoming
}

// MaxReducer keeps the larger value.
type MaxReducer[T cmp.Ordered] struct{}

// Zero returns the zero value of T.
func (MaxReducer[T]) Zero() T {
	var z T
	return z
}

// Reduce returns the maximum of existing and incoming.
func (MaxReducer[T]) Reduce(existing, incoming T) T {
	return max(existing, incoming)
}

// MinReducer keeps the smaller value.
type MinReducer[T cmp.Ordered] struct{}

// Zero returns the zero value of T.
func (MinReducer[T]) Zero() T {
	var z T
	return z
}

// Reduce returns the minimum of existing and incoming.
func (MinReducer[T]) Reduce(existing, incoming T) T {
	return min(existing, incoming)
}

// MergeMapReducer unions two maps; later keys overwrite earlier.
type MergeMapReducer[K comparable, V any] struct{}

// Zero returns nil (empty map).
func (MergeMapReducer[K, V]) Zero() map[K]V {
	return nil
}

// Reduce merges incoming map into existing map.
func (MergeMapReducer[K, V]) Reduce(existing, incoming map[K]V) map[K]V {
	if existing == nil {
		existing = make(map[K]V)
	}

	maps.Copy(existing, incoming)

	return existing
}

// FirstReducer keeps the earliest non-zero value.
type FirstReducer[T comparable] struct{}

// Zero returns the zero value of T.
func (FirstReducer[T]) Zero() T {
	var z T
	return z
}

// Reduce returns existing if non-zero, otherwise incoming.
func (FirstReducer[T]) Reduce(existing, incoming T) T {
	var z T
	if existing != z {
		return existing
	}

	return incoming
}

// LastReducer is an alias for ReplaceReducer - keeps the most recent value.
type LastReducer[T any] = ReplaceReducer[T]

// -----------------------------------------------------------------------------
// Reducer Wrappers
// -----------------------------------------------------------------------------

// SkipZeroReducer wraps a reducer to skip zero-value inputs.
// This preserves the existing value when the incoming value is the zero value.
type SkipZeroReducer[T comparable, R Reducer[T]] struct {
	Inner R
}

// Zero delegates to the inner reducer.
func (r SkipZeroReducer[T, R]) Zero() T {
	return r.Inner.Zero()
}

// Reduce skips the incoming value if it's the zero value.
func (r SkipZeroReducer[T, R]) Reduce(existing, incoming T) T {
	var zero T
	if incoming == zero {
		return existing
	}

	return r.Inner.Reduce(existing, incoming)
}

// NewSkipZeroReducer creates a SkipZeroReducer wrapper around a reducer.
func NewSkipZeroReducer[T comparable, R Reducer[T]](inner R) SkipZeroReducer[T, R] {
	return SkipZeroReducer[T, R]{Inner: inner}
}

// -----------------------------------------------------------------------------
// Type-erased Reducer for Runtime
// -----------------------------------------------------------------------------

// ReducerFunc is a type-erased reducer function for runtime use.
// It wraps the generic Reducer[T] for storage in maps and runtime dispatch.
type ReducerFunc struct {
	// ZeroFn returns the zero value for this reducer.
	ZeroFn func() any

	// ReduceFn merges incoming into existing and returns the result.
	ReduceFn func(existing, incoming any) any
}

// WrapReducer creates a type-erased ReducerFunc from a generic Reducer[T].
// This is used at graph build time to store reducers in a registry.
func WrapReducer[T any](r Reducer[T]) ReducerFunc {
	return ReducerFunc{
		ZeroFn: func() any {
			return r.Zero()
		},
		ReduceFn: func(existing, incoming any) any {
			var ex T
			if existing != nil {
				ex = coerceToType[T](existing)
			} else {
				ex = r.Zero()
			}

			inc := coerceToType[T](incoming)
			return r.Reduce(ex, inc)
		},
	}
}

// coerceToType converts a value to type T.
func coerceToType[T any](v any) T {
	// Direct type match (fast path)
	if typed, ok := v.(T); ok {
		return typed
	}

	panic(fmt.Sprintf("cannot coerce %T to %T", v, *new(T)))
}
