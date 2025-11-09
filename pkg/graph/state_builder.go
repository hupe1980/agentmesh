package graph

import (
	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// StateBuilder provides a fluent API for constructing State with common channel patterns.
// It simplifies state initialization by providing high-level methods for typical use cases,
// eliminating the need to understand low-level channel mechanics.
//
// Example usage:
//
//	state := graph.NewStateBuilder().
//	    WithMessages(100).                    // Message history with limit
//	    WithLastValue("status", "pending").   // Latest-only channel
//	    WithCounter("iterations").            // Accumulating counter
//	    WithFlag("completed").                // Boolean flag
//	    WithList("results").                  // Append-only list
//	    Build()
//
// This eliminates the verbose pattern of:
//
//	state := graph.NewState(100)
//	state.AddChannel(channel.NewLastValueChannel("status"))
//	state.Set("status", "pending")
//	state.AddChannel(channel.NewBinaryOpChannel("iterations", 0, addFunc))
//	// ... and so on
type StateBuilder struct {
	maxMessages int
	channels    []channel.Channel
	initialVals map[string]any
	initialMsgs []message.Message
}

// NewStateBuilder creates a new state builder with sensible defaults.
// Default message limit is 100 (can be changed with WithMessages or WithUnlimitedMessages).
func NewStateBuilder() *StateBuilder {
	return &StateBuilder{
		maxMessages: 100, // Sensible default to prevent unbounded growth
		channels:    make([]channel.Channel, 0),
		initialVals: make(map[string]any),
		initialMsgs: nil,
	}
}

// WithMessages sets the message history limit for the "messages" channel.
// Use 0 for unlimited (not recommended for production).
//
// Example:
//
//	builder.WithMessages(50)  // Keep last 50 messages
func (b *StateBuilder) WithMessages(maxMessages int) *StateBuilder {
	b.maxMessages = maxMessages
	return b
}

// WithUnlimitedMessages removes the message history limit.
// Warning: This can lead to unbounded memory growth. Use with caution.
//
// Example:
//
//	builder.WithUnlimitedMessages()  // No message limit
func (b *StateBuilder) WithUnlimitedMessages() *StateBuilder {
	b.maxMessages = 0
	return b
}

// WithInitialMessages sets initial messages to be added to the state when built.
// This is useful for pre-populating system prompts, context, or conversation history.
//
// Example:
//
//	systemMsg := message.NewSystemMessageFromText("You are a helpful assistant")
//	builder.WithInitialMessages(systemMsg)
func (b *StateBuilder) WithInitialMessages(messages ...message.Message) *StateBuilder {
	b.initialMsgs = append(b.initialMsgs, messages...)
	return b
}

// WithLastValue adds a LastValueChannel that stores only the most recent value.
// Perfect for status fields, configuration values, or any state that only needs
// the latest value.
//
// Example:
//
//	builder.WithLastValue("status", "pending")
//	builder.WithLastValue("temperature", 72)
func (b *StateBuilder) WithLastValue(name string, initialValue any) *StateBuilder {
	ch := channel.NewLastValueChannel(name)
	b.channels = append(b.channels, ch)
	b.initialVals[name] = initialValue
	return b
}

// WithCounter adds a BinaryOpChannel that accumulates numeric values by addition.
// Useful for iteration counts, scores, or any accumulating metric.
//
// Example:
//
//	builder.WithCounter("iterations")      // Starts at 0
//	builder.WithCounter("score")           // Accumulates scores
func (b *StateBuilder) WithCounter(name string) *StateBuilder {
	addFunc := func(oldValue, newValue any) any {
		oldInt, _ := oldValue.(int)
		newInt, _ := newValue.(int)
		return oldInt + newInt
	}
	ch := channel.NewBinaryOpChannel(name, 0, addFunc)
	b.channels = append(b.channels, ch)
	return b
}

// WithFlag adds a LastValueChannel for boolean flags.
// Convenient for tracking completion state, validation results, etc.
//
// Example:
//
//	builder.WithFlag("completed")          // Starts false
//	builder.WithFlag("validated")          // Starts false
func (b *StateBuilder) WithFlag(name string) *StateBuilder {
	return b.WithLastValue(name, false)
}

// WithList adds a TopicChannel for accumulating a list of values.
// Values are appended and never overwritten, useful for logs, history, or results.
//
// Example:
//
//	builder.WithList("action_history")    // Track sequence of actions
//	builder.WithList("errors")            // Collect all errors
func (b *StateBuilder) WithList(name string) *StateBuilder {
	ch := channel.NewTopicChannel(name, 0) // Unlimited by default
	b.channels = append(b.channels, ch)
	return b
}

// WithListLimit adds a TopicChannel with a maximum size limit.
// When the limit is reached, oldest values are discarded (FIFO).
//
// Example:
//
//	builder.WithListLimit("recent_actions", 10)  // Keep last 10 actions
func (b *StateBuilder) WithListLimit(name string, maxValues int) *StateBuilder {
	ch := channel.NewTopicChannel(name, maxValues)
	b.channels = append(b.channels, ch)
	return b
}

// WithMap adds a BinaryOpChannel that merges map values.
// Useful for collecting results from parallel tasks or accumulating structured data.
//
// Example:
//
//	builder.WithMap("task_results")       // Merge results from parallel nodes
//	builder.WithMap("metadata")           // Accumulate metadata fields
func (b *StateBuilder) WithMap(name string) *StateBuilder {
	mergeFunc := func(oldValue, newValue any) any {
		oldMap, _ := oldValue.(map[string]any)
		newMap, _ := newValue.(map[string]any)
		if oldMap == nil && newMap == nil {
			return nil
		}
		merged := make(map[string]any)
		// Copy old values
		for k, v := range oldMap {
			merged[k] = v
		}
		// Merge new values (overwrites on conflict)
		for k, v := range newMap {
			merged[k] = v
		}
		return merged
	}
	ch := channel.NewBinaryOpChannel(name, map[string]any{}, mergeFunc)
	b.channels = append(b.channels, ch)
	return b
}

// WithBinaryOp adds a custom BinaryOpChannel with user-defined reducer function.
// Use this when the built-in channel types (Counter, Map, etc.) don't fit your needs.
//
// Example:
//
//	builder.WithBinaryOp("concatenated", "", func(old, new any) any {
//	    return old.(string) + new.(string)
//	})
func (b *StateBuilder) WithBinaryOp(name string, initialValue any, reducer func(any, any) any) *StateBuilder {
	ch := channel.NewBinaryOpChannel(name, initialValue, reducer)
	b.channels = append(b.channels, ch)
	return b
}

// WithChannel adds a custom channel implementation.
// Use this for advanced scenarios where you need full control over channel behavior.
//
// Example:
//
//	builder.WithChannel(myCustomChannel)
func (b *StateBuilder) WithChannel(ch channel.Channel) *StateBuilder {
	b.channels = append(b.channels, ch)
	return b
}

// Build constructs the final State with all configured channels and initial values.
// This creates the "messages" channel automatically with the configured limit.
//
// Example:
//
//	state := builder.Build()
func (b *StateBuilder) Build() *State {
	// Create state with configured message limit
	state := NewState(b.maxMessages)

	// Add all configured channels
	for _, ch := range b.channels {
		state.AddChannel(ch)
	}

	// Set initial values
	for name, value := range b.initialVals {
		_ = state.Set(name, value) // Ignore error - channel just created, won't fail
	}

	// Add initial messages if configured
	if len(b.initialMsgs) > 0 {
		state.AddMessages(b.initialMsgs)
	}

	return state
}
