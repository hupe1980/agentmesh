package agent

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// MessageState provides message management on top of state.Manager.
// It wraps the core state system with message-specific operations,
// keeping the core state system domain-agnostic while providing
// convenient message handling for agent use cases.
type MessageState struct {
	manager *state.Manager
	key     state.ListKey[message.Message]

	// Optional execution context tracking
	runID   string
	graphID string
}

// MessageStateOption configures MessageState behavior.
type MessageStateOption func(*MessageState)

// WithMaxMessages limits the message history to the most recent N messages.
// A value of 0 means unbounded (default).
//
// Example:
//
//	ms := NewMessageState(mgr, WithMaxMessages(100)) // Keep last 100 messages
func WithMaxMessages(max int) MessageStateOption {
	return func(ms *MessageState) {
		ms.key = state.NewListKey[message.Message]("messages", max)
	}
}

// WithRunID sets the execution run ID for context tracking.
// This is optional metadata that can be used for debugging or monitoring.
func WithRunID(runID string) MessageStateOption {
	return func(ms *MessageState) {
		ms.runID = runID
	}
}

// WithGraphID sets the graph ID for context tracking.
// This is optional metadata that can be used for debugging or monitoring.
func WithGraphID(graphID string) MessageStateOption {
	return func(ms *MessageState) {
		ms.graphID = graphID
	}
}

// NewMessageState creates a message-aware state wrapper around a state.Manager.
//
// The MessageState provides convenient message operations while keeping the
// underlying state system generic. Messages are stored as pure message.Message
// values without execution metadata pollution.
//
// Example:
//
//	mgr := state.NewManager()
//	ms := NewMessageState(mgr, WithMaxMessages(100))
//
//	// Append messages
//	ms.AppendMessages(ctx, []message.Message{
//	    message.NewHumanMessageFromText("Hello"),
//	})
//
//	// Read messages
//	view, _ := mgr.CreateReadView(ctx)
//	messages := ms.GetMessages(view)
func NewMessageState(manager *state.Manager, opts ...MessageStateOption) *MessageState {
	ms := &MessageState{
		manager: manager,
		key:     state.NewListKey[message.Message]("messages", 0), // Default: unbounded
	}

	// Apply options
	for _, opt := range opts {
		opt(ms)
	}

	// Register the messages key with the manager
	if err := state.RegisterListKey(manager, ms.key); err != nil {
		// Key might already be registered, which is fine
		// Continue silently to support multiple MessageState instances
		// sharing the same manager
	}

	return ms
}

// Manager returns the underlying state manager.
// This allows access to the full state.Manager API when needed.
func (ms *MessageState) Manager() *state.Manager {
	return ms.manager
}

// AppendMessages adds messages to the state.
// Messages are appended to the message history respecting the max messages limit.
// The messages are stored in a TopicChannel which accumulates values via append semantics.
//
// Example:
//
//	err := ms.AppendMessages(ctx, []message.Message{
//	    message.NewHumanMessageFromText("Hello"),
//	    message.NewAIMessageFromText("Hi there!"),
//	})
func (ms *MessageState) AppendMessages(ctx context.Context, messages []message.Message) error {
	if len(messages) == 0 {
		return nil
	}

	// Store the slice directly in updates - ApplyUpdates will append it
	// because ms.key is registered as a ListKey (TopicChannel semantics)
	updates := state.NewUpdates()
	updates[ms.key.Name()] = messages
	return state.ApplyUpdates(ctx, ms.manager, updates)
}

// AppendMessage adds a single message to the state.
// Convenience wrapper around AppendMessages for single message operations.
//
// Example:
//
//	err := ms.AppendMessage(ctx, message.NewHumanMessageFromText("Hello"))
func (ms *MessageState) AppendMessage(ctx context.Context, msg message.Message) error {
	return ms.AppendMessages(ctx, []message.Message{msg})
}

// GetMessages retrieves all messages from the state.
// Returns an empty slice if no messages exist.
// Reads from the TopicChannel which accumulates all appended messages.
//
// Example:
//
//	view, _ := ms.Manager().CreateReadView(ctx)
//	messages := ms.GetMessages(view)
//	for _, msg := range messages {
//	    fmt.Println(msg.Content())
//	}
func (ms *MessageState) GetMessages(view *state.ReadView) []message.Message {
	// Use the embedded Key from ListKey which has the correct type []message.Message
	return state.GetFromView(view, ms.key.Key)
}

// LastMessage returns the most recent message from the state.
// Returns nil if no messages exist.
//
// Example:
//
//	view, _ := ms.Manager().CreateReadView(ctx)
//	lastMsg := ms.LastMessage(view)
//	if lastMsg != nil {
//	    fmt.Println("Last:", lastMsg.Content())
//	}
func (ms *MessageState) LastMessage(view *state.ReadView) message.Message {
	messages := ms.GetMessages(view)
	if len(messages) == 0 {
		return nil
	}
	return messages[len(messages)-1]
}

// MessageCount returns the number of messages in the state.
//
// Example:
//
//	view, _ := ms.Manager().CreateReadView(ctx)
//	count := ms.MessageCount(view)
//	fmt.Printf("Total messages: %d\n", count)
func (ms *MessageState) MessageCount(view *state.ReadView) int {
	return len(ms.GetMessages(view))
}

// ClearMessages removes all messages from the state.
// This resets the TopicChannel to empty, clearing all accumulated messages.
//
// Example:
//
//	err := ms.ClearMessages(ctx)
func (ms *MessageState) ClearMessages(ctx context.Context) error {
	// Use ResetInManager to clear the TopicChannel
	return state.ResetInManager(ctx, ms.manager, ms.key.Name())
}

// RunID returns the optional run ID if set.
func (ms *MessageState) RunID() string {
	return ms.runID
}

// GraphID returns the optional graph ID if set.
func (ms *MessageState) GraphID() string {
	return ms.graphID
}
