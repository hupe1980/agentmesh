package agent

import (
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// MessagesKey is the standard key for storing conversation messages in agent state.
// This is an alias to graph.MessagesKey for convenient use in the agent package.
var MessagesKey = graph.MessagesKey

// GetMessages retrieves the message history from a View.
// Returns an empty slice if no messages exist.
//
// Example:
//
//	messages := agent.GetMessages(view)
//	for _, msg := range messages {
//	    fmt.Println(msg.Content())
//	}
func GetMessages(view graph.View) []message.Message {
	return graph.GetList(view, MessagesKey)
}

// LastMessage returns the last message from the history, or nil if empty.
//
// Example:
//
//	lastMsg := agent.LastMessage(view)
//	if lastMsg != nil {
//	    fmt.Println("Last:", lastMsg.Content())
//	}
func LastMessage(view graph.View) message.Message {
	msgs := GetMessages(view)
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}
