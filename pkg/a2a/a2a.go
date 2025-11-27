package a2a

import (
	"context"
	"encoding/json"
	"fmt"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Client wraps an A2A client for use in AgentMesh workflows.
// It provides a clean interface for calling remote A2A agents with automatic message conversion.
type Client struct {
	client  *a2aclient.Client
	skillID string
	card    *a2atypes.AgentCard
}

// NewClient creates a new A2A client from an agent card URL.
// The skillID identifies which skill/capability of the agent to invoke.
func NewClient(ctx context.Context, agentCardURL string, skillID string, opts ...a2aclient.FactoryOption) (*Client, error) {
	if agentCardURL == "" {
		return nil, fmt.Errorf("agentCardURL cannot be empty")
	}
	if skillID == "" {
		return nil, fmt.Errorf("skillID cannot be empty")
	}

	// Resolve the agent card
	card, err := agentcard.DefaultResolver.Resolve(ctx, agentCardURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent card: %w", err)
	}

	return NewClientFromCard(ctx, card, skillID, opts...)
}

// NewClientFromCard creates a new A2A client from an existing agent card.
// The skillID identifies which skill/capability of the agent to invoke.
func NewClientFromCard(ctx context.Context, card *a2atypes.AgentCard, skillID string, opts ...a2aclient.FactoryOption) (*Client, error) {
	if err := validate.NotNil(card, "agent card"); err != nil {
		return nil, err
	}
	if skillID == "" {
		return nil, fmt.Errorf("skillID cannot be empty")
	}

	// Create A2A client
	client, err := a2aclient.NewFromCard(ctx, card, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}

	return &Client{
		client:  client,
		skillID: skillID,
		card:    card,
	}, nil
}

// SendMessage sends a message to the A2A agent and returns the response.
// Handles conversion between AgentMesh and A2A message formats.
func (c *Client) SendMessage(ctx context.Context, msg message.Message) ([]message.Message, error) {
	// Convert to A2A format
	a2aMsg, err := ConvertToA2AMessage(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message: %w", err)
	}

	// Add skill ID to metadata
	if a2aMsg.Metadata == nil {
		a2aMsg.Metadata = make(map[string]any)
	}
	a2aMsg.Metadata["skillID"] = c.skillID

	// Send message to A2A agent
	resp, err := c.client.SendMessage(ctx, &a2atypes.MessageSendParams{
		Message: a2aMsg,
	})
	if err != nil {
		return nil, fmt.Errorf("a2a agent call failed: %w", err)
	}

	// Extract and convert response (can be either Task or Message)
	switch r := resp.(type) {
	case *a2atypes.Task:
		// Extract message from Task
		a2aMsg := extractMessageFromTask(r)
		if a2aMsg == nil {
			return nil, fmt.Errorf("task has no message content (no artifacts, status message, or history)")
		}
		return ConvertFromA2AMessage(a2aMsg)
	case *a2atypes.Message:
		// Direct message response
		return ConvertFromA2AMessage(r)
	default:
		return nil, fmt.Errorf("unexpected response type from a2a agent: %T", resp)
	}
}

// StreamMessages sends a message and streams responses from the A2A agent.
// Returns an iterator that yields messages as they arrive.
func (c *Client) StreamMessages(ctx context.Context, msg message.Message) func(func(message.Message, error) bool) {
	return func(yield func(message.Message, error) bool) {
		// Convert to A2A format
		a2aMsg, err := ConvertToA2AMessage(msg)
		if err != nil {
			yield(nil, fmt.Errorf("failed to convert message: %w", err))
			return
		}

		// Add skill ID to metadata
		if a2aMsg.Metadata == nil {
			a2aMsg.Metadata = make(map[string]any)
		}
		a2aMsg.Metadata["skillID"] = c.skillID

		// Stream messages from A2A agent
		stream := c.client.SendStreamingMessage(ctx, &a2atypes.MessageSendParams{
			Message: a2aMsg,
		})

		// Process streamed events
		for event, err := range stream {
			if err != nil {
				yield(nil, fmt.Errorf("streaming error: %w", err))
				return
			}

			// Extract message from event
			a2aMsg := extractMessageFromEvent(event)
			if a2aMsg == nil {
				// No message to convert, skip this event
				continue
			}

			msgs, err := ConvertFromA2AMessage(a2aMsg)
			if err != nil {
				yield(nil, fmt.Errorf("failed to convert streamed message: %w", err))
				return
			}

			for _, m := range msgs {
				if !yield(m, nil) {
					return
				}
			}
		}
	}
}

// Card returns the resolved agent card.
func (c *Client) Card() *a2atypes.AgentCard {
	return c.card
}

// SkillID returns the skill ID this client is configured for.
func (c *Client) SkillID() string {
	return c.skillID
}

// ConvertToA2AMessage converts an AgentMesh message to an A2A message.
// Handles SystemMessage, HumanMessage, AIMessage, ToolMessage, and FunctionMessage types.
func ConvertToA2AMessage(msg message.Message) (*a2atypes.Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}

	switch m := msg.(type) {
	case *message.SystemMessage:
		return convertSystemMessage(m), nil
	case *message.HumanMessage:
		return convertHumanMessage(m), nil
	case *message.AIMessage:
		return convertAIMessage(m), nil
	case *message.ToolMessage:
		return convertToolMessage(m), nil
	case *message.FunctionMessage:
		return convertFunctionMessage(m), nil
	default:
		return nil, fmt.Errorf("unsupported message type: %T", msg)
	}
}

