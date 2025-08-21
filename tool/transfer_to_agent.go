package tool

import (
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// transferToAgentTool requests orchestration transfer to a named sub‑agent.
type transferToAgentTool struct{}

// NewTransferToAgentTool constructs the transfer tool instance.
func NewTransferToAgentTool() Tool { return &transferToAgentTool{} }

func (t *transferToAgentTool) Name() string { return "transfer_to_agent" }

func (t *transferToAgentTool) Description() string {
	return "Request transfer of control to another sub-agent by name. Use when another agent is better suited."
}

func (t *transferToAgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{"type": "string", "description": "Target agent name"},
		},
		"required": []string{"agent"},
	}
}

func (t *transferToAgentTool) Call(tc *core.ToolContext, args map[string]any) (any, error) {
	raw, ok := args["agent"]
	if !ok {
		return nil, fmt.Errorf("missing required field 'agent'")
	}
	agentName, ok := raw.(string)
	if !ok || agentName == "" {
		return nil, fmt.Errorf("field 'agent' must be non-empty string")
	}
	tc.TransferToAgent(agentName)
	return map[string]any{"transferred": true, "agent": agentName}, nil
}
