package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/core"
)

// ParallelAgentOptions holds configuration for a ParallelAgent.
type ParallelAgentOptions struct {
	// Human-readable agent description
	Description string
	// Maximum execution time for all children
	Timeout time.Duration
	// Agent executor for running agent tasks
	AgentExecutor core.AgentExecutor
}

// ParallelAgent coordinates the concurrent execution of multiple child agents.
//
// This agent type enables parallel processing by executing child agents
// simultaneously with proper branch isolation. Each child agent receives
// a separate branch context to prevent state conflicts while maintaining
// access to the shared session state.
type ParallelAgent struct {
	*BaseAgent                       // Embedded base agent functionality
	timeout       time.Duration      // Maximum execution time for all children
	agentExecutor core.AgentExecutor // Executor for running agent tasks
}

// NewParallelAgent creates a new parallel execution coordinator.
// Children are wired at construction; the hierarchy is read-only at runtime.
// The agent executes the provided children concurrently, each with its own
// branch (via RequestContext.NewBranchContextForSubAgent), preventing state
// conflicts while sharing session state. If timeout > 0, each child run is
// bounded by the specified duration.
func NewParallelAgent(name string, subAgents []core.Agent, optFns ...func(o *ParallelAgentOptions)) *ParallelAgent {
	opts := ParallelAgentOptions{
		Description:   "",
		Timeout:       0, // No timeout by default
		AgentExecutor: DefaultAgentExecutor,
	}

	// Apply option functions to override defaults
	for _, fn := range optFns {
		fn(&opts)
	}

	a := &ParallelAgent{
		timeout:       opts.Timeout,
		agentExecutor: opts.AgentExecutor,
	}

	a.BaseAgent = NewBaseAgent(a, name, opts.Description)
	if len(subAgents) > 0 {
		if err := a.AddSubAgents(subAgents...); err != nil {
			panic(err)
		}
	}

	return a
}

// Run executes all child agents concurrently with branch isolation.
func (p *ParallelAgent) Run(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
	var wg sync.WaitGroup

	subAgents := p.SubAgents()
	errCh := make(chan error, len(subAgents))

	// Launch all child agents in separate goroutines
	for _, child := range subAgents {
		wg.Add(1)
		go func(c core.Agent) {
			defer wg.Done()

			// Create isolated branch context for state separation
			branchCtx := reqCtx.NewBranchContextForSubAgent(fmt.Sprintf("%s.%s", p.Name(), c.Name()))

			// Apply timeout if configured
			if p.timeout > 0 {
				timeoutCtx, cancel := context.WithTimeout(ctx, p.timeout)
				defer cancel()

				ctx = timeoutCtx
			}

			// Execute child agent with isolated context
			if err := p.agentExecutor.Execute(ctx, branchCtx, c, writer); err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
					err = fmt.Errorf("parallel execution timed out for agent %s: %w", c.Name(), core.ErrParallelTimeout)
				}
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

// Interface compliance (compile-time assertions)
var _ core.Agent = (*ParallelAgent)(nil)
