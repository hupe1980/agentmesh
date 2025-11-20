package state

import (
	"errors"
	"fmt"
)

// UpdateBuilder provides type-safe state update construction.
// All updates are validated at build time to ensure:
// - Keys are registered
// - Types match registered key types
// - No duplicate keys
//
// Example usage:
//
//	builder := NewUpdateBuilder()
//	SetUpdate(builder, counterKey, 42)
//	AppendUpdate(builder, messagesKey, msg1, msg2)
//	updates, err := builder.Build()
type UpdateBuilder struct {
	updates map[string]any
	errors  []error
}

// NewUpdateBuilder creates a new type-safe update builder.
func NewUpdateBuilder() *UpdateBuilder {
	return &UpdateBuilder{
		updates: make(map[string]any),
		errors:  make([]error, 0),
	}
}

// SetUpdate adds a typed key-value pair to the update builder.
// Compile-time type safety ensures the value matches the key's type.
//
// Example:
//
//	counterKey := NewKey[int]("counter", 0)
//	SetUpdate(builder, counterKey, 42) // ✓ Type-safe
//	SetUpdate(builder, counterKey, "hello") // ✗ Compile error
func SetUpdate[T any](b *UpdateBuilder, key Key[T], value T) *UpdateBuilder {
	if _, exists := b.updates[key.name]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", key.name))
		return b
	}
	b.updates[key.name] = value
	return b
}

// AppendUpdate adds typed values to append to a list key.
// The values are wrapped in SliceOf[T] for efficient append operations.
//
// Example:
//
//	messagesKey := NewListKey[message.Message]("messages", 100)
//	AppendUpdate(builder, messagesKey, msg1, msg2, msg3)
func AppendUpdate[T any](b *UpdateBuilder, key ListKey[T], values ...T) *UpdateBuilder {
	if _, exists := b.updates[key.name]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", key.name))
		return b
	}
	if len(values) == 0 {
		// Empty append is valid but no-op
		return b
	}
	b.updates[key.name] = SliceOf[T](values)
	return b
}

// Delete marks a key for deletion from state.
// This removes the key entirely, not just setting it to zero value.
//
// Example:
//
//	builder.Delete(tempKey)
func (b *UpdateBuilder) Delete(keyName string) *UpdateBuilder {
	if _, exists := b.updates[keyName]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", keyName))
		return b
	}
	// Use a sentinel value to indicate deletion
	b.updates[keyName] = deleteMarker{}
	return b
}

// SetRaw adds an untyped key-value pair to the updates.
// Use this only when you don't have a typed Key[T] available.
// Prefer Set() for type safety.
//
// Example:
//
//	builder.SetRaw("dynamic_key", value)
func (b *UpdateBuilder) SetRaw(keyName string, value any) *UpdateBuilder {
	if _, exists := b.updates[keyName]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", keyName))
		return b
	}
	b.updates[keyName] = value
	return b
}

// Build constructs the Updates map and returns any validation errors.
// If any errors occurred during building (duplicate keys, etc.), they are returned.
//
// Note: This does NOT validate that keys are registered in the manager.
// That validation happens when ApplyUpdates is called on the manager.
func (b *UpdateBuilder) Build() (Updates, error) {
	if len(b.errors) > 0 {
		return nil, errors.Join(b.errors...)
	}
	return b.updates, nil
}

// MustBuild is like Build but panics on error.
// Use only in tests or when errors are impossible.
func (b *UpdateBuilder) MustBuild() Updates {
	updates, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("UpdateBuilder.MustBuild: %v", err))
	}
	return updates
}

// IsEmpty returns true if no updates have been added.
func (b *UpdateBuilder) IsEmpty() bool {
	return len(b.updates) == 0
}

// deleteMarker is a sentinel value to indicate key deletion.
type deleteMarker struct{}
