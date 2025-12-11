package message

import "github.com/hupe1980/agentmesh/pkg/graph"

// MessagesKey is the standard state key for storing conversation messages in graph state.
// Messages are stored as Message instances in append-only fashion.
//
// This key should be used consistently across agents and graph workflows to ensure
// proper message history tracking and state management.
//
// Example:
//
//	g := graph.New[[]message.Message, message.Message](message.MessagesKey)
//	g.Node("process", func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
//	    messages := message.GetMessages(scope)
//	    // Process messages...
//	    return graph.Append(message.MessagesKey, newMessage).End()
//	})
var MessagesKey = graph.NewListKey[Message]("messages")

// GetMessages retrieves the message history from a ReadOnlyScope.
// Returns an empty slice if no messages exist.
//
// Example:
//
//	messages := message.GetMessages(scope)
//	for _, msg := range messages {
//	    fmt.Println(msg.Content())
//	}
func GetMessages(scope graph.ReadOnlyScope) []Message {
	return graph.GetList(scope, MessagesKey)
}

// LastMessage returns the last message from the history, or nil if empty.
//
// Example:
//
//	lastMsg := message.LastMessage(scope)
//	if lastMsg != nil {
//	    fmt.Println("Last:", lastMsg.Content())
//	}
func LastMessage(scope graph.ReadOnlyScope) Message {
	msgs := GetMessages(scope)
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}
