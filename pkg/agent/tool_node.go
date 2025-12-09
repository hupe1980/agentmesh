package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// ToolNodeConfig holds configuration for creating a tool node function.
type ToolNodeConfig struct {
	Executor    tool.Executor     // Static executor (mutually exclusive with Toolset)
	Toolset     tool.Toolset      // Dynamic toolset for runtime tool discovery
	Middleware  []tool.Middleware // Middleware to apply to the executor
	ModelTarget string            // Target node to route back to (default: "model")
}

// ToolNodeOption configures a ToolNodeConfig.
type ToolNodeOption func(*ToolNodeConfig)

// WithToolExecutor sets a static tool executor.
// For dynamic tool discovery, use WithToolNodeToolset instead.
func WithToolExecutor(executor tool.Executor) ToolNodeOption {
	return func(c *ToolNodeConfig) {
		c.Executor = executor
	}
}

// WithToolNodeToolset sets a dynamic toolset for runtime tool discovery.
// Tools are discovered on each invocation with access to the current graph state.
func WithToolNodeToolset(ts tool.Toolset) ToolNodeOption {
	return func(c *ToolNodeConfig) {
		c.Toolset = ts
	}
}

// WithToolNodeMiddleware sets middleware to apply to the tool executor.
func WithToolNodeMiddleware(middleware ...tool.Middleware) ToolNodeOption {
	return func(c *ToolNodeConfig) {
		c.Middleware = append(c.Middleware, middleware...)
	}
}

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
//   - Discovers tools from the configured Toolset (or uses static Executor)
//   - Converts tool calls to executor format
//   - Delegates execution to the Executor
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
// Example with static executor:
//
//	executor := tool.NewSequentialExecutor(toolRegistry)
//	toolFn, err := agent.NewToolNodeFunc(agent.WithToolExecutor(executor))
//
// Example with dynamic toolset:
//
//	toolFn, err := agent.NewToolNodeFunc(agent.WithToolNodeToolset(mcpToolset))
func NewToolNodeFunc(opts ...ToolNodeOption) (graph.NodeFunc, error) {
	cfg := &ToolNodeConfig{
		ModelTarget: "model",
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// Validate: must have either Executor or Toolset
	if cfg.Executor == nil && cfg.Toolset == nil {
		return nil, fmt.Errorf("agent: either Executor or Toolset must be provided")
	}

	return func(ctx context.Context, view graph.View) (*graph.Command, error) {
		// Extract tool calls from the last AI message
		toolCalls := extractToolCalls(view)
		if toolCalls == nil {
			return graph.To(cfg.ModelTarget)
		}

		// Resolve executor: use Toolset if provided, otherwise use static Executor
		executor, err := resolveToolExecutor(ctx, view, cfg)
		if err != nil {
			return graph.Fail(err)
		}

		// Convert message.ToolCall to tool.Call format
		calls := convertToolCalls(toolCalls)

		// Execute via the executor
		results, err := executor.Execute(ctx, calls)
		if err != nil {
			return graph.Fail(err)
		}

		// Convert results to ToolMessages
		toolMessages := resultsToMessages(results)

		return graph.Append(MessagesKey, toolMessages...).To(cfg.ModelTarget)
	}, nil
}

// extractToolCalls retrieves tool calls from the last message if it's an AI message.
func extractToolCalls(view graph.View) []message.ToolCall {
	lastMsg := LastMessage(view)
	if lastMsg == nil {
		return nil
	}

	ai, ok := lastMsg.(*message.AIMessage)
	if !ok || ai == nil || len(ai.ToolCalls) == 0 {
		return nil
	}

	return ai.ToolCalls
}

// resolveToolExecutor returns an executor based on configuration.
// If a Toolset is configured, tools are dynamically discovered; otherwise, the static Executor is used.
func resolveToolExecutor(ctx context.Context, view graph.View, cfg *ToolNodeConfig) (tool.Executor, error) {
	var executor tool.Executor

	if cfg.Toolset != nil {
		var err error

		executor, err = createExecutorFromToolset(ctx, view, cfg.Toolset)
		if err != nil {
			return nil, err
		}
	} else {
		executor = cfg.Executor
	}

	// Apply middleware if provided
	if len(cfg.Middleware) > 0 {
		executor = tool.Chain(executor, cfg.Middleware...)
	}

	return executor, nil
}

// createExecutorFromToolset dynamically discovers tools and creates an executor.
func createExecutorFromToolset(ctx context.Context, view graph.View, ts tool.Toolset) (tool.Executor, error) {
	tools, err := ts.ListTools(ctx, view)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	// Build tool registry from discovered tools
	toolRegistry := make(map[string]tool.Tool, len(tools))
	for _, t := range tools {
		if t != nil {
			toolRegistry[t.Name()] = t
		}
	}

	return tool.NewSequentialExecutor(toolRegistry,
		tool.WithErrorPrefix("react agent"),
		tool.WithContinueOnError(false)), nil
}

// convertToolCalls converts message.ToolCall to tool.Call format.
func convertToolCalls(toolCalls []message.ToolCall) []tool.Call {
	calls := make([]tool.Call, len(toolCalls))
	for i, tc := range toolCalls {
		calls[i] = tool.Call{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: tc.Arguments,
		}
	}

	return calls
}

// resultsToMessages converts tool execution results to ToolMessages.
func resultsToMessages(results []tool.ExecutionResult) []message.Message {
	toolMessages := make([]message.Message, 0, len(results))
	for _, result := range results {
		if result.Error != nil {
			toolMessages = append(toolMessages,
				message.NewToolMessage(result.ToolCallID, fmt.Sprintf("Error: %v", result.Error)))
		} else {
			text := formatToolResult(result.Result)
			toolMessages = append(toolMessages,
				message.NewToolMessage(result.ToolCallID, text))
		}
	}

	return toolMessages
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
