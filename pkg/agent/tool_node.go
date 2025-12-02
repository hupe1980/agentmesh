package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// ToolNodeConfig holds configuration for creating a tool node function.
type ToolNodeConfig struct {
	Executor    tool.Executor
	ModelTarget string // Target node to route back to (default: "model")
}

// ToolNodeOption configures a ToolNodeConfig.
type ToolNodeOption func(*ToolNodeConfig)

// WithModelTarget sets the target node to route back to after tool execution.
// Default is "model".
func WithModelTarget(target string) ToolNodeOption {
	return func(c *ToolNodeConfig) {
		c.ModelTarget = target
	}
}

// NewToolNodeFunc creates a graph.NodeFunc that executes tools.
//
// The function:
//   - Extracts tool calls from the last AI message
//   - Converts them to executor format
//   - Delegates execution to the provided Executor
//   - Formats results as ToolMessages
//   - Routes back to model
//
// The Executor handles all execution concerns including:
//   - Sequential vs parallel execution
//   - Error handling (continueOnError, errorPrefix)
//   - Plugin lifecycle (BeforeTool, AfterTool, OnToolError)
//   - Observability (tracing, metrics, logging)
//   - Concurrency control (maxConcurrency for parallel execution)
//
// Example:
//
//	executor := tool.NewSequentialExecutor(toolRegistry)
//	toolFn, err := agent.NewToolNodeFunc(executor)
//
//	g.Node("tool", toolFn, "model")
func NewToolNodeFunc(executor tool.Executor, opts ...ToolNodeOption) (graph.NodeFunc, error) {
	if err := validate.NotNil(executor, "agent: executor"); err != nil {
		return nil, err
	}

	cfg := &ToolNodeConfig{
		Executor:    executor,
		ModelTarget: "model",
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Get last message from state
		lastMsg := LastMessage(view)
		if lastMsg == nil {
			// No message, route back to model (no updates needed)
			return graph.To(cfg.ModelTarget)
		}

		ai, ok := lastMsg.(*message.AIMessage)
		if !ok || ai == nil {
			// Not an AI message, route back to model
			return graph.To(cfg.ModelTarget)
		}

		if len(ai.ToolCalls) == 0 {
			// No tool calls, route back to model
			return graph.To(cfg.ModelTarget)
		}

		// Convert message.ToolCall to tool.Call format
		calls := make([]tool.Call, len(ai.ToolCalls))
		for i, tc := range ai.ToolCalls {
			calls[i] = tool.Call{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			}
		}

		// Execute via the executor
		results, err := cfg.Executor.Execute(ctx, calls)
		if err != nil {
			return graph.Fail(err)
		}

		// Convert results to ToolMessages
		toolMessages := make([]message.Message, 0, len(results))
		for _, result := range results {
			if result.Error != nil {
				// Error already handled by executor (logged, metricsed, etc.)
				// Just format it as a tool message
				toolMessages = append(toolMessages,
					message.NewToolMessage(result.ToolCallID, fmt.Sprintf("Error: %v", result.Error)))
			} else {
				// Format successful result
				text := formatToolResult(result.Result)
				toolMessages = append(toolMessages,
					message.NewToolMessage(result.ToolCallID, text))
			}
		}

		// Return only the NEW tool messages generated
		// The state manager will append them to the existing messages list
		return graph.Append(MessagesKey, toolMessages...).To(cfg.ModelTarget)
	}, nil
}

// formatToolResult converts a tool result to a string representation.
func formatToolResult(result any) string {
	if result == nil {
		return "null"
	}

	switch v := result.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", result)
	}
}
