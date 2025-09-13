package agent

import (
	"github.com/hupe1980/agentmesh/core"
)

// BaseAgent bundles shared identity and hierarchy helpers for concrete agents.
// Embed it in your agent types and supply a Run method to satisfy core.Agent.
// Use AddSubAgents to build trees; SetParent is invoked internally.
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

// SetParent assigns a parent exactly once.
// Returns an error if a parent is already set (immutable),
// if attempting self-parenting, nil, or creating a cycle.
func (b *BaseAgent) SetParent(p core.Agent) error {
	if p == nil {
		return core.ErrParentNil
	}

	if p == b.self {
		return core.ErrSelfParent
	}

	if b.parent != nil && b.parent != p {
		return core.ErrParentAlreadySet
	}

	for ancestor := p; ancestor != nil; ancestor = ancestor.Parent() { // cycle detection
		if ancestor == b.self {
			return core.ErrParentCycle
		}
	}

	b.parent = p

	return nil
}

// AddSubAgents appends child agents and sets their parent (once). If a child already
// has a different parent, an error is returned and no further children are added.
func (b *BaseAgent) AddSubAgents(children ...core.Agent) error {
	toAdd := make([]core.Agent, 0, len(children))

	for _, child := range children {
		if child == nil {
			continue
		}

		if child == b.self {
			return core.ErrSubAgentSelf
		}

		if err := child.SetParent(b.self); err != nil {
			return err
		}

		toAdd = append(toAdd, child)
	}

	b.subAgents = append(b.subAgents, toAdd...)

	return nil
}

// Parent returns the current parent agent or nil if this agent is root.
func (b *BaseAgent) Parent() core.Agent {
	return b.parent
}

// SubAgents returns a shallow copy of current child agents for safe iteration.
func (b *BaseAgent) SubAgents() []core.Agent {
	result := make([]core.Agent, len(b.subAgents))
	copy(result, b.subAgents)

	return result
}

// HasSubAgents returns true if this agent manages one or more sub-agents.
func (b *BaseAgent) HasSubAgents() bool { return len(b.subAgents) > 0 }

// FindAgent performs a depth-first search starting at this agent and including
// its descendants, returning the first agent whose Name matches.
func (b *BaseAgent) FindAgent(name string) (core.Agent, error) {
	if name == b.name {
		return b.self, nil
	}

	return b.findSubAgent(name)
}

// findSubAgent searches only within this agent's descendants (excluding self)
func (b *BaseAgent) findSubAgent(name string) (core.Agent, error) {
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
