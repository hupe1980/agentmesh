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
//	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
//	    messages := message.GetMessages(view)
//	    // Process messages...
//	    return graph.Append(message.MessagesKey, newMessage).End()
//	})
var MessagesKey = graph.NewListKey[Message]("messages")

// GetMessages retrieves the message history from a View.
// Returns an empty slice if no messages exist.
//
// Example:
//
//	messages := message.GetMessages(view)
//	for _, msg := range messages {
//	    fmt.Println(msg.Content())
//	}
func GetMessages(view graph.View) []Message {
	return graph.GetList(view, MessagesKey)
}

// LastMessage returns the last message from the history, or nil if empty.
//
// Example:
//
//	lastMsg := message.LastMessage(view)
//	if lastMsg != nil {
//	    fmt.Println("Last:", lastMsg.Content())
//	}
func LastMessage(view graph.View) Message {
	msgs := GetMessages(view)
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}
