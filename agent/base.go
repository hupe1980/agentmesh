package agent

import (
	"github.com/hupe1980/agentmesh/core"
)

// parentSetter is a private interface implemented by agents in this package
// that need to allow BaseAgent to wire parent relationships internally.
type parentSetter interface {
	setParent(core.Agent)
}

// BaseAgent bundles shared identity and hierarchy helpers for concrete agents.
// Embed it in your agent types and supply a Run method to satisfy core.Agent.
// Mutation of the hierarchy is internal-only; callers build trees via
// constructors.
type BaseAgent struct {
	self        core.Agent   // Concrete agent embedding this BaseAgent (for Parent/FindAgent)
	name        string       // Human-readable name
	description string       // Detailed description of agent's purpose
	parent      core.Agent   // Parent agent in hierarchical structures
	subAgents   []core.Agent // Child agents managed by this agent
}

// NewBaseAgent constructs a BaseAgent.
func NewBaseAgent(self core.Agent, name, description string) *BaseAgent {
	if self == nil {
		panic("self agent is nil (pass the concrete agent)")
	}

	return &BaseAgent{
		self:        self,
		name:        name,
		description: description,
	}
}

// Name returns the human-readable name for this agent.
func (b *BaseAgent) Name() string { return b.name }

// Description returns a detailed description of this agent's purpose.
func (b *BaseAgent) Description() string { return b.description }

// setSubAgents replaces this agent's child set (internal wiring used by constructors).
func (b *BaseAgent) setSubAgents(children ...core.Agent) {
	// Clear existing relationships to prevent orphaned references
	for _, child := range b.subAgents {
		if ps, ok := child.(parentSetter); ok {
			ps.setParent(nil)
		}
	}

	b.subAgents = nil

	// Establish new parent-child relationships
	for _, child := range children {
		if ps, ok := child.(parentSetter); ok {
			ps.setParent(b.self)
		}

		b.subAgents = append(b.subAgents, child)
	}
}

// setParent establishes the parent-child relationship for this agent (internal).
func (b *BaseAgent) setParent(p core.Agent) {
	b.parent = p
}

// Parent returns the current parent agent or nil if this agent is root.
func (b *BaseAgent) Parent() core.Agent {
	return b.parent
}

// SubAgents returns a shallow copy of current child agents for safe iteration.
func (b *BaseAgent) SubAgents() []core.Agent {
	// Return a copy to prevent external mutation
	result := make([]core.Agent, len(b.subAgents))
	copy(result, b.subAgents)

	return result
}

// HasSubAgents returns true if this agent manages one or more sub-agents.
func (b *BaseAgent) HasSubAgents() bool {
	return len(b.subAgents) > 0
}

// FindAgent performs a depth-first search starting at this agent and including
// its descendants, returning the first agent whose Name matches.
//
// If the provided name matches this agent's Name, this agent is returned.
// If no agent matches, ErrAgentNotFound is returned.
func (b *BaseAgent) FindAgent(name string) (core.Agent, error) {
	if name == b.name {
		return b.self, nil
	}

	return b.FindSubAgent(name)
}

// FindSubAgent searches only within this agent's descendants (excluding self)
// using depth-first traversal and returns the first matching agent.
// If no descendant matches, ErrAgentNotFound is returned.
func (b *BaseAgent) FindSubAgent(name string) (core.Agent, error) {
	// Search through all child agents
	for _, sub := range b.SubAgents() {
		if result, err := sub.FindAgent(name); err == nil {
			return result, nil
		}
	}

	return nil, core.ErrAgentNotFound
}

// RootAgent returns the root ancestor of this agent in the hierarchy.
func (b *BaseAgent) RootAgent() core.Agent {
	current := b.self
	for parent := b.Parent(); parent != nil; parent = parent.Parent() {
		current = parent
	}

	return current
}
