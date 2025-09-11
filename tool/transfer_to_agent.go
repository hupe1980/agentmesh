package tool

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// NewTransferToAgentTool constructs the transfer tool instance using FuncTool.
func NewTransferToAgentTool() core.Tool {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"description": "Target agent name",
			},
		},
		"required": []string{"agent"},
	}

	return NewFuncTool(
		"transfer_to_agent",
		"Request transfer of control to another sub-agent by name. Use when another agent is better suited.",
		params,
		func(_ context.Context, tc core.ToolContext, args map[string]any) (any, error) {
			raw, ok := args["agent"]
			if !ok {
				return nil, NewError("transfer_to_agent", "missing required field 'agent'", "VALIDATION_ERROR")
			}

			agentName, ok := raw.(string)
			if !ok || agentName == "" {
				return nil, NewError("transfer_to_agent", "field 'agent' must be non-empty string", "VALIDATION_ERROR")
			}

			tc.TransferToAgent(agentName)

			return map[string]any{
				"transferred": true,
				"agent":       agentName,
			}, nil
		},
	)
}
