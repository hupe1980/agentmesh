package core

import (
	"context"
)

// Tool defines the interface for extending agent capabilities with external functions.
//
// Tools can be registered with agents to enable function calling, allowing
// agents to perform actions beyond text generation such as API calls, calculations,
// database queries, or any other programmatic operations.
//
// All tools have access to ToolContext for session state, agent flow control,
// authentication, memory, and artifact management. This enables tools to build
// sophisticated workflows and integrate deeply with the AgentMesh framework infrastructure.
//
// Tool implementations should:
//   - Provide clear, descriptive names and descriptions
//   - Define proper JSON schema for parameters
//   - Handle errors gracefully
//   - Be thread-safe if used concurrently
//   - Follow consistent naming conventions
type Tool interface {
	// Name returns the unique identifier for this tool.
	// Names should be descriptive and follow function naming conventions (snake_case recommended).
	Name() string

	// Description returns a human-readable description of what this tool does.
	// This description is provided to the LLM to help it understand when and how to use the tool.
	Description() string

	// Parameters returns a JSON schema describing the expected input format.
	// This schema is used for parameter validation and LLM function calling.
	Parameters() map[string]any

	// ProcessModelRequest allows the tool to modify the outgoing ModelRequest
	ProcessModelRequest(ctx context.Context, toolCtx ToolContext, req *ModelRequest) error

	// Call executes the tool with structured arguments and ToolContext.
	// This method provides tools with access to session state, agent actions,
	// authentication, memory, and artifact management capabilities.
	// Arguments are parsed from JSON and validated against the tool's schema.
	Call(ctx context.Context, toolCtx ToolContext, args map[string]any) (any, error)
}

// ToolExecutor abstracts executing a Tool with a prepared argument map and ToolContext.
// It mirrors AgentExecutor for symmetry and future middleware (logging, tracing, retry).
type ToolExecutor interface {
	Execute(ctx context.Context, toolCtx ToolContext, tool Tool, args map[string]any) (any, error)
}

// ToolExecutorFunc is a function adapter implementing ToolExecutor.
type ToolExecutorFunc func(context.Context, ToolContext, Tool, map[string]any) (any, error)

// Execute calls the underlying function.
func (f ToolExecutorFunc) Execute(
	ctx context.Context,
	toolCtx ToolContext,
	tool Tool,
	args map[string]any,
) (any, error) {
	return f(ctx, toolCtx, tool, args)
}
