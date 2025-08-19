// Package flow provides execution flow management for AgentMesh agents.
//
// Flows orchestrate the execution pipeline of agents, allowing for modular
// and configurable processing of requests and responses. This design enables
// clean separation of concerns and easy extensibility.
package flow

import (
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/tool"
)

// Flow defines the interface for agent execution flows.
//
// A flow orchestrates the complete execution pipeline of an agent,
// from processing the initial request to generating the final response.
// Different flow implementations can provide different capabilities
// such as simple execution, agent transfers, or complex multi-step workflows.
type Flow interface {
	// Execute runs the flow with the given context and request.
	// It returns a channel of events that represent the execution progress.
	Execute(runCtx *core.RunContext) (<-chan core.Event, error)
}

// FlowAgent defines the interface that agents must implement to work with flows.
//
// This interface provides flows with access to agent capabilities without
// exposing the full agent implementation details.
type FlowAgent interface {
	// GetName returns the agent's display name.
	GetName() string

	// GetLLM returns the language model instance.
	GetLLM() model.Model

	ResolveInstructions(runCtx *core.RunContext) (string, error)

	// GetTools returns the registered tools for function calling.
	GetTools() map[string]tool.Tool

	// GetSubAgents returns the list of child agents.
	GetSubAgents() []FlowAgent

	// IsFunctionCallingEnabled returns whether function calling is enabled.
	IsFunctionCallingEnabled() bool

	// IsStreamingEnabled returns whether streaming responses are enabled.
	IsStreamingEnabled() bool

	// IsTransferEnabled returns whether agent transfer is enabled.
	IsTransferEnabled() bool

	// GetOutputKey returns the session state key for saving responses.
	GetOutputKey() string

	// MaxHistoryMessages returns the maximum number of conversation history messages to keep.
	MaxHistoryMessages() int

	// ExecuteTool executes a named tool with the given arguments.
	ExecuteTool(toolCtx *core.ToolContext, toolName string, args string) (interface{}, error)

	// TransferToAgent transfers execution to a named sub-agent.
	TransferToAgent(runCtx *core.RunContext, agentName string) error
}

// RequestProcessor processes the request before sending it to the LLM.
type RequestProcessor interface {
	// Name returns the processor's identifier.
	Name() string
	// ProcessRequest modifies the chat request before LLM execution.
	ProcessRequest(runCtx *core.RunContext, req *model.Request, agent FlowAgent) error
}

// ResponseProcessor processes the response after receiving it from the LLM.
type ResponseProcessor interface {
	// Name returns the processor's identifier.
	Name() string
	// ProcessResponse handles the LLM response and may generate additional events.
	ProcessResponse(runCtx *core.RunContext, resp *model.Response, agent FlowAgent) error
}
