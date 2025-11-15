package graph

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// CreateInputConverter creates a type-safe converter from I to []message.Message.
// This is the public API for creating input converters.
func CreateInputConverter[I any]() func(I) ([]message.Message, error) {
	return createInputConverter[I]()
}

// CreateOutputConverter creates a type-safe converter from state.ExecutionResult to O.
// This is the public API for creating output converters.
func CreateOutputConverter[O any]() func(state.ExecutionResult) (O, error) {
	return createOutputConverter[O]()
}

// extractTextFromMessage extracts text content from a message's parts.
func extractTextFromMessage(msg message.Message) string {
	if msg == nil {
		return ""
	}

	var texts []string
	for _, part := range msg.Parts() {
		if textPart, ok := part.(message.TextPart); ok {
			texts = append(texts, textPart.Text)
		}
	}

	return strings.Join(texts, "")
}

// createInputConverter creates a type-safe converter from I to []message.Message.
// For common input types, it returns optimized converters with zero overhead.
func createInputConverter[I any]() func(I) ([]message.Message, error) {
	var zero I
	inputType := reflect.TypeOf(zero)

	// Check if I is already []message.Message (most common case)
	messagesType := reflect.TypeOf([]message.Message{})
	if inputType == messagesType {
		return func(input I) ([]message.Message, error) {
			// Type assertion is safe here - we verified the type
			return any(input).([]message.Message), nil
		}
	}

	// Check if I is map[string]any (state-based graphs)
	stateType := reflect.TypeOf(map[string]any{})
	if inputType == stateType {
		return func(input I) ([]message.Message, error) {
			// Extract messages from state map if present
			stateMap := any(input).(map[string]any)
			if msgs, ok := stateMap["messages"]; ok {
				if messages, ok := msgs.([]message.Message); ok {
					return messages, nil
				}
			}
			// No messages in state - return empty slice
			return []message.Message{}, nil
		}
	}

	// Check if I is string (text-based graphs)
	stringType := reflect.TypeOf("")
	if inputType == stringType {
		return func(input I) ([]message.Message, error) {
			text := any(input).(string)
			return []message.Message{message.NewHumanMessageFromText(text)}, nil
		}
	}

	// Generic fallback for other types
	return func(input I) ([]message.Message, error) {
		return nil, fmt.Errorf("cannot convert %T to []message.Message (unsupported input type)", input)
	}
}

// createOutputConverter creates a type-safe converter from state.ExecutionResult to O.
// For common output types, it returns optimized converters with zero overhead.
func createOutputConverter[O any]() func(state.ExecutionResult) (O, error) {
	var zero O
	outputType := reflect.TypeOf(zero)

	// Check if O is already state.ExecutionResult (most common case)
	resultType := reflect.TypeOf(state.ExecutionResult{})
	if outputType == resultType {
		return func(result state.ExecutionResult) (O, error) {
			// Type assertion is safe here - we verified the type
			return any(result).(O), nil
		}
	}

	// Check if O is message.Message interface type
	// Note: message.Message is an interface, so we check if we're asking for an interface type
	var msgInterface message.Message
	if outputType == reflect.TypeOf((*message.Message)(nil)).Elem() {
		return func(result state.ExecutionResult) (O, error) {
			if result.Message == nil {
				return zero, fmt.Errorf("no message in execution result")
			}
			// Return the message
			return any(result.Message).(O), nil
		}
	}

	// Check if O is string (text output)
	stringType := reflect.TypeOf("")
	if outputType == stringType {
		return func(result state.ExecutionResult) (O, error) {
			if result.Message == nil {
				return any("").(O), nil
			}
			// Extract text from message parts
			text := extractTextFromMessage(result.Message)
			return any(text).(O), nil
		}
	}

	// Check if O is map[string]any (state output)
	stateType := reflect.TypeOf(map[string]any{})
	if outputType == stateType {
		return func(result state.ExecutionResult) (O, error) {
			// Convert ExecutionResult to state map
			stateMap := make(map[string]any)
			if result.Message != nil {
				stateMap["message"] = result.Message
			}
			if result.Updates != nil {
				for k, v := range result.Updates {
					stateMap[k] = v
				}
			}
			stateMap["node"] = result.Node
			stateMap["graph_id"] = result.GraphID
			return any(stateMap).(O), nil
		}
	}

	_ = msgInterface // Avoid unused variable

	// Generic fallback for other types
	return func(result state.ExecutionResult) (O, error) {
		return zero, fmt.Errorf("cannot convert state.ExecutionResult to %T (unsupported output type)", zero)
	}
}
