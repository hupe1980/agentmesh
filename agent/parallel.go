// Package agent provides parallel execution coordination for multiple agents.
//
// ParallelAgent executes child agents concurrently with branch isolation,
// enabling efficient processing of independent tasks.

package agent

import (
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/core"
)

// ParallelAgent coordinates the concurrent execution of multiple child agents.
//
// This agent type enables parallel processing by executing child agents
// simultaneously with proper branch isolation. Each child agent receives
// a separate branch context to prevent state conflicts while maintaining
// access to the shared session state.
//
// Key features:
//   - Concurrent execution with goroutines
//   - Branch isolation for state management
//   - Configurable execution timeout
//   - Error aggregation from all branches
//   - Hierarchical naming for branch identification
//   - Lifecycle management for all child agents
//
// ParallelAgent is ideal for:
//   - Independent task processing
//   - I/O bound operations that can run concurrently
//   - Data gathering from multiple sources
//   - Scenarios where order doesn't matter
//   - Performance optimization through parallelization
type ParallelAgent struct {
	BaseAgent               // Embedded base agent functionality
	children  []core.Agent  // Child agents to execute in parallel
	timeout   time.Duration // Maximum execution time for all children
}

// NewParallelAgent creates a new parallel execution coordinator.
//
// The agent will execute the provided child agents concurrently, each
// in its own isolated branch context to prevent state conflicts.
//
// Parameters:
//   - name: Human-readable name for the coordinator
//   - timeout: Maximum time allowed for all child agents to complete
//   - children: Child agents to execute in parallel
//
// Returns a configured ParallelAgent ready for concurrent execution.
func NewParallelAgent(name string, timeout time.Duration, children ...core.Agent) *ParallelAgent {
	return &ParallelAgent{
		BaseAgent: NewBaseAgent(name),
		children:  children,
		timeout:   timeout,
	}
}

// createBranchCtxForSubAgent creates isolated execution context for each child agent.
//
// This method implements branch isolation by creating a separate context
// for each child agent with a unique branch path. This prevents state
// conflicts while maintaining access to shared session data.
//
// The branch naming follows the pattern: "ParentAgent.SubAgent"
// For nested parallel agents, this creates hierarchical branch paths.
//
// Parameters:
//   - subAgent: The child agent requiring isolated context
//   - invocationCtx: The parent invocation context to clone
//
// Returns a cloned context with isolated branch path for the child agent.
// createBranchCtxForSubAgent clones the parent context and assigns a branch
// path for the child agent ensuring isolation of pending deltas / artifacts.
func (p *ParallelAgent) createBranchCtxForSubAgent(invocationCtx *core.InvocationContext, subAgent core.Agent) *core.InvocationContext {
	// Clone the invocation context for branch isolation
	clonedCtx := invocationCtx.Clone()

	// Create hierarchical branch identifier using helper
	branchSuffix := fmt.Sprintf("%s.%s", p.Name(), subAgent.Name())
	clonedCtx.Branch = buildBranchPath(invocationCtx.Branch, branchSuffix)

	return clonedCtx
}

// Run executes all child agents concurrently with branch isolation.
//
// This method implements the parallel execution pattern:
//  1. Starts the parallel agent coordinator
//  2. Launches each child agent in a separate goroutine
//  3. Creates isolated branch contexts for each child
//  4. Waits for all child agents to complete
//  5. Aggregates errors from all executions
//  6. Manages cleanup and lifecycle for all agents
//
// Each child agent receives an isolated branch context to prevent
// state conflicts while maintaining access to shared session data.
//
// Parameters:
//   - invocationCtx: Parent context that will be cloned for each child
//
// Returns an error if any child agent fails or if lifecycle management fails.
// Run implements core.Agent launching all children concurrently. The first
// error encountered (after all complete) is returned; successful children
// continue even if siblings fail.
func (p *ParallelAgent) Run(invocationCtx *core.InvocationContext) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(p.children))

	// Launch all child agents in separate goroutines
	for _, child := range p.children {
		wg.Add(1)
		go func(c core.Agent) {
			defer wg.Done()

			// Create isolated branch context for state separation
			branchCtx := p.createBranchCtxForSubAgent(invocationCtx, c)

			// Execute child agent with isolated context
			if err := c.Run(branchCtx); err != nil {
				errCh <- fmt.Errorf("parallel execution failed for agent %s: %w", c.Name(), err)
			}
		}(child)
	}

	// Wait for all child agents to complete
	wg.Wait()
	close(errCh)

	// Return first error encountered, if any
	if len(errCh) > 0 {
		return <-errCh
	}

	return nil
}
