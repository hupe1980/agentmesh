// Package command provides a fluent API for building state updates with type safety.
// It is used primarily in node functions to construct the return tuple of
// ([]string, state.Updates, error).
//
// Example usage:
//
//	import "github.com/hupe1980/agentmesh/pkg/command"
//
//	func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
//	    return command.New().
//	        Set(statusKey, "completed").
//	        Set(counterKey, 42).
//	        To("next_node")
//	}
package command

import "github.com/hupe1980/agentmesh/pkg/state"

// Command provides a fluent API for building state updates with type safety.
// It eliminates the need for .Name() calls when setting key-value pairs.
//
// Example:
//
//	return command.New().
//	    Set(messagesKey, append(msgs, newMsg)).
//	    Set(countKey, count + 1).
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

// Set adds a key-value pair to the updates map.
// While the value is accepted as any, type safety is encouraged at the call site
// by using typed Key[T] values which guide the caller to provide the correct type.
// If the Command has already encountered an error, Set returns immediately without modifying state.
//
// Example:
//
//	var msgKey = state.NewKey[[]string]("messages", nil)
//	cmd.Set(msgKey, []string{"hello"})  // Type guidance from Key[T]
func (c *Command) Set(key interface{ Name() string }, value any) *Command {
	if c.err != nil {
		return c // Skip if already errored
	}
	c.m[key.Name()] = value
	return c
}

// SetAll adds all key-value pairs from another Command.
// This enables merging updates from multiple sources while maintaining fluent API.
//
// Example:
//
//	listUpdates := command.Append(msgKey, "hello")
//	return command.New().
//	    Set(statusKey, "processing").
//	    SetAll(listUpdates).
//	    To("next")
func (c *Command) SetAll(other *Command) *Command {
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

// With applies a function to the Command, enabling method-like chaining.
// This allows you to use Append/AppendMany as if they were methods.
//
// Example:
//
//	return command.New().
//	    Set(statusKey, "processing").
//	    With(func(c *Command) *Command {
//	        return command.Append(msgKey, "Started", c)
//	    }).
//	    To("next")
//
// Or more idiomatically using a helper pattern:
//
//	append := func(key state.ListKey[string], val string) func(*Command) *Command {
//	    return func(c *Command) *Command { return command.Append(key, val, c) }
//	}
//	return command.New().
//	    Set(statusKey, "processing").
//	    With(append(msgKey, "Started")).
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
//	return []string{"next"}, command.New().Set(key, val).Build()
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
//	    Set(messagesKey, msgs).
//	    Set(counterKey, 42).
//	    To("next", "backup")  // Returns ([]string, state.Updates, error)
func (c *Command) To(targets ...string) ([]string, state.Updates, error) {
	if c.err != nil {
		return nil, nil, c.err
	}
	return targets, state.Updates(c.m), nil
}

// Append adds a single value to a list key and returns a Command for chaining.
// It can extend an existing Command or create a new one.
//
// Examples:
//
//	// Start a new chain with append
//	return command.Append(msgKey, "hello").To("next")
//
//	// Extend existing Command (chainable via SetAll)
//	return command.New().
//	    Set(statusKey, "done").
//	    SetAll(command.Append(msgKey, "hello")).
//	    To("next")
func Append[T any](key state.ListKey[T], value T, extend ...*Command) *Command {
	c := firstOrNew(extend)
	return appendToList(c, key, []T{value})
}

// AppendMany adds multiple values to a list key and returns a Command for chaining.
// It can extend an existing Command or create a new one.
//
// Examples:
//
//	// Start a new chain with append
//	return command.AppendMany(msgKey, []string{"a", "b"}).To("next")
//
//	// Extend existing Command (chainable via SetAll)
//	return command.New().
//	    Set(statusKey, "done").
//	    SetAll(command.AppendMany(msgKey, []string{"a", "b"})).
//	    To("next")
func AppendMany[T any](key state.ListKey[T], values []T, extend ...*Command) *Command {
	c := firstOrNew(extend)
	return appendToList(c, key, values)
}

// firstOrNew returns the first Command in the slice or a new Command if none exist.
func firstOrNew(cmd []*Command) *Command {
	if len(cmd) > 0 && cmd[0] != nil {
		return cmd[0]
	}
	return New()
}

// appendToList handles all list modification in one place.
// Prevents code duplication between Append and AppendMany.
func appendToList[T any](cmd *Command, key state.ListKey[T], values []T) *Command {
	if cmd.err != nil {
		return cmd
	}

	// Apply max-size if configured
	if n := key.MaxSize(); n > 0 && len(values) > n {
		values = values[len(values)-n:]
	}

	cmd.m[key.Name()] = state.SliceOf[T](values)
	return cmd
}
