package graph

import (
	"errors"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// CommandBuilder provides unified construction of state updates and routing decisions.
// Use this for CommandNodes with DYNAMIC routing (routing decision in the logic).
//
// CommandBuilder combines state updates and control flow into a single fluent API,
// making it impossible to forget passing updates to routing functions.
//
// Example usage:
//
//	return graph.NewCommand().
//	    Set(statusKey, "processing").
//	    Set(attemptsKey, 1).
//	    Append(logKey, "Started processing").
//	    Goto("next_step")
//
// For conditional routing:
//
//	cmd := graph.NewCommand().Set(processedKey, true)
//	if valid {
//	    return cmd.Goto("success")
//	}
//	return cmd.Goto("retry")
type CommandBuilder struct {
	updates map[string]any
	errors  []error
}

// NewCommand creates a new command builder for dynamic routing.
// Use this in CommandNode functions that need to make routing decisions.
func NewCommand() *CommandBuilder {
	return &CommandBuilder{
		updates: make(map[string]any),
		errors:  make([]error, 0),
	}
}

// CommandSet adds a typed key-value pair to the command builder.
// Compile-time type safety ensures the value matches the key's type.
//
// Example:
//
//	counterKey := state.NewKey[int]("counter", 0)
//	graph.CommandSet(builder, counterKey, 42) // ✓ Type-safe
//	graph.CommandSet(builder, counterKey, "hello") // ✗ Compile error
func CommandSet[T any](b *CommandBuilder, key state.Key[T], value T) *CommandBuilder {
	if _, exists := b.updates[key.Name()]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", key.Name()))
		return b
	}
	b.updates[key.Name()] = value
	return b
}

// CommandAppend adds typed values to append to a list key.
// The values are wrapped in state.SliceOf[T] for efficient append operations.
//
// Example:
//
//	messagesKey := state.NewListKey[string]("messages", 100)
//	graph.CommandAppend(builder, messagesKey, "msg1", "msg2", "msg3")
func CommandAppend[T any](b *CommandBuilder, key state.ListKey[T], values ...T) *CommandBuilder {
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
func (b *CommandBuilder) SetRaw(keyName string, value any) *CommandBuilder {
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
func (b *CommandBuilder) Delete(keyName string) *CommandBuilder {
	if _, exists := b.updates[keyName]; exists {
		b.errors = append(b.errors, fmt.Errorf("duplicate key %q in updates", keyName))
		return b
	}
	b.updates[keyName] = deleteMarker{}
	return b
}

// Goto creates a command that routes to a single target with the accumulated updates.
// This is a terminal operation that builds the Command and returns any validation errors.
//
// Example:
//
//	return graph.NewCommand().
//	    Set(statusKey, "done").
//	    Goto("next_node")
func (b *CommandBuilder) Goto(target string) (*Command, error) {
	if len(b.errors) > 0 {
		return nil, errors.Join(b.errors...)
	}
	return &Command{
		Updates: b.updates,
		Goto:    []string{target},
	}, nil
}

// GotoAll creates a command that routes to multiple targets (parallel execution).
// This is a terminal operation that builds the Command and returns any validation errors.
//
// Example:
//
//	return graph.NewCommand().
//	    Set(startedKey, true).
//	    GotoAll("task1", "task2", "task3")
func (b *CommandBuilder) GotoAll(targets ...string) (*Command, error) {
	if len(b.errors) > 0 {
		return nil, errors.Join(b.errors...)
	}
	return &Command{
		Updates: b.updates,
		Goto:    targets,
	}, nil
}

// End creates a command that terminates execution with the accumulated updates.
// This is a terminal operation that builds the Command and returns any validation errors.
//
// Example:
//
//	return graph.NewCommand().
//	    Set(finalKey, result).
//	    End()
func (b *CommandBuilder) End() (*Command, error) {
	if len(b.errors) > 0 {
		return nil, errors.Join(b.errors...)
	}
	return &Command{
		Updates: b.updates,
		Goto:    []string{EndNode},
	}, nil
}

// IsEmpty returns true if no updates have been added.
func (b *CommandBuilder) IsEmpty() bool {
	return len(b.updates) == 0
}

// deleteMarker is a sentinel value to indicate key deletion.
// This is recognized by state.Manager.ApplyUpdates() to delete keys.
type deleteMarker struct{}
