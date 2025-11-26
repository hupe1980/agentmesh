package graph

import "github.com/hupe1980/agentmesh/pkg/state"

// Command provides a fluent API for building state updates with type safety.
// It eliminates the need for .Name() calls when setting key-value pairs.
//
// Example:
//
//	return NewCommand().
//	    Set(messagesKey, append(msgs, newMsg)).
//	    Set(countKey, count + 1).
//	    To("next")
//
// Command accumulates errors during building and returns them in Build() or To().
type Command struct {
	m   map[string]any
	err error
}

// NewCommand creates a new Command builder for constructing state updates.
func NewCommand() *Command {
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

// Build returns the accumulated updates and any error encountered during building.
// This method matches the last two return values of NodeFunc: (state.Updates, error).
//
// Example:
//
//	return []string{"next"}, NewCommand().Set(key, val).Build()
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
//	return NewCommand().
//	    Set(messagesKey, msgs).
//	    Set(countKey, 42).
//	    To("next", "backup")  // Returns ([]string, state.Updates, error)
func (c *Command) To(targets ...string) ([]string, state.Updates, error) {
	if c.err != nil {
		return nil, nil, c.err
	}
	return targets, state.Updates(c.m), nil
}
