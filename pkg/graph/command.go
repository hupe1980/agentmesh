package graph

// Command is what a node returns: state updates and next targets.
//
// Create with:
//   - To("next")                     - just routing
//   - Set(key, val).To("next")       - with updates
//   - Fail(err)                      - error
//
// Example - simple routing:
//
//	return graph.To("next")
//	return graph.To(graph.END)
//
// Example - with one update:
//
//	return graph.Set(StatusKey, "done").To("next")
//
// Example - with multiple updates:
//
//	return graph.Set(Key1, val1).
//	    Set(Key2, val2).
//	    Append(ListKey, item).
//	    To("next")
//
// Example - conditional:
//
//	if done {
//	    return graph.To(graph.END)
//	}
//	return graph.Set(CountKey, count+1).To("process")
//
// Example - error:
//
//	if err != nil {
//	    return graph.Fail(err)
//	}
type Command struct {
	Updates Updates  // State changes
	Next    []string // Next nodes to execute (or END)
}

// To creates a Command that routes to the specified targets without updates.
//
// Example:
//
//	return graph.To("next")
//	return graph.To(graph.END)
//	return graph.To("a", "b")  // parallel
func To(targets ...string) (*Command, error) {
	return &Command{Updates: nil, Next: targets}, nil
}

// Fail returns an error without a command.
//
// Example:
//
//	if err != nil {
//	    return graph.Fail(err)
//	}
func Fail(err error) (*Command, error) {
	return nil, err
}

// -----------------------------------------------------------------------------
// CommandBuilder - fluent builder for commands with updates
// -----------------------------------------------------------------------------

// CommandBuilder accumulates updates before creating a Command.
type CommandBuilder struct {
	updates Updates
}

// Cmd creates an empty CommandBuilder for incremental building.
// Use this when you need to build commands conditionally.
//
// Example:
//
//	cmd := graph.Cmd()
//	if turn >= maxTurns {
//	    return cmd.To(graph.END)
//	}
//
//	resp, err := model.Generate(ctx, req)
//	if err != nil {
//	    return graph.Fail(err)
//	}
//
//	cmd.With(graph.SetValue(TurnKey, turn+1))
//	cmd.With(graph.AppendValue(MessagesKey, resp.Message))
//	return cmd.To("next")
func Cmd() *CommandBuilder {
	return &CommandBuilder{updates: make(Updates)}
}

// With applies a type-safe update function to the builder.
// Use with SetValue, AppendValue for compile-time type checking.
//
// Example:
//
//	cmd.With(graph.SetValue(statusKey, "done"))   // ✅ Type-safe
//	cmd.With(graph.SetValue(statusKey, 42))       // ❌ Compile error
func (b *CommandBuilder) With(fn func(*CommandBuilder) *CommandBuilder) *CommandBuilder {
	return fn(b)
}

// SetValue returns a type-safe update function for use with With().
// The generic parameter T is inferred from Key[T], ensuring type safety.
//
// Example:
//
//	cmd.With(graph.SetValue(statusKey, "done"))   // ✅ Type-safe
//	cmd.With(graph.SetValue(statusKey, 42))       // ❌ Compile error
func SetValue[T any](key Key[T], value T) func(*CommandBuilder) *CommandBuilder {
	return func(b *CommandBuilder) *CommandBuilder {
		b.updates[key.Name()] = value
		return b
	}
}

// AppendValue returns a type-safe append function for use with With().
// The generic parameter T is inferred from ListKey[T].
// The result is wrapped in SliceOf for zero-reflection iteration.
//
// Example:
//
//	cmd.With(graph.AppendValue(messagesKey, msg1, msg2))
func AppendValue[T any](key ListKey[T], values ...T) func(*CommandBuilder) *CommandBuilder {
	return func(b *CommandBuilder) *CommandBuilder {
		// Get existing list or create new one
		var list []T
		if existing, ok := b.updates[key.Name()]; ok {
			switch v := existing.(type) {
			case SliceOf[T]:
				list = []T(v)
			case []T:
				list = v
			}
		}
		list = append(list, values...)
		// Wrap in SliceOf for zero-reflection iteration
		b.updates[key.Name()] = SliceOf[T](list)
		return b
	}
}

// Set creates a CommandBuilder with a typed value (starter function).
// The generic parameter T is inferred from Key[T], ensuring type safety.
//
// Example:
//
//	graph.Set(statusKey, "done").To("next")       // ✅ Type-safe
func Set[T any](key Key[T], value T) *CommandBuilder {
	return &CommandBuilder{
		updates: Updates{key.Name(): value},
	}
}

// Append creates a CommandBuilder that appends values to a list key (starter function).
// The values are wrapped in SliceOf for zero-reflection iteration.
//
// Example:
//
//	graph.Append(messagesKey, msg1, msg2).To("next")
func Append[T any](key ListKey[T], values ...T) *CommandBuilder {
	return &CommandBuilder{
		updates: Updates{key.Name(): SliceOf[T](values)},
	}
}

// To creates a Command with the accumulated updates and specified targets.
//
// Example:
//
//	return graph.Set(key, val).To("next")
func (b *CommandBuilder) To(targets ...string) (*Command, error) {
	return &Command{Updates: b.updates, Next: targets}, nil
}

// End is shorthand for To(END).
//
// Example:
//
//	return graph.Set(key, val).End()
func (b *CommandBuilder) End() (*Command, error) {
	return b.To(END)
}
