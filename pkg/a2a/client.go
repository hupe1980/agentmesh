package a2a

import (
	"context"
	"fmt"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// AgentTool wraps an external A2A agent as an AgentMesh tool.
type AgentTool struct {
	client      *a2aclient.Client
	skillID     string
	name        string
	description string
}

// NewAgentTool creates a new tool that calls an external A2A agent.
// The agentCardURL should point to the agent's card endpoint.
func NewAgentTool(ctx context.Context, agentCardURL string, skillID string, opts ...a2aclient.FactoryOption) (*AgentTool, error) {
	// Resolve the agent card
	card, err := agentcard.DefaultResolver.Resolve(ctx, agentCardURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent card: %w", err)
	}

	// Create client
	client, err := a2aclient.NewFromCard(ctx, card, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}

	// Find the skill in the card to get name and description
	var skillName, skillDescription string
	for i := range card.Skills {
		if card.Skills[i].ID == skillID {
			skillName = card.Skills[i].Name
			skillDescription = card.Skills[i].Description
			break
		}
	}

	if skillName == "" {
		skillName = fmt.Sprintf("a2a_%s", skillID)
		skillDescription = fmt.Sprintf("A2A agent skill: %s", skillID)
	}

	return &AgentTool{
		client:      client,
		skillID:     skillID,
		name:        skillName,
		description: skillDescription,
	}, nil
}

// Name returns the tool name.
func (t *AgentTool) Name() string {
	return t.name
}

// Description returns the tool description.
func (t *AgentTool) Description() string {
	return t.description
}

// Call executes the A2A agent with the given input.
func (t *AgentTool) Call(ctx context.Context, input string) (any, error) {
	// Create A2A message with skill ID in metadata
	msg := a2atypes.NewMessage("user", a2atypes.TextPart{Text: input})
	msg.Metadata = map[string]any{"skillID": t.skillID}

	// Send message to the A2A agent
	resp, err := t.client.SendMessage(ctx, &a2atypes.MessageSendParams{
		Message: msg,
	})

	if err != nil {
		return "", fmt.Errorf("A2A agent call failed: %w", err)
	}

	// Extract response message
	if msgResult, ok := resp.(interface{ GetMessage() *a2atypes.Message }); ok {
		return ExtractTextContent(msgResult.GetMessage()), nil
	}

	return "", fmt.Errorf("unexpected response type from A2A agent")
}

// Definition returns the tool definition for the LLM.
func (t *AgentTool) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.name,
			Description: t.description,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{
						"type":        "string",
						"description": "The input message to send to the A2A agent",
					},
				},
				"required": []string{"input"},
			},
		},
	}
}

// AgentNode creates a graph node function that calls an external A2A agent.
// This allows integrating A2A agents directly into AgentMesh graphs.
func AgentNode(ctx context.Context, agentCardURL string, skillID string, opts ...a2aclient.FactoryOption) func(context.Context, state.ReadView) (state.Updates, error) {
	// Create the client upfront - errors will be returned when node executes
	card, cardErr := agentcard.DefaultResolver.Resolve(ctx, agentCardURL)
	var client *a2aclient.Client
	var clientErr error

	if cardErr == nil {
		client, clientErr = a2aclient.NewFromCard(ctx, card, opts...)
	}

	// Return a node function
	return func(ctx context.Context, view state.ReadView) (state.Updates, error) {
		// Check for setup errors
		if cardErr != nil {
			return nil, fmt.Errorf("failed to resolve agent card: %w", cardErr)
		}
		if clientErr != nil {
			return nil, fmt.Errorf("failed to create A2A client: %w", clientErr)
		}

		// Get last message from state
		lastMsg := agent.LastMessage(view)
		if lastMsg == nil {
			return state.NoUpdate(), nil
		}

		// Convert to A2A format
		a2aMsg, err := ConvertToA2AMessage(lastMsg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}

		// Add skill ID to metadata
		if a2aMsg.Metadata == nil {
			a2aMsg.Metadata = make(map[string]any)
		}
		a2aMsg.Metadata["skillID"] = skillID

		// Send message to A2A agent
		resp, err := client.SendMessage(ctx, &a2atypes.MessageSendParams{
			Message: a2aMsg,
		})

		if err != nil {
			return nil, fmt.Errorf("A2A agent call failed: %w", err)
		}

		// Extract and convert response
		if msgGetter, ok := resp.(interface{ GetMessage() *a2atypes.Message }); ok {
			resultMessages, err := ConvertFromA2AMessage(msgGetter.GetMessage())
			if err != nil {
				return nil, fmt.Errorf("failed to convert response: %w", err)
			}

			// Return converted messages as state updates
			updates := state.Updates{
				agent.MessagesKey.Name(): resultMessages,
			}
			return updates, nil
		}

		return nil, fmt.Errorf("unexpected response type from A2A agent")
	}
}

// StreamingAgentNode creates a graph node that streams responses from an A2A agent.
func StreamingAgentNode(ctx context.Context, agentCardURL string, skillID string, opts ...a2aclient.FactoryOption) func(context.Context, state.ReadView) (state.Updates, error) {
	card, cardErr := agentcard.DefaultResolver.Resolve(ctx, agentCardURL)
	var client *a2aclient.Client
	var clientErr error

	if cardErr == nil {
		client, clientErr = a2aclient.NewFromCard(ctx, card, opts...)
	}

	return func(ctx context.Context, view state.ReadView) (state.Updates, error) {
		if cardErr != nil {
			return nil, fmt.Errorf("failed to resolve agent card: %w", cardErr)
		}
		if clientErr != nil {
			return nil, fmt.Errorf("failed to create A2A client: %w", clientErr)
		}

		// Get last message from state
		lastMsg := agent.LastMessage(view)
		if lastMsg == nil {
			return state.NoUpdate(), nil
		}

		a2aMsg, err := ConvertToA2AMessage(lastMsg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert message: %w", err)
		}

		// Add skill ID to metadata
		if a2aMsg.Metadata == nil {
			a2aMsg.Metadata = make(map[string]any)
		}
		a2aMsg.Metadata["skillID"] = skillID

		// Stream messages from A2A agent
		stream := client.SendStreamingMessage(ctx, &a2atypes.MessageSendParams{
			Message: a2aMsg,
		})

		var resultMessages []message.Message

		// Collect all streamed events
		for event, err := range stream {
			if err != nil {
				return nil, fmt.Errorf("streaming error: %w", err)
			}

			// Try to extract message from event
			if msgGetter, ok := event.(interface{ GetMessage() *a2atypes.Message }); ok {
				msgs, err := ConvertFromA2AMessage(msgGetter.GetMessage())
				if err != nil {
					return nil, fmt.Errorf("failed to convert streamed message: %w", err)
				}
				resultMessages = append(resultMessages, msgs...)
			}
		}

		// Return all collected messages
		updates := state.Updates{
			agent.MessagesKey.Name(): resultMessages,
		}
		return updates, nil
	}
}

// Ensure AgentTool implements the tool.Tool interface
var _ tool.Tool = (*AgentTool)(nil)
