package state

import (
	"github.com/hupe1980/agentmesh/pkg/message"
)

// MessagesKey is the standard key for storing conversation messages in state.
// Messages are stored as ExecutionResult to preserve metadata (node, timestamp, etc.).
// Use 0 for unbounded message history, or a positive number to limit history.
var MessagesKey = NewListKey[ExecutionResult]("__messages__", 0)

// GetMessages retrieves the message history from a ReadView.
// Returns an empty slice if no messages exist.
func GetMessages(view *ReadView) []ExecutionResult {
	return GetFromView(view, MessagesKey.Key)
}

// AppendMessages adds new messages to the message history in updates.
// This stores the slice directly - State.ApplyUpdates will append to existing messages.
func AppendMessages(updates Updates, messages []ExecutionResult) {
	if len(messages) == 0 {
		return
	}
	// Store the slice directly - ApplyUpdates handles appending
	updates[MessagesKey.Name()] = messages
}

// ExtractMessageContent extracts just the message.Message from ExecutionResults.
// Useful when you need the raw messages without metadata.
func ExtractMessageContent(results []ExecutionResult) []message.Message {
	if len(results) == 0 {
		return nil
	}
	messages := make([]message.Message, len(results))
	for i, r := range results {
		messages[i] = r.Message
	}
	return messages
}

// LastMessage returns the last message from the history, or nil if empty.
func LastMessage(view *ReadView) *ExecutionResult {
	msgs := GetMessages(view)
	if len(msgs) == 0 {
		return nil
	}
	return &msgs[len(msgs)-1]
}

// LastMessageContent returns just the message content of the last message, or nil if empty.
func LastMessageContent(view *ReadView) message.Message {
	last := LastMessage(view)
	if last == nil {
		return nil
	}
	return last.Message
}
