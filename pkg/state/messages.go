package state

import (
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Deprecated: MessagesKey has been moved to pkg/agent.MessagesKey.
// This remains here for backward compatibility but will be removed in a future version.
// Use agent.MessagesKey instead for new code.
//
// MessagesKey is the standard key for storing conversation messages in state.
// Messages are stored as message.Message instances in append-only fashion.
// Use 0 for unbounded message history, or a positive number to limit history.
var MessagesKey = NewListKey[message.Message]("__messages__", 0)

// Deprecated: GetMessages has been moved to pkg/agent.GetMessages.
// This remains here for backward compatibility but will be removed in a future version.
// Use agent.GetMessages instead for new code.
//
// GetMessages retrieves the message history from a ReadView.
// Returns an empty slice if no messages exist.
func GetMessages(view *ReadView) []message.Message {
	return GetFromView(view, MessagesKey.Key)
}

// Deprecated: AppendMessages has been moved to pkg/agent.AppendMessages.
// This remains here for backward compatibility but will be removed in a future version.
// Use agent.AppendMessages instead for new code.
//
// AppendMessages adds new messages to the message history in updates.
// This stores the slice directly - ApplyUpdates will append to existing messages.
func AppendMessages(updates Updates, messages []message.Message) {
	if len(messages) == 0 {
		return
	}
	// Store the slice directly - ApplyUpdates handles appending
	updates[MessagesKey.Name()] = messages
}

// LastMessage returns the last message from the history, or nil if empty.
func LastMessage(view *ReadView) message.Message {
	msgs := GetMessages(view)
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}

// LastMessageContent returns the last message from history, or nil if empty.
// This is an alias for LastMessage for backward compatibility.
func LastMessageContent(view *ReadView) message.Message {
	return LastMessage(view)
}

// ExtractMessageContent is a no-op pass-through for backward compatibility.
// Since messages are now stored directly as message.Message (not wrapped in ExecutionResult),
// this function simply returns the input slice unchanged.
func ExtractMessageContent(messages []message.Message) []message.Message {
	return messages
}
