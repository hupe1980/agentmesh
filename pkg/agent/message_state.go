package agent

import (
	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// MessagesKey is the standard key for storing conversation messages in agent state.
// This is the agent-layer equivalent of what was previously in pkg/state.
// Messages are stored as message.Message instances in append-only fashion.
// Use 0 for unbounded message history, or a positive number to limit history.
var MessagesKey = state.NewListKey[message.Message](graph.MessagesKeyName, 0)

// GetMessages retrieves the message history from a ReadView.
// Returns an empty slice if no messages exist.
//
// Example:
//
//	view, _ := mgr.CreateReadView(ctx)
//	messages := agent.GetMessages(view)
//	for _, msg := range messages {
//	    fmt.Println(msg.Content())
//	}
func GetMessages(view *state.ReadView) []message.Message {
	return state.GetFromView(view, MessagesKey.Key)
}

// AppendMessages adds new messages to the message history in updates.
// Wraps the messages slice in SliceOf[T] for proper channel handling.
//
// Example:
//
//	updates := state.NewUpdates()
//	agent.AppendMessages(updates, []message.Message{
//	    message.NewHumanMessageFromText("Hello"),
//	    message.NewAIMessageFromText("Hi there!"),
//	})
//	mgr.ApplyUpdates(ctx, updates)
func AppendMessages(updates state.Updates, messages []message.Message) {
	if len(messages) == 0 {
		return
	}
	// Wrap in SliceOf[T] so the channel recognizes it as a slice to append
	updates[MessagesKey.Name()] = channel.SliceOf[message.Message](messages)
}

// LastMessage returns the last message from the history, or nil if empty.
//
// Example:
//
//	view, _ := mgr.CreateReadView(ctx)
//	lastMsg := agent.LastMessage(view)
//	if lastMsg != nil {
//	    fmt.Println("Last:", lastMsg.Content())
//	}
func LastMessage(view *state.ReadView) message.Message {
	msgs := GetMessages(view)
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}

// RegisterMessagesKey registers the MessagesKey with a state manager.
// This should be called during agent initialization to ensure the messages
// channel is properly set up in the state system.
//
// Example:
//
//	mgr := state.NewManager()
//	if err := agent.RegisterMessagesKey(mgr); err != nil {
//	    return err
//	}
func RegisterMessagesKey(mgr *state.Manager) error {
	return state.RegisterListKey(mgr, MessagesKey)
}
