package agent

import (
	"iter"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// MessagesKey is the standard key for storing conversation messages in agent state.
// This is the agent-layer equivalent of what was previously in pkg/state.
// Messages are stored as message.Message instances in append-only fashion.
// Use 0 for unbounded message history, or a positive number to limit history.
var MessagesKey = state.NewListKey[message.Message]("__messages__", 0)

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
// This stores the slice directly - Manager.ApplyUpdates will append to existing messages.
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
	// Store the slice directly - ApplyUpdates handles appending
	updates[MessagesKey.Name()] = messages
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
func RegisterMessagesKey(mgr state.Manager) error {
	return state.RegisterListKey(mgr, MessagesKey)
}

// CollectMessages collects all messages from an iterator sequence.
// This is a convenience function for message-specific agent execution.
// Skips nil messages for backward compatibility.
//
// Example:
//
//	messages, err := agent.CollectMessages(agent.Run(ctx, messages))
//	if err != nil {
//	    return err
//	}
func CollectMessages(seq iter.Seq2[message.Message, error]) ([]message.Message, error) {
	messages := make([]message.Message, 0)
	for msg, err := range seq {
		if err != nil {
			return messages, err
		}
		// Skip nil messages for backward compatibility
		if msg != nil {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}
