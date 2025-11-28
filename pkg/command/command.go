// Package command provides a truly type-safe fluent API for building state updates.
// It uses function-based helpers with generics to enforce compile-time type checking.
//
// Example usage:
//
//	import "github.com/hupe1980/agentmesh/pkg/command"
//
//	func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	    return command.New().
//	        With(command.SetValue(statusKey, "completed")).
//	        With(command.SetValue(counterKey, 42)).
//	        To("next_node")
//	}
package command

import "github.com/hupe1980/agentmesh/pkg/state"

// Command provides a fluent API for building state updates with true type safety
// through function-based helpers.
//
// Example:
//
//	return command.New().
//	    With(command.SetValue(statusKey, "completed")).
//	    With(command.Append(msgKey, newMsg)).
//	    To("next")
//
// Command accumulates errors during building and returns them in Build() or To().
type Command struct {
	m   map[string]any
	err error
}

// New creates a new Command builder for constructing state updates.
func New() *Command {
	return &Command{m: make(map[string]any)}
}

// With applies a function to the Command, enabling method-like chaining.
// All type-safe helpers return func(*Command) *Command for use with With().
//
// Example:
//
//	return command.New().
//	    With(command.SetValue(statusKey, "processing")).
//	    With(command.Append(msgKey, "Started")).
//	    To("next")
func (c *Command) With(fn func(*Command) *Command) *Command {
	if c.err != nil {
		return c
	}
	return fn(c)
}

// Build returns the accumulated updates and any error encountered during building.
// This method matches the last two return values of NodeFunc: (state.Updates, error).
//
// Example:
//
//	updates, err := command.New().
//	    With(command.SetValue(key, val)).
//	    Build()
func (c *Command) Build() (state.Updates, error) {
	if c.err != nil {
		return nil, c.err
	}
	return state.Updates(c.m), nil
}

// To returns a complete tuple for NodeFunc: (targets, updates, error).
// This is a convenience method that combines target routing with state updates.
//
// Example:
//
//	return command.New().
//	    With(command.SetValue(statusKey, "done")).
//	    With(command.Append(msgKey, msg)).
//	    To("next", "backup")
func (c *Command) To(targets ...string) ([]string, state.Updates, error) {
	if c.err != nil {
		return nil, nil, c.err
	}
	return targets, state.Updates(c.m), nil
}

// SetValue sets a typed value for a key with compile-time type checking.
// The generic parameter T is inferred from the Key[T], ensuring type safety.
//
// Example:
//
//	var statusKey = state.NewKey[string]("status", "")
//	cmd.With(command.SetValue(statusKey, "completed"))  // ✅ Type-safe
//	cmd.With(command.SetValue(statusKey, 42))           // ❌ Compile error
func SetValue[T any](key state.Key[T], value T) func(*Command) *Command {
	return func(c *Command) *Command {
		if c.err != nil {
			return c
		}

		c.m[key.Name()] = value
		return c
	}
}

// Append adds one or more values to a list key.
// Returns a function for use with Command.With().
//
// Note: MaxSize enforcement happens at the state manager level when updates
// are applied, not during command building.
//
// Examples:
//
//	// Single value
//	cmd.With(command.Append(msgKey, "hello"))
//
//	// Multiple values
//	cmd.With(command.Append(msgKey, "a", "b", "c"))
//
//	// Spread a slice
//	msgs := []string{"x", "y"}
//	cmd.With(command.Append(msgKey, msgs...))
func Append[T any](key state.ListKey[T], values ...T) func(*Command) *Command {
	return func(c *Command) *Command {
		if c.err != nil {
			return c
		}

		// Get existing list from command's accumulated updates (if any)
		var list []T
		if existing, ok := c.m[key.Name()]; ok {
			if sv, ok := existing.(state.SliceValue); ok {
				slice := sv.ToSlice()
				list = make([]T, 0, len(slice)+len(values))
				for _, v := range slice {
					if typed, ok := v.(T); ok {
						list = append(list, typed)
					}
				}
			}
		}
		list = append(list, values...)

		c.m[key.Name()] = state.SliceOf[T](list)
		return c
	}
}

// Merge merges updates from another Command into the current one.
// Returns a function for use with Command.With().
//
// Example:
//
//	cmd1 := command.New().With(command.SetValue(statusKey, "a"))
//	cmd2 := command.New().With(command.SetValue(counterKey, 1))
//
//	return command.New().
//	    With(command.Merge(cmd1)).
//	    With(command.Merge(cmd2)).
//	    To("next")
func Merge(other *Command) func(*Command) *Command {
	return func(c *Command) *Command {
		if c.err != nil {
			return c
		}
		if other.err != nil {
			c.err = other.err
			return c
		}
		for k, v := range other.m {
			c.m[k] = v
		}
		return c
	}
}

// SetAll merges updates from multiple Commands.
// Returns a function for use with Command.With().
//
// Example:
//
//	cmd1 := command.New().With(command.SetValue(statusKey, "a"))
//	cmd2 := command.New().With(command.SetValue(counterKey, 1))
//
//	return command.New().
//	    With(command.SetAll(cmd1, cmd2)).
//	    To("next")
func SetAll(others ...*Command) func(*Command) *Command {
	return func(c *Command) *Command {
		for _, other := range others {
			if c.err != nil {
				return c
			}
			if other.err != nil {
				c.err = other.err
				return c
			}
			for k, v := range other.m {
				c.m[k] = v
			}
		}
		return c
	}
}
