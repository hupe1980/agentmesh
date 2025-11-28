package state

// UpdateBuilder provides a fluent, type-safe API for constructing Updates maps.
// Unlike command.Command (which is for nodes with routing), UpdateBuilder is for
// direct state mutations without routing logic (middleware, initialization, etc.).
//
// Uses compile-time type checking through Key[T] and ListKey[T] types.
//
// Example:
//
//	var msgKey = NewListKey[string]("messages", 100)
//	var countKey = NewKey[int]("count", 0)
//
//	updates := NewUpdateBuilder().
//	    With(SetValue(countKey, 42)).
//	    With(AppendValue(msgKey, "hello", "world")).
//	    Build()
type UpdateBuilder struct {
	m   map[string]any
	err error
}

// NewUpdateBuilder creates a new UpdateBuilder for constructing state updates.
func NewUpdateBuilder() *UpdateBuilder {
	return &UpdateBuilder{m: make(map[string]any)}
}

// With applies a modifier function to the UpdateBuilder, enabling method-like chaining.
// This is used with the type-safe helper functions like SetValue and AppendValue.
//
// Example:
//
//	updates := NewUpdateBuilder().
//	    With(SetValue(key, "value")).
//	    With(AppendValue(listKey, "item1", "item2")).
//	    Build()
func (b *UpdateBuilder) With(fn func(*UpdateBuilder) *UpdateBuilder) *UpdateBuilder {
	if b.err != nil {
		return b
	}
	return fn(b)
}

// SetValue sets a typed value for a key with compile-time type checking.
// Returns a function for use with UpdateBuilder.With().
//
// Example:
//
//	var statusKey = NewKey[string]("status", "")
//	updates := NewUpdateBuilder().
//	    With(SetValue(statusKey, "completed")).
//	    Build()
func SetValue[T any](key Key[T], value T) func(*UpdateBuilder) *UpdateBuilder {
	return func(b *UpdateBuilder) *UpdateBuilder {
		if b.err != nil {
			return b
		}
		b.m[key.Name()] = value
		return b
	}
}

// AppendValue adds one or more values to a list key with compile-time type checking.
// Returns a function for use with UpdateBuilder.With().
//
// Note: This creates a new list with the given values. When applied via ApplyUpdates(),
// the StateManager will merge this with existing list values.
//
// Examples:
//
//	// Single value
//	updates := NewUpdateBuilder().
//	    With(AppendValue(msgKey, "hello")).
//	    Build()
//
//	// Multiple values
//	updates := NewUpdateBuilder().
//	    With(AppendValue(msgKey, "a", "b", "c")).
//	    Build()
//
//	// Spread a slice
//	msgs := []string{"x", "y"}
//	updates := NewUpdateBuilder().
//	    With(AppendValue(msgKey, msgs...)).
//	    Build()
func AppendValue[T any](key ListKey[T], values ...T) func(*UpdateBuilder) *UpdateBuilder {
	return func(b *UpdateBuilder) *UpdateBuilder {
		if b.err != nil {
			return b
		}
		b.m[key.Name()] = SliceOf[T](values)
		return b
	}
}

// Build returns the accumulated Updates map.
//
// Example:
//
//	updates := NewUpdateBuilder().
//	    With(SetValue(key, val)).
//	    Build()
func (b *UpdateBuilder) Build() Updates {
	return Updates(b.m)
}
