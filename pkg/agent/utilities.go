package agent

import (
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// MessagesKey is the standard key for storing conversation messages in agent state.
// This is an alias to message.MessagesKey for convenient use in the agent package.
var MessagesKey = message.MessagesKey

// GetMessages retrieves the message history from a Scope.
// Returns an empty slice if no messages exist.
//
// This is a convenience wrapper around graph.GetList for use in agent code.
//
// Example:
//
//	messages := agent.GetMessages(scope)
//	for _, msg := range messages {
//	    fmt.Println(msg.Content())
//	}
func GetMessages(scope message.Scope) []message.Message {
	return graph.GetList(scope, MessagesKey)
}

// LastMessage returns the last message from the history, or nil if empty.
//
// This is a convenience wrapper for use in agent code.
//
// Example:
//
//	lastMsg := agent.LastMessage(scope)
//	if lastMsg != nil {
//	    fmt.Println("Last:", lastMsg.Content())
//	}
func LastMessage(scope message.Scope) message.Message {
	msgs := GetMessages(scope)
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}
