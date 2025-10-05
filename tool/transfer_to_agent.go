package tool

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// TransferToAgentInput defines the input structure for the transfer_to_agent tool.
type TransferToAgentInput struct {
	// Agent is the name of the target agent to transfer control to.
	Agent string `json:"agent" jsonschema:"description=Target agent name"`
}

// NewTransferToAgentTool constructs the transfer tool instance using FuncTool.
func NewTransferToAgentTool() (core.Tool, error) {
	return NewFuncToolFromType(
		"transfer_to_agent",
		"Request transfer of control to another sub-agent by name. Use when another agent is better suited.",
		&TransferToAgentInput{},
		func(ctx context.Context, tc core.ToolContext, args *TransferToAgentInput) (any, error) {
			if args == nil || args.Agent == "" {
				return nil, NewError("transfer_to_agent", "field 'agent' must be non-empty string", "VALIDATION_ERROR")
			}

			tc.TransferToAgent(args.Agent)

			return map[string]any{
				"transferred": true,
				"agent":       args.Agent,
			}, nil
		},
	)
}
