package tool

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// NewExitLoopTool constructs the exit loop tool instance using FuncTool.
func NewExitLoopTool() core.Tool {
	params := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}

	return NewFuncTool(
		"exit_loop",
		"Exits the loop. Call this function only when you are instructed to do so.",
		params,
		func(ctx context.Context, tc core.ToolContext, args map[string]any) (any, error) {
			tc.EventActions().Escalate = core.Bool(true)

			return map[string]any{}, nil
		},
	)
}
