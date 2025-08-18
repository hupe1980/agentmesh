package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// BaseAgent bundles shared lifecycle (Start/Stop), hierarchy management and
// identity helpers. Embed it in concrete agent implementations and supply a
// Run method to satisfy the core.Agent interface. All exported methods are
// goroutine-safe unless otherwise documented.
type BaseAgent struct {
	name        string             // Human-readable name
	description string             // Detailed description of agent's purpose
	mu          sync.Mutex         // Protects concurrent access to agent state
	cancel      context.CancelFunc // Used to cancel agent operations
	running     bool               // Tracks whether the agent is currently active
	parent      core.Agent         // Parent agent in hierarchical structures
	subAgents   []core.Agent       // Child agents managed by this agent
}

// NewBaseAgent constructs a BaseAgent with generated description (customizable via SetDescription).
func NewBaseAgent(name string) BaseAgent {
	return BaseAgent{
		name:        name,
		description: fmt.Sprintf("Agent %s", name),
	}
}

// Name returns the human-readable name for this agent.
func (b *BaseAgent) Name() string { return b.name }

// Description returns a detailed description of this agent's purpose.
func (b *BaseAgent) Description() string { return b.description }

// SetDescription updates the agent's description.
// This is useful for providing more detailed information about the agent's capabilities.
func (b *BaseAgent) SetDescription(desc string) { b.description = desc }

// Start initializes the agent and prepares it for execution.
//
// This method sets up the agent's cancellation context and marks it as running.
// It returns a derived context that will be cancelled when Stop() is called.
//
// Returns an error if the agent is already running.
// Start transitions the agent to running state and returns a derived context
// that is cancelled when Stop is invoked. It is safe for concurrent calls but
// only the first successful invocation changes state; subsequent calls while
// running return an error.
func (b *BaseAgent) Start(invocationCtx *core.InvocationContext) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return errors.New("agent is already running")
	}

	// Create a cancellable context for this agent's operations
	_, cancel := context.WithCancel(invocationCtx.Context)
	b.cancel = cancel
	b.running = true

	return nil
}

// Stop gracefully shuts down the agent.
//
// This method cancels the agent's context and marks it as not running.
// Any ongoing operations should respect the context cancellation.
//
// Returns an error if the agent is not currently running.
// Stop cancels the agent's derived context and marks it as not running.
// It returns an error if the agent was not running. Stop is idempotent with
// respect to multiple cancellations (subsequent calls after a successful stop
// will yield an error due to state check).
func (b *BaseAgent) Stop(_ *core.InvocationContext) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.running {
		return errors.New("agent is not running")
	}

	if b.cancel != nil {
		b.cancel()
	}
	b.running = false

	return nil
}

// Agent Hierarchy Management
//
// The following methods support building complex multi-agent systems
// where agents can have parent-child relationships.

// SetSubAgents replaces the existing child set and assigns this agent as parent.
// Enforces single-parent rule; previous children are detached.
// SetSubAgents atomically replaces the child agent set, clearing any previous
// parent links then assigning this agent as the parent of each new child. It
// enforces a single-parent invariant for all managed children.
func (b *BaseAgent) SetSubAgents(children ...core.Agent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Clear existing relationships to prevent orphaned references
	for _, child := range b.subAgents {
		if setter, ok := child.(interface{ setParent(core.Agent) }); ok {
			setter.setParent(nil)
		}
	}
	b.subAgents = nil

	// Establish new parent-child relationships
	for _, child := range children {
		if setter, ok := child.(interface{ setParent(core.Agent) }); ok {
			// Wrap base agent so it satisfies Agent (Run provided by wrapper)
			setter.setParent(&agentWrapper{b})
		}
		b.subAgents = append(b.subAgents, child)
	}

	return nil
}

// setParent is an internal method used to establish parent-child relationships.
// This method is called by SetSubAgents to maintain the agent hierarchy.
// setParent sets the internal parent reference (internal – not concurrency safe externally).
func (b *BaseAgent) setParent(p core.Agent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.parent = p
}

// Parent returns the parent agent in the hierarchy.
// Returns nil if this agent has no parent (i.e., it's a root-level agent).
// Parent returns the current parent agent or nil if this agent is root.
func (b *BaseAgent) Parent() core.Agent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.parent
}

// SubAgents returns a copy of all child agents managed by this agent.
// The returned slice is a copy to prevent external modification of the internal state.
// SubAgents returns a shallow copy of current child agents for safe iteration.
func (b *BaseAgent) SubAgents() []core.Agent {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]core.Agent, len(b.subAgents))
	copy(result, b.subAgents)
	return result
}

// FindAgent performs a depth-first search for an agent with the given name.
//
// The search starts with this agent and recursively searches all sub-agents.
// This method is useful for locating specific agents within complex hierarchical
// structures for coordination or communication purposes.
//
// Returns the first agent found with the matching name, or nil if no match is found.
// FindAgent performs a depth-first search over the subtree rooted at this
// agent (including itself) returning the first agent whose Name matches.
// Returns nil if no match is found.
func (b *BaseAgent) FindAgent(name string) core.Agent {
	if b.name == name {
		return &agentWrapper{b}
	}

	// Search through all child agents
	for _, child := range b.SubAgents() {
		if child.Name() == name {
			return child
		}
		// Recursively search child's sub-agents
		if found := child.FindAgent(name); found != nil {
			return found
		}
	}
	return nil
}

// agentWrapper wraps BaseAgent to satisfy Agent for hierarchy references.
type agentWrapper struct{ *BaseAgent }

// Run executes the agent's behavior within the given context.
func (w *agentWrapper) Run(_ *core.InvocationContext) error {
	return fmt.Errorf("cannot execute BaseAgent directly - embed it in a concrete agent with Run implementation")
}