func convertSystemMessage(m *message.SystemMessage) *a2atypes.Message {
	var parts []a2atypes.Part
	for _, part := range m.Parts() {
		if textPart, ok := part.(message.TextPart); ok {
			parts = append(parts, a2atypes.TextPart{Text: textPart.Text})
		}
	}
	// A2A doesn't have a system role, so we convert to user role
	return a2atypes.NewMessage(a2atypes.MessageRoleUser, parts...)
}

func convertHumanMessage(m *message.HumanMessage) *a2atypes.Message {
	var parts []a2atypes.Part
	for _, part := range m.Parts() {
		if textPart, ok := part.(message.TextPart); ok {
			parts = append(parts, a2atypes.TextPart{Text: textPart.Text})
		}
	}
	return a2atypes.NewMessage(a2atypes.MessageRoleUser, parts...)
}

func convertAIMessage(m *message.AIMessage) *a2atypes.Message {
	messageParts := m.Parts()
	parts := make([]a2atypes.Part, 0, len(messageParts)+len(m.ToolCalls))

	// Convert message parts
	for _, part := range messageParts {
		switch p := part.(type) {
		case message.TextPart:
			parts = append(parts, a2atypes.TextPart{Text: p.Text})
		case message.FunctionCallPart:
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

	return a2atypes.NewMessage(a2atypes.MessageRoleAgent, parts...)
}

func convertToolMessage(m *message.ToolMessage) *a2atypes.Message {
	textContent := extractTextFromParts(m.Parts())
	parts := []a2atypes.Part{
		a2atypes.DataPart{
			Data: map[string]any{
				"toolCallID": m.ToolCallID,
				"result":     textContent,
			},
		},
	}
	return a2atypes.NewMessage(a2atypes.MessageRoleAgent, parts...)
}

func convertFunctionMessage(m *message.FunctionMessage) *a2atypes.Message {
	textContent := extractTextFromParts(m.Parts())
	parts := []a2atypes.Part{
		a2atypes.DataPart{
			Data: map[string]any{
				"function": m.Name,
				"result":   textContent,
			},
		},
	}
	return a2atypes.NewMessage(a2atypes.MessageRoleAgent, parts...)
}

func extractTextFromParts(parts []message.Part) string {
	var text string
	for _, part := range parts {
		if textPart, ok := part.(message.TextPart); ok {
			text += textPart.Text
		}
	}
	return text
}

// ConvertFromA2AMessage converts an A2A message to AgentMesh messages.
// May return multiple messages if the A2A message contains complex parts.
// Note: A2A only supports 'user' and 'agent' roles.
func ConvertFromA2AMessage(a2aMsg *a2atypes.Message) ([]message.Message, error) {
	if a2aMsg == nil {
		return nil, fmt.Errorf("a2a message cannot be nil")
	}

	switch a2aMsg.Role {
	case a2atypes.MessageRoleUser:
		return convertA2AToHumanMessage(a2aMsg)
	case a2atypes.MessageRoleAgent:
		return convertA2AToAgentMessage(a2aMsg)
	default:
		return nil, fmt.Errorf("unsupported A2A message role: %s", a2aMsg.Role)
	}
}

func convertA2AToHumanMessage(a2aMsg *a2atypes.Message) ([]message.Message, error) {
	var textParts []message.Part
	for _, part := range a2aMsg.Parts {
		if textPart, ok := part.(a2atypes.TextPart); ok {
			textParts = append(textParts, message.TextPart{Text: textPart.Text})
		}
	}
	if len(textParts) > 0 {
		return []message.Message{message.NewHumanMessage(textParts)}, nil
	}
	return []message.Message{}, nil
}

func convertA2AToAgentMessage(a2aMsg *a2atypes.Message) ([]message.Message, error) {
	var parts []message.Part
	for _, part := range a2aMsg.Parts {
		convertedPart, err := convertA2APart(part)
		if err != nil {
			return nil, err
		}
		if convertedPart != nil {
			parts = append(parts, convertedPart)
		}
	}
	if len(parts) > 0 {
		return []message.Message{message.NewAIMessage(parts)}, nil
	}
	return []message.Message{}, nil
}

func convertA2APart(part a2atypes.Part) (message.Part, error) {
	switch p := part.(type) {
	case a2atypes.TextPart:
		return message.TextPart{Text: p.Text}, nil
	case a2atypes.DataPart:
		dataJSON, err := json.Marshal(p.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data part: %w", err)
		}
		return message.TextPart{Text: fmt.Sprintf("[Data: %s]", string(dataJSON))}, nil
	case a2atypes.FilePart:
		return message.TextPart{Text: "[File]"}, nil
	default:
		return nil, nil
	}
}

// ConvertMessagesToA2A converts a slice of AgentMesh messages to A2A messages.
func ConvertMessagesToA2A(messages []message.Message) ([]*a2atypes.Message, error) {
	if messages == nil {
		return nil, nil
	}

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
	if a2aMessages == nil {
		return nil, nil
	}

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
// Returns an empty string if the message contains no text parts.
func ExtractTextContent(a2aMsg *a2atypes.Message) string {
	if a2aMsg == nil {
		return ""
	}

	var text string
	for _, part := range a2aMsg.Parts {
		if textPart, ok := part.(a2atypes.TextPart); ok {
			text += textPart.Text
		}
	}
	return text
}

// ExtractTextFromMessages extracts all text content from AgentMesh messages.
// Returns an empty string if no text content is found.
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

// extractMessageFromTask extracts a message from an A2A Task following priority:
// 1. Artifacts (streaming/generated content) - use last artifact
// 2. Status.Message (status updates with embedded message)
// 3. History (conversation history) - use last message
// Returns nil if no message content is available.
func extractMessageFromTask(task *a2atypes.Task) *a2atypes.Message {
	switch {
	case len(task.Artifacts) > 0:
		// Use last artifact's parts (streaming/generated content)
		lastArtifact := task.Artifacts[len(task.Artifacts)-1]
		return &a2atypes.Message{
			ID:        "", // Artifact doesn't have message ID
			Role:      a2atypes.MessageRoleAgent,
			Parts:     lastArtifact.Parts,
			TaskID:    task.ID,
			ContextID: task.ContextID,
		}
	case task.Status.Message != nil:
		// Use status message (status updates with embedded message)
		return task.Status.Message
	case len(task.History) > 0:
		// Fallback to last history message
		return task.History[len(task.History)-1]
	default:
		return nil
	}
}

// extractMessageFromEvent extracts a message from an A2A event.
// Handles Task, Message, TaskStatusUpdateEvent, and TaskArtifactUpdateEvent.
// Returns nil if the event contains no message content.
func extractMessageFromEvent(event a2atypes.Event) *a2atypes.Message {
	switch e := event.(type) {
	case *a2atypes.Task:
		// Extract from Task (same priority as SendMessage)
		return extractMessageFromTask(e)
	case *a2atypes.Message:
		// Direct message event
		return e
	case *a2atypes.TaskStatusUpdateEvent:
		// Status update with optional embedded message
		return e.Status.Message // May be nil
	case *a2atypes.TaskArtifactUpdateEvent:
		// Artifact update event - convert artifact to message
		if e.Artifact != nil {
			return &a2atypes.Message{
				Role:      a2atypes.MessageRoleAgent,
				Parts:     e.Artifact.Parts,
				TaskID:    e.TaskID,
				ContextID: e.ContextID,
			}
		}
		return nil
	default:
		// Unknown event type
		return nil
	}
}
