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

// IsConversationalContext checks if the current execution has conversation history.
// Returns true if query rephrasing or context-aware processing would be beneficial.
//
// Detection is based on:
//   - Presence of AI responses (indicates prior exchange)
//   - Multiple human messages (indicates multi-turn conversation)
//   - Memory context from Conversational wrapper
//
// Example:
//
//	if agent.IsConversationalContext(scope) {
//	    // Handle as follow-up question
//	} else {
//	    // Handle as standalone query
//	}
func IsConversationalContext(scope message.Scope) bool {
	msgs := GetMessages(scope)

	// Count human and AI messages to detect conversation
	humanCount := 0
	aiCount := 0

	for _, msg := range msgs {
		switch msg.Type() {
		case message.TypeHuman:
			humanCount++
		case message.TypeAI:
			aiCount++
		}
	}

	// Conversation exists if there's prior exchange (at least 1 AI response)
	// or multiple human messages
	if aiCount > 0 || humanCount > 1 {
		return true
	}

	// Check if Conversational wrapper injected memory context
	if len(graph.Get(scope, MemoryContextKey)) > 0 {
		return true
	}

	return false
}

// GetConversationHistory extracts prior messages for context.
// Returns messages excluding the current (last) human query.
//
// This is useful for rephrasing queries or providing conversation context
// to models that need to understand the full conversation.
//
// Example:
//
//	history := agent.GetConversationHistory(messages)
//	// history contains all messages except the last human query
func GetConversationHistory(msgs []message.Message) []message.Message {
	if len(msgs) <= 1 {
		return nil
	}

	// Find the last human message index
	lastHumanIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type() == message.TypeHuman {
			lastHumanIdx = i
			break
		}
	}

	if lastHumanIdx <= 0 {
		return nil
	}

	// Return everything before the last human message
	return msgs[:lastHumanIdx]
}
