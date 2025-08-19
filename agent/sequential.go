package agent

import (
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// SequentialAgent coordinates the execution of multiple child agents in sequence.
//
// This agent type enables complex workflows by executing child agents one after
// another, passing the accumulated session state between them. Each agent's
// output becomes available to subsequent agents in the sequence.
//
// Key features:
//   - Ordered execution with state propagation
//   - Early termination on errors
//   - Session state accumulation across steps
//   - Hierarchical agent management
//   - Lifecycle management for all child agents
//
// SequentialAgent is ideal for:
//   - Multi-step data processing pipelines
//   - Workflows requiring specific execution order
//   - Complex tasks broken into specialized subtasks
//   - Scenarios where agent outputs build upon each other
type SequentialAgent struct {
	BaseAgent              // Embedded base agent functionality
	children  []core.Agent // Child agents to execute in sequence
}

// NewSequentialAgent creates a new sequential execution coordinator.
//
// The agent will execute the provided child agents in the order they are
// specified, passing session state between each execution step.
//
// Parameters:
//   - name: Human-readable name for the coordinator
//   - children: Child agents to execute in sequence
//
// Returns a configured SequentialAgent ready for execution.
func NewSequentialAgent(name string, children ...core.Agent) *SequentialAgent {
	return &SequentialAgent{
		BaseAgent: NewBaseAgent(name),
		children:  children,
	}
}

// Run executes all child agents sequentially with shared session state.
//
// This method implements the sequential execution pattern:
//  1. Starts the sequential agent coordinator
//  2. Executes each child agent in the specified order
//  3. Propagates session state between executions
//  4. Stops execution on the first error encountered
//  5. Manages cleanup and lifecycle for all agents
//
// The same RunContext is passed to all child agents, allowing
// them to share session state and build upon each other's results.
//
// Parameters:
//   - invocationCtx: Shared context with session state and execution metadata
//
// Returns an error if any child agent fails or if lifecycle management fails.
// Run implements core.Agent. It executes each child agent in the supplied
// context order; errors stop further processing immediately.
func (s *SequentialAgent) Run(invocationCtx *core.RunContext) error {
	// Execute child agents in sequence, propagating state between them
	for _, child := range s.children {
		// Pass the same invocation context to maintain shared state
		if err := child.Run(invocationCtx); err != nil {
			return fmt.Errorf("sequential execution failed at agent %s: %w", child.Name(), err)
		}
	}

	return nil
}
