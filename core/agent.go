package core

import (
	"context"
)

// AgentIdentity captures the common metadata exposed by agents.
type AgentIdentity interface {
	// Name returns the human-readable name for this agent.
	Name() string

	// Description returns a detailed description of this agent's purpose.
	Description() string
}

// HierarchicalAgent exposes read-only tree navigation for agents.
type HierarchicalAgent interface {
	// Parent returns the current parent agent or nil if this agent is root.
	Parent() Agent

	// RootAgent returns the root ancestor of this agent in the hierarchy.
	RootAgent() Agent

	// SubAgents returns a shallow copy of current child agents for safe iteration.
	SubAgents() []Agent

	// HasSubAgents returns whether the agent has child agents.
	HasSubAgents() bool

	// FindAgent performs a depth-first search over the subtree rooted at this
	// agent (including itself) returning the first agent whose Name matches.
	FindAgent(name string) (Agent, error)

	// FindSubAgent performs a depth-first search over the subtree rooted at this
	// agent (excluding itself) returning the first agent whose Name matches.
	FindSubAgent(name string) (Agent, error)
}

// Agent is the executable contract implemented by all processing units.
//
// Agents receive input via a RequestContext and stream Events through an
// EventWriter. Agents can be composed into trees (parent/children) to build
// complex workflows. Hierarchy mutation is handled by constructors; the
// interface is read-only for traversal.
type Agent interface {
	AgentIdentity

	HierarchicalAgent

	// Run executes the agent's processing logic.
	Run(ctx context.Context, reqCtx RequestContext, writer EventWriter) error
}

// FlowAgent is the orchestration-facing contract used by flows and processors.
// It exposes capabilities and metadata without requiring the executable Run method.
// Implemented by agent implementations to participate in flow orchestration.
type FlowAgent interface {
	AgentIdentity

	HierarchicalAgent

	// ResolveInstructions returns the agent's instructions for the given context.
	ResolveInstructions(ctx context.Context, roCtx ReadonlyContext) (string, error)

	// Model returns the language model instance.
	Model() Model

	// Tools returns the registered tools for function calling.
	Tools() map[string]Tool

	// MaxHistoryMessages returns the maximum number of conversation history messages to keep.
	MaxHistoryMessages() int

	// IsFunctionCallingEnabled returns whether function calling is enabled.
	IsFunctionCallingEnabled() bool

	// IsStreamingEnabled returns whether streaming responses are enabled.
	IsStreamingEnabled() bool

	// IsTransferToPeersEnabled returns whether transfer to peer/sub-agents is enabled.
	IsTransferToPeersEnabled() bool

	// IsTransferToParentEnabled returns whether transfer to parent is enabled.
	IsTransferToParentEnabled() bool

	// OutputKey returns the session state key for saving responses.
	OutputKey() string
}

// AgentExecutor abstracts agent execution with lifecycle hooks.
type AgentExecutor interface {
	Execute(ctx context.Context, reqCtx RequestContext, ag Agent, w EventWriter) error
}

// AgentExecutorFunc is an adapter to allow plain functions to satisfy AgentExecutor.
type AgentExecutorFunc func(context.Context, RequestContext, Agent, EventWriter) error

// Execute calls the underlying function to execute the agent with the given context, request context,
// agent, and event writer.
func (f AgentExecutorFunc) Execute(ctx context.Context, reqCtx RequestContext, ag Agent, w EventWriter) error {
	return f(ctx, reqCtx, ag, w)
}
