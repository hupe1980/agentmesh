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
	// SetParent sets the parent agent
	SetParent(p Agent) error

	// Parent returns the current parent agent or nil if this agent is root.
	Parent() Agent

	// RootAgent returns the root ancestor of this agent in the hierarchy.
	RootAgent() Agent

	// AddSubAgents appends child agents to this agent, establishing parent-child relationships.
	AddSubAgents(children ...Agent) error

	// SubAgents returns a shallow copy of current child agents for safe iteration.
	SubAgents() []Agent

	// HasSubAgents returns whether the agent has child agents.
	HasSubAgents() bool

	// FindAgent performs a depth-first search over the subtree rooted at this
	// agent (including itself) returning the first agent whose Name matches.
	FindAgent(name string) (Agent, error)
}

// InstructionResolver defines the interface for resolving instructions.
type InstructionResolver interface {
	// ResolveInstructions resolves the instructions for the agent.
	ResolveInstructions(ctx context.Context, roCtx ReadonlyContext) (string, error)
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

// HistoryMode determines what kind of history an agent receives.
type HistoryMode int

const (
	// HistoryNone means no history, current turn only.
	HistoryNone HistoryMode = iota
	// HistoryOwn includes only the agent’s own history plus user messages
	HistoryOwn
	// HistoryAll includes all history (multi-agent)
	HistoryAll
)

// FlowAgent represents the orchestration-facing view of an agent used by flows and processors.
// It bundles identity, hierarchy, instruction resolution, model/tools, feature flags and output key.
// This is intentionally separate from Agent (which defines Run) so flows can orchestrate without
// requiring the concrete execution entrypoint.
type FlowAgent interface {
	AgentIdentity
	HierarchicalAgent
	InstructionResolver

	// Model returns the underlying model used by the agent.
	Model() Model
	// ModelCapabilities returns the capabilities of the underlying model.
	ModelCapabilities() *ModelCapabilities
	// ResolveTools aggregates tools from tools and toolsets.
	ResolveTools(ctx context.Context, roCtx ReadonlyContext) ([]Tool, error)

	// MaxHistoryMessages limits how many past messages to include.
	MaxHistoryMessages() int
	// HistoryMode controls what kind of history the agent receives.
	HistoryMode() HistoryMode
	// IsStreamingEnabled indicates whether the agent streams model output.
	IsStreamingEnabled() bool
	// IsTransferToPeersEnabled indicates whether the agent may transfer to peer agents.
	IsTransferToPeersEnabled() bool
	// IsTransferToParentEnabled indicates whether the agent may transfer to its parent.
	IsTransferToParentEnabled() bool

	// OutputSchema returns the expected output schema for responses.
	OutputSchema() Opt[OutputSchema]
	// OutputKey specifies where the final output should be stored in session state.
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
