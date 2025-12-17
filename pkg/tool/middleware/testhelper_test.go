package middleware

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/tool"
)

// mockToolExecutor creates a simple tool executor that returns the given result for each call.
func mockToolExecutor(result string) tool.Executor {
	return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
		results := make([]tool.ExecutionResult, len(calls))
		for i, call := range calls {
			results[i] = tool.ExecutionResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Result:     result,
			}
		}
		return results, nil
	})
}
