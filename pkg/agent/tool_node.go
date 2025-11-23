package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// ToolNode is a graph node that executes a tool.Executor to process tool calls.
//
// The ToolNode is a thin orchestration layer that:
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
//	node, err := agent.NewToolNode(executor)
type ToolNode struct {
	name     string
	executor tool.Executor
	targets  []string
}

// ToolNodeOption configures a ToolNode.
type ToolNodeOption func(*ToolNode)

// WithToolNodeName sets the name of the tool node (default: "tool").
func WithToolNodeName(name string) ToolNodeOption {
	return func(n *ToolNode) {
		n.name = name
	}
}

// WithToolTargets sets the possible routing targets for this node.
// Default is []string{"model"}.
func WithToolTargets(targets []string) ToolNodeOption {
	return func(n *ToolNode) {
		n.targets = targets
	}
}

// NewToolNode creates a new tool node that executes the provided executor.
//
// The executor encapsulates all tool execution logic including sequential vs parallel
// execution, error handling, plugins, and observability. This allows for flexible
// executor implementations that can be swapped without modifying the node.
//
// Example:
//
//	executor := tool.NewParallelExecutor(toolRegistry,
//	    tool.WithContinueOnError(true),
//	    tool.WithMaxConcurrency(5))
//	node, err := agent.NewToolNode(executor,
//	    agent.WithToolNodeName("tools"),
//	    agent.WithToolTargets([]string{"model"}))
func NewToolNode(executor tool.Executor, opts ...ToolNodeOption) (*ToolNode, error) {
	if executor == nil {
		return nil, fmt.Errorf("agent: executor cannot be nil")
	}

	node := &ToolNode{
		name:     "tool",
		executor: executor,
		targets:  []string{"model"}, // Default target
	}

	for _, opt := range opts {
		opt(node)
	}

	return node, nil
}

// Name returns the name of the tool node.
func (n *ToolNode) Name() string {
	return n.name
}

// Targets returns the possible routing destinations for this node.
func (n *ToolNode) Targets() []string {
	if len(n.targets) > 0 {
		return n.targets
	}
	// Default target for backward compatibility
	return []string{"model"}
}

// Execute processes tool calls from the last AI message by delegating to the executor.
func (n *ToolNode) Execute(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
	// Get last message from state
	lastMsg := LastMessage(view)
	if lastMsg == nil {
		// No message, route back to model
		return graph.GotoOne("model"), nil
	}

	ai, ok := lastMsg.(*message.AIMessage)
	if !ok || ai == nil {
		// Not an AI message, route back to model
		return graph.GotoOne("model"), nil
	}

	if len(ai.ToolCalls) == 0 {
		// No tool calls, route back to model
		return graph.GotoOne("model"), nil
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

	// Inject plugin manager from context into executor context
	if pm := callbacks.FromContext(ctx); pm != nil {
		ctx = tool.WithPlugin(ctx, pm)
	}

	// Execute via the executor - it handles plugins, observability, parallel/sequential, etc.
	results, err := n.executor.Execute(ctx, calls)
	if err != nil {
		return nil, err
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

	builder := state.NewUpdateBuilder()
	state.AppendUpdate(builder, MessagesKey, toolMessages...)
	updates, _ := builder.Build()

	// Tool node always routes back to model after execution
	return graph.Goto("model", updates), nil
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
