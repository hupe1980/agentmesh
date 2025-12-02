package agent

import (
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// MessagesKey is the standard key for storing conversation messages in agent state.
// This is an alias to message.MessagesKey for convenient use in the agent package.
var MessagesKey = message.MessagesKey

// GetMessages retrieves the message history from a View.
// Returns an empty slice if no messages exist.
//
// This is a convenience wrapper around message.GetMessages for use in agent code.
//
// Example:
//
//	messages := agent.GetMessages(view)
//	for _, msg := range messages {
//	    fmt.Println(msg.Content())
//	}
func GetMessages(view graph.View) []message.Message {
	return message.GetMessages(view)
}

// LastMessage returns the last message from the history, or nil if empty.
//
// This is a convenience wrapper around message.LastMessage for use in agent code.
//
// Example:
//
//	lastMsg := agent.LastMessage(view)
//	if lastMsg != nil {
//	    fmt.Println("Last:", lastMsg.Content())
//	}
func LastMessage(view graph.View) message.Message {
	return message.LastMessage(view)
}
