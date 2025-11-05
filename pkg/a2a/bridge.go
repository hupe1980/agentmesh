package a2a

import (
	"encoding/json"
	"fmt"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/hupe1980/agentmesh/pkg/message"
)

const roleAgent = "agent"

// ConvertToA2AMessage converts an AgentMesh message to an A2A message.
func ConvertToA2AMessage(msg message.Message) (*a2atypes.Message, error) {
	var role a2atypes.MessageRole
	var parts []a2atypes.Part

	switch m := msg.(type) {
	case *message.SystemMessage:
		role = "system"
		for _, part := range m.Parts() {
			if textPart, ok := part.(message.TextPart); ok {
				parts = append(parts, a2atypes.TextPart{Text: textPart.Text})
			}
		}

	case *message.HumanMessage:
		role = "user"
		for _, part := range m.Parts() {
			if textPart, ok := part.(message.TextPart); ok {
				parts = append(parts, a2atypes.TextPart{Text: textPart.Text})
			}
		}

	case *message.AIMessage:
		role = roleAgent
		for _, part := range m.Parts() {
			switch p := part.(type) {
			case message.TextPart:
				parts = append(parts, a2atypes.TextPart{Text: p.Text})
			case message.FunctionCallPart:
				// Convert function call to A2A data part
				parts = append(parts, a2atypes.DataPart{
					Data: map[string]any{
						"name":      p.FunctionCall.Name,
						"arguments": p.FunctionCall.Arguments,
					},
				})
			}
		}
		// Handle tool calls if present
		for _, toolCall := range m.ToolCalls {
			parts = append(parts, a2atypes.DataPart{
				Data: map[string]any{
					"name":      toolCall.Name,
					"arguments": toolCall.Arguments,
					"type":      toolCall.Type,
				},
			})
		}

	case *message.ToolMessage:
		// Tool results can be represented as agent messages with data
		role = roleAgent
		// Extract text from parts
		textContent := ""
		for _, part := range m.Parts() {
			if textPart, ok := part.(message.TextPart); ok {
				textContent += textPart.Text
			}
		}
		parts = append(parts, a2atypes.DataPart{
			Data: map[string]any{
				"toolCallID": m.ToolCallID,
				"result":     textContent,
			},
		})

	case *message.FunctionMessage:
		// Function results as agent messages
		role = "agent"
		textContent := ""
		for _, part := range m.Parts() {
			if textPart, ok := part.(message.TextPart); ok {
				textContent += textPart.Text
			}
		}
		parts = append(parts, a2atypes.DataPart{
			Data: map[string]any{
				"function": m.Name,
				"result":   textContent,
			},
		})

	default:
		return nil, fmt.Errorf("unsupported message type: %T", msg)
	}

	return a2atypes.NewMessage(role, parts...), nil
}

// ConvertFromA2AMessage converts an A2A message to AgentMesh messages.
func ConvertFromA2AMessage(a2aMsg *a2atypes.Message) ([]message.Message, error) {
	var messages []message.Message

	switch a2aMsg.Role {
	case "system":
		var textParts []message.Part
		for _, part := range a2aMsg.Parts {
			if textPart, ok := part.(a2atypes.TextPart); ok {
				textParts = append(textParts, message.TextPart{Text: textPart.Text})
			}
		}
		if len(textParts) > 0 {
			messages = append(messages, message.NewSystemMessage(textParts))
		}

	case "user":
		var textParts []message.Part
		for _, part := range a2aMsg.Parts {
			if textPart, ok := part.(a2atypes.TextPart); ok {
				textParts = append(textParts, message.TextPart{Text: textPart.Text})
			}
		}
		if len(textParts) > 0 {
			messages = append(messages, message.NewHumanMessage(textParts))
		}

	case "agent":
		var parts []message.Part
		for _, part := range a2aMsg.Parts {
			switch p := part.(type) {
			case a2atypes.TextPart:
				parts = append(parts, message.TextPart{Text: p.Text})
			case a2atypes.DataPart:
				// Convert data parts to text representation for now
				dataJSON, err := json.Marshal(p.Data)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal data part: %w", err)
				}
				parts = append(parts, message.TextPart{
					Text: fmt.Sprintf("[Data: %s]", string(dataJSON)),
				})
			case a2atypes.FilePart:
				// Convert file parts to text representation
				parts = append(parts, message.TextPart{
					Text: "[File]",
				})
			}
		}
		if len(parts) > 0 {
			messages = append(messages, message.NewAIMessage(parts))
		}

	default:
		return nil, fmt.Errorf("unsupported A2A message role: %s", a2aMsg.Role)
	}

	return messages, nil
}

// ConvertMessagesToA2A converts a slice of AgentMesh messages to A2A messages.
func ConvertMessagesToA2A(messages []message.Message) ([]*a2atypes.Message, error) {
	a2aMessages := make([]*a2atypes.Message, 0, len(messages))

	for _, msg := range messages {
		a2aMsg, err := ConvertToA2AMessage(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}
		a2aMessages = append(a2aMessages, a2aMsg)
	}

	return a2aMessages, nil
}

// ConvertMessagesFromA2A converts a slice of A2A messages to AgentMesh messages.
func ConvertMessagesFromA2A(a2aMessages []*a2atypes.Message) ([]message.Message, error) {
	var messages []message.Message

	for _, a2aMsg := range a2aMessages {
		msgs, err := ConvertFromA2AMessage(a2aMsg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert A2A message: %w", err)
		}
		messages = append(messages, msgs...)
	}

	return messages, nil
}

// ExtractTextContent extracts all text content from an A2A message.
func ExtractTextContent(a2aMsg *a2atypes.Message) string {
	var text string
	for _, part := range a2aMsg.Parts {
		if textPart, ok := part.(a2atypes.TextPart); ok {
			text += textPart.Text
		}
	}
	return text
}

// ExtractTextFromMessages extracts all text content from AgentMesh messages.
func ExtractTextFromMessages(messages []message.Message) string {
	var text string
	for _, msg := range messages {
		switch m := msg.(type) {
		case *message.SystemMessage:
			for _, part := range m.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					text += textPart.Text + "\n"
				}
			}
		case *message.HumanMessage:
			for _, part := range m.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					text += textPart.Text + "\n"
				}
			}
		case *message.AIMessage:
			for _, part := range m.Parts() {
				if textPart, ok := part.(message.TextPart); ok {
					text += textPart.Text + "\n"
				}
			}
		}
	}
	return text
}
