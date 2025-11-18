package exec

import (
	"github.com/hupe1980/agentmesh/pkg/message"
)

// StateMessage wraps messages with state updates for distributed synchronization.
// This enables distributing state changes across nodes when using a distributed
// message bus (e.g., Redis).
//
// When enabled, each node's state updates are serialized into StateMessage
// and sent through the message bus. Remote nodes receive and apply these updates
// to maintain consistent distributed state.
type StateMessage struct {
	// Messages contains the messages
	Messages []message.Message `json:"messages,omitempty"`

	// Updates contains key-value state updates to be applied
	Updates map[string]any `json:"updates,omitempty"`

	// Metadata contains additional routing or processing hints
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NewStateMessage creates a new state message with messages and updates.
func NewStateMessage(messages []message.Message, updates map[string]any) StateMessage {
	return StateMessage{
		Messages: messages,
		Updates:  updates,
		Metadata: make(map[string]string),
	}
}

// WithMetadata adds metadata to the state message.
func (sm StateMessage) WithMetadata(key, value string) StateMessage {
	if sm.Metadata == nil {
		sm.Metadata = make(map[string]string)
	}
	sm.Metadata[key] = value
	return sm
}

// ToMessage converts StateMessage to a generic message.Message.
// The StateMessage is embedded as structured data in the message content.
func (sm StateMessage) ToMessage() message.Message {
	// Create a system message containing the state update data
	// This allows state synchronization to work with the existing message infrastructure
	data := map[string]any{
		"__type":   "state_message",
		"messages": sm.Messages,
		"updates":  sm.Updates,
		"metadata": sm.Metadata,
	}
	parts := message.Parts{
		message.DataPart{Data: data},
	}
	return message.NewSystemMessage(parts)
}

// FromMessage extracts StateMessage from a message.Message.
// Returns nil if the message doesn't contain a StateMessage.
func FromMessage(msg message.Message) *StateMessage {
	parts := msg.Parts()
	if len(parts) == 0 {
		return nil
	}

	// Check if first part is a DataPart containing StateMessage
	//nolint:nestif // State message deserialization requires nested type checking
	if dataPart, ok := parts[0].(message.DataPart); ok {
		if typeVal, ok := dataPart.Data["__type"].(string); ok && typeVal == "state_message" {
			sm := &StateMessage{}

			if messages, ok := dataPart.Data["messages"].([]message.Message); ok {
				sm.Messages = messages
			}

			if updates, ok := dataPart.Data["updates"].(map[string]any); ok {
				sm.Updates = updates
			}

			if metadata, ok := dataPart.Data["metadata"].(map[string]string); ok {
				sm.Metadata = metadata
			}

			return sm
		}
	}

	return nil
}
