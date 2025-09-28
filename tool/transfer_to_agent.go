package tool

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/jsonschema"
)

// TransferToAgentInput defines the input structure for the transfer_to_agent tool.
type TransferToAgentInput struct {
	// Agent is the name of the target agent to transfer control to.
	Agent string `json:"agent" jsonschema:"description=Target agent name"`
}

// NewTransferToAgentTool constructs the transfer tool instance using FuncTool.
func NewTransferToAgentTool() (core.Tool, error) {
	params, err := jsonschema.MapFromStruct(TransferToAgentInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON schema: %w", err)
	}

	return NewFuncTool(
		"transfer_to_agent",
		"Request transfer of control to another sub-agent by name. Use when another agent is better suited.",
		params,
		func(ctx context.Context, tc core.ToolContext, args TransferToAgentInput) (any, error) {
			agentName := args.Agent
			if agentName == "" {
				return nil, NewError("transfer_to_agent", "field 'agent' must be non-empty string", "VALIDATION_ERROR")
			}

			tc.TransferToAgent(agentName)

			return map[string]any{
				"transferred": true,
				"agent":       agentName,
			}, nil
		},
	), nil
}
