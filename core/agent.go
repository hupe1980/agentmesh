package core

import (
	"context"
	"fmt"
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

// ExecuteAgent runs an agent with BeforeAgent / AfterAgent hook semantics.
//
// Lifecycle:
//  1. BeforeAgent: if it returns a non-nil []Part, the agent's Run is skipped and
//     those parts are emitted as a synthetic assistant event (short-circuit). AfterAgent still runs.
//  2. Agent Run (only if not short-circuited) emits its normal events directly to the provided writer.
//  3. AfterAgent: if it returns a non-nil []Part, a new assistant event is appended
//     (it does not mutate or retract earlier output).
//
// History is strictly append-only; no prior events are modified or removed.
func ExecuteAgent(ctx context.Context, reqCtx RequestContext, ag Agent, w EventWriter) error {
	// If the RequestContext's agent identity doesn't match the target agent's name,
	// clone the context so emitted events have the correct Author. This centralizes
	// transfer / delegation behavior so callers don't need to clone manually.
	if reqCtx.AgentName() != ag.Name() { // lightweight check; cloning is cheap (shallow)
		reqCtx = CloneRequestContextWithAgent(reqCtx, ag)
	}

	// BeforeAgent short-circuit path
	if parts, err := reqCtx.RunBeforeAgent(ctx, ag); err != nil {
		return fmt.Errorf("plugin: before_agent: %w", err)
	} else if parts != nil {
		assist := NewFullAssistantEvent(reqCtx.RunID(), reqCtx.AgentName(), parts...)
		if err := w.Write(ctx, assist); err != nil {
			return fmt.Errorf("failed to write synthetic assistant event: %w", err)
		}
		return runAfterAgent(ctx, reqCtx, ag, w)
	}

	if err := ag.Run(ctx, reqCtx, w); err != nil {
		return err
	}
	return runAfterAgent(ctx, reqCtx, ag, w)
}

// runAfterAgent invokes the AfterAgent plugin hook and, if parts are returned,
// appends a new assistant event. Returns any error encountered.
func runAfterAgent(ctx context.Context, reqCtx RequestContext, ag Agent, w EventWriter) error {
	if afterParts, err := reqCtx.RunAfterAgent(ctx, ag); err != nil {
		return fmt.Errorf("plugin: after_agent: %w", err)
	} else if afterParts != nil {
		repl := NewFullAssistantEvent(reqCtx.RunID(), reqCtx.AgentName(), afterParts...)
		if err := w.Write(ctx, repl); err != nil {
			return fmt.Errorf("failed to write after_agent replacement event: %w", err)
		}
	}

	return nil
}
