package a2a

import (
	"context"
	"encoding/json"
	"fmt"

	a2atypes "github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// Tool wraps an external A2A agent as an AgentMesh tool.
// It allows calling remote A2A-compliant agents from within AgentMesh workflows.
type Tool struct {
	client      *a2aclient.Client
	skillID     string
	name        string
	description string
	card        *a2atypes.AgentCard
}

// Option configures an A2A tool.
type Option func(*Tool)

// WithName sets a custom name for the tool.
// If not provided, the skill name from the agent card will be used.
func WithName(name string) Option {
	return func(t *Tool) {
		t.name = name
	}
}

// WithDescription sets a custom description for the tool.
// If not provided, the skill description from the agent card will be used.
func WithDescription(description string) Option {
	return func(t *Tool) {
		t.description = description
	}
}

// NewTool creates a new A2A tool that calls an external agent.
//
// The agentCardURL should point to the agent's card endpoint (typically at /.well-known/agent-card).
// The skillID identifies which skill/capability of the agent to invoke.
// Additional a2aclient.FactoryOption can be provided to configure the underlying A2A client.
//
// Example:
//
//	tool, err := a2a.NewTool(
//	    ctx,
//	    "https://agent.example.com",
//	    "translation",
//	    a2a.WithName("translator"),
//	)
func NewTool(ctx context.Context, agentCardURL string, skillID string, opts ...any) (*Tool, error) {
	// Separate A2A tool options from a2aclient options
	var toolOpts []Option
	var clientOpts []a2aclient.FactoryOption

	for _, opt := range opts {
		switch o := opt.(type) {
		case Option:
			toolOpts = append(toolOpts, o)
		case a2aclient.FactoryOption:
			clientOpts = append(clientOpts, o)
		}
	}

	// Resolve the agent card
	card, err := agentcard.DefaultResolver.Resolve(ctx, agentCardURL)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent card from %s: %w", agentCardURL, err)
	}

	// Create A2A client
	client, err := a2aclient.NewFromCard(ctx, card, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create A2A client: %w", err)
	}

	// Find the skill in the card to get default name and description
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
	}
	if skillDescription == "" {
		skillDescription = fmt.Sprintf("Call A2A agent skill: %s", skillID)
	}

	t := &Tool{
		client:      client,
		skillID:     skillID,
		name:        skillName,
		description: skillDescription,
		card:        card,
	}

	// Apply custom options
	for _, opt := range toolOpts {
		opt(t)
	}

	return t, nil
}

// Name returns the tool name.
func (t *Tool) Name() string {
	return t.name
}

// Description returns the tool description.
func (t *Tool) Description() string {
	return t.description
}

// Definition returns the tool definition for the LLM.
func (t *Tool) Definition() *tool.ToolDefinition {
	// Build parameters schema from the agent card skill if available
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The input message to send to the A2A agent",
			},
		},
		"required": []string{"input"},
	}

	// Try to get more specific parameters from the skill
	for i := range t.card.Skills {
		if t.card.Skills[i].ID == t.skillID {
			if len(t.card.Skills[i].Examples) > 0 {
				params["properties"].(map[string]any)["input"].(map[string]any)["examples"] = t.card.Skills[i].Examples
			}
			break
		}
	}

	return &tool.ToolDefinition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.name,
			Description: t.description,
			Parameters:  params,
		},
	}
}

// Call executes the A2A agent with the given input.
func (t *Tool) Call(ctx context.Context, argsJSON string) (any, error) {
	// Parse the input arguments
	var args struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Create A2A message with skill ID in metadata
	msg := a2atypes.NewMessage("user", a2atypes.TextPart{Text: args.Input})
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]any)
	}
	msg.Metadata["skillID"] = t.skillID

	// Send message to the A2A agent
	resp, err := t.client.SendMessage(ctx, &a2atypes.MessageSendParams{
		Message: msg,
	})
	if err != nil {
		return nil, fmt.Errorf("A2A agent call failed: %w", err)
	}

	// Extract response message and convert to string
	if msgGetter, ok := resp.(interface{ GetMessage() *a2atypes.Message }); ok {
		responseMsg := msgGetter.GetMessage()
		return extractTextContent(responseMsg), nil
	}

	return nil, fmt.Errorf("unexpected response type from A2A agent")
}

// extractTextContent extracts all text content from an A2A message.
func extractTextContent(msg *a2atypes.Message) string {
	var text string
	for _, part := range msg.Parts {
		if textPart, ok := part.(a2atypes.TextPart); ok {
			if text != "" {
				text += "\n"
			}
			text += textPart.Text
		}
	}
	return text
}

// AgentCard returns the resolved agent card.
// This can be useful for inspecting the agent's capabilities.
func (t *Tool) AgentCard() *a2atypes.AgentCard {
	return t.card
}

// Ensure Tool implements tool.Tool interface
var _ tool.Tool = (*Tool)(nil)
