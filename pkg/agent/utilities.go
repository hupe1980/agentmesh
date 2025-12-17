package agent

import (
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

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
func IsConversationalContext(scope graph.Scope) bool {
	msgs := scope.Messages()

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
