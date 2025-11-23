package graph

import (
	"errors"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// UpdateBuilder provides type-safe state update construction for StaticNodes.
// Use this for nodes with STATIC routing (always go to the same target).
//
// All updates are validated at build time to ensure:
// - No duplicate keys
// - Type safety through generic methods
//
// Example usage:
//
//	builder.AddStaticNode("process", targets, func(ctx, view) (state.Updates, error) {
//	    return graph.NewUpdate().
//	        Set(counterKey, 42).
//	        Append(messagesKey, msg1, msg2).
//	        Build()
//	})
type UpdateBuilder struct {
	updates map[string]any
	errors  []error
}

// NewUpdate creates a new update builder for static routing nodes.
// Use this in AddStaticNode functions that always route to the same target.
func NewUpdate() *UpdateBuilder {
	return &UpdateBuilder{
		updates: make(map[string]any),
		errors:  make([]error, 0),
	}
}

// UpdateSet adds a typed key-value pair to the update builder.
// Compile-time type safety ensures the value matches the key's type.
//
// Example:
//
//	counterKey := state.NewKey[int]("counter", 0)
//	graph.UpdateSet(builder, counterKey, 42) // ✓ Type-safe
//	graph.UpdateSet(builder, counterKey, "hello") // ✗ Compile error
func UpdateSet[T any](b *UpdateBuilder, key state.Key[T], value T) *UpdateBuilder {
	if _, exists := b.updates[key.Name()]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", key.Name()))
		return b
	}
	b.updates[key.Name()] = value
	return b
}

// UpdateAppend adds typed values to append to a list key.
// The values are wrapped in state.SliceOf[T] for efficient append operations.
//
// Example:
//
//	messagesKey := state.NewListKey[string]("messages", 100)
//	graph.UpdateAppend(builder, messagesKey, "msg1", "msg2", "msg3")
func UpdateAppend[T any](b *UpdateBuilder, key state.ListKey[T], values ...T) *UpdateBuilder {
	if _, exists := b.updates[key.Name()]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", key.Name()))
		return b
	}
	if len(values) == 0 {
		// Empty append is valid but no-op
		return b
	}
	b.updates[key.Name()] = state.SliceOf[T](values)
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

// Delete marks a key for deletion from state.
// This removes the key entirely, not just setting it to zero value.
//
// Example:
//
//	builder.Delete("temp_key")
func (b *UpdateBuilder) Delete(keyName string) *UpdateBuilder {
	if _, exists := b.updates[keyName]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", keyName))
		return b
	}
	// Use a sentinel value to indicate deletion
	b.updates[keyName] = deleteMarker{}
	return b
}

// Build constructs the Updates map and returns any validation errors.
// If any errors occurred during building (duplicate keys, etc.), they are returned.
//
// This is a terminal operation for UpdateBuilder.
func (b *UpdateBuilder) Build() (state.Updates, error) {
	if len(b.errors) > 0 {
		return nil, errors.Join(b.errors...)
	}
	return b.updates, nil
}

// MustBuild is like Build but panics on error.
// Use only in tests or when errors are impossible.
func (b *UpdateBuilder) MustBuild() state.Updates {
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
