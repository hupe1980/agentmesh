package graph

import "github.com/hupe1980/agentmesh/pkg/message"

// MessagesKeyName is the string name used for the messages state key.
// Use this constant when initializing state maps in tests or when accessing
// the raw state map.
const MessagesKeyName = "messages"

// messagesKey is the internal state key for storing conversation messages in graph state.
// Messages are stored as message.Message instances in append-only fashion.
// Use Reply/ReplyAll commands or scope.Messages() to access messages.
var messagesKey = NewListKey[message.Message](MessagesKeyName)

// GetMessages retrieves the message history from a ReadOnlyScope.
// This is a convenience function - you can also use scope.Messages() directly.
func GetMessages(scope ReadOnlyScope) []message.Message {
	return scope.Messages()
}

// LastMessage returns the last message from the history, or nil if empty.
func LastMessage(scope ReadOnlyScope) message.Message {
	msgs := scope.Messages()
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}
