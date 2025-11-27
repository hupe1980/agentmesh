package state

// UpdateBuilder provides a fluent, type-safe API for constructing Updates maps.
// It enforces compile-time type checking through Key[T] and ListKey[T] types.
//
// Example:
//
//	var msgKey = NewListKey[string]("messages", 100)
//	var countKey = NewKey[int]("count", 0)
//
//	updates := NewUpdateBuilder().
//	    Set(countKey, 42).
//	    AppendMany(msgKey, []string{"hello", "world"}).
//	    Build()
type UpdateBuilder struct {
	m   map[string]any
	err error
}

// NewUpdateBuilder creates a new UpdateBuilder for constructing state updates.
func NewUpdateBuilder() *UpdateBuilder {
	return &UpdateBuilder{m: make(map[string]any)}
}

// Set adds a key-value pair to the updates map with type safety from Key[T].
// The Key[T] parameter ensures the value type matches the key's declared type.
//
// Example:
//
//	var key = NewKey[string]("name", "")
//	builder.Set(key, "Alice")  // Type-safe: only string allowed
func (b *UpdateBuilder) Set(key interface{ Name() string }, value any) *UpdateBuilder {
	if b.err != nil {
		return b
	}
	b.m[key.Name()] = value
	return b
}

// WithAppend adds a single value to a list-typed state key with compile-time type safety.
// This allows chaining with Set() for mixed updates.
//
// Example:
//
//	var msgKey = NewListKey[string]("messages", 100)
//	updates := WithAppend(
//	    NewUpdateBuilder().Set(counterKey, 42),
//	    msgKey, "hello",
//	).Build()
func WithAppend[T any](b *UpdateBuilder, key ListKey[T], value T) *UpdateBuilder {
	if b.err != nil {
		return b
	}
	b.m[key.Name()] = SliceOf[T]([]T{value})
	return b
}

// WithAppendMany adds multiple values to a list-typed state key with compile-time type safety.
// This allows chaining with Set() for mixed updates.
//
// Example:
//
//	var msgKey = NewListKey[string]("messages", 100)
//	updates := WithAppendMany(
//	    NewUpdateBuilder().Set(counterKey, 42),
//	    msgKey, []string{"hello", "world"},
//	).Build()
func WithAppendMany[T any](b *UpdateBuilder, key ListKey[T], values []T) *UpdateBuilder {
	if b.err != nil {
		return b
	}
	b.m[key.Name()] = SliceOf[T](values)
	return b
}

// Build returns the accumulated Updates map and any error encountered.
//
// Example:
//
//	updates := NewUpdateBuilder().Set(key, val).Build()
func (b *UpdateBuilder) Build() Updates {
	return Updates(b.m)
}

// AppendUpdate creates an UpdateBuilder that appends a single value to a list key.
// This provides compile-time type safety for list operations.
//
// Example:
//
//	var msgKey = NewListKey[string]("messages", 100)
//	updates := AppendUpdate(msgKey, "hello").Build()
func AppendUpdate[T any](key ListKey[T], value T) *UpdateBuilder {
	builder := NewUpdateBuilder()
	builder.m[key.Name()] = SliceOf[T]([]T{value})
	return builder
}

// AppendManyUpdates creates an UpdateBuilder that appends multiple values to a list key.
// This provides compile-time type safety for batch list operations.
//
// Example:
//
//	var msgKey = NewListKey[string]("messages", 100)
//	updates := AppendManyUpdates(msgKey, []string{"hello", "world"}).Build()
func AppendManyUpdates[T any](key ListKey[T], values []T) *UpdateBuilder {
	builder := NewUpdateBuilder()
	builder.m[key.Name()] = SliceOf[T](values)
	return builder
}
