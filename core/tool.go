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
	Call(ctx context.Context, toolCtx ToolContext, args string) (any, error)
}

// Toolset defines a collection of tools that can be managed together.
type Toolset interface {
	ListTools(ctx context.Context, roCtx ReadonlyContext) ([]Tool, error)
	Close() error
}

// ToolExecutor defines an interface for executing tools with a given context, request context, and tool registry.
// It also takes a list of function calls to execute.
type ToolExecutor interface {
	Execute(ctx context.Context, reqCtx RequestContext, toolRegistry map[string]Tool, fnCalls []*FunctionCall,
	) ([]*Event, error)
}

// ToolExecutorFunc is a function adapter implementing ToolExecutor.
type ToolExecutorFunc func(context.Context, RequestContext, map[string]Tool, []*FunctionCall) ([]*Event, error)

// Execute calls the underlying function.
func (f ToolExecutorFunc) Execute(
	ctx context.Context,
	reqCtx RequestContext,
	toolRegistry map[string]Tool,
	fnCalls []*FunctionCall,
) ([]*Event, error) {
	return f(ctx, reqCtx, toolRegistry, fnCalls)
}
