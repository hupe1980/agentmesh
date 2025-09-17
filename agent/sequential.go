package agent

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/executor"
)

// SequentialAgentOptions holds configuration for a SequentialAgent.
type SequentialAgentOptions struct {
	// Human-readable agent description
	Description string
	// Agent executor for running agent tasks
	AgentExecutor core.AgentExecutor
}

// SequentialAgent coordinates the execution of multiple child agents in sequence.
//
// This agent type enables complex workflows by executing child agents one after
// another, passing the accumulated session state between them. Each agent's
// output becomes available to subsequent agents in the sequence.
type SequentialAgent struct {
	*BaseAgent                       // Embedded base agent functionality
	agentExecutor core.AgentExecutor // Executor for running agent tasks
}

// NewSequentialAgent creates a new sequential execution coordinator.
// Children are wired at construction; the hierarchy is read-only at runtime.
// The agent executes the provided child agents in order, passing the same
// RequestContext and queue so state and outputs flow through the pipeline.
func NewSequentialAgent(
	name string,
	subAgents []core.Agent,
	optFns ...func(o *SequentialAgentOptions),
) *SequentialAgent {
	opts := SequentialAgentOptions{
		Description:   "",
		AgentExecutor: executor.DefaultAgentExecutor,
	}

	// Apply option functions to override defaults
	for _, fn := range optFns {
		fn(&opts)
	}

	a := &SequentialAgent{agentExecutor: opts.AgentExecutor}
	a.BaseAgent = NewBaseAgent(a, name, opts.Description)
	if len(subAgents) > 0 {
		if err := a.AddSubAgents(subAgents...); err != nil {
			panic(err) // Should not happen with valid input
		}
	}

	return a
}

// Run executes all child agents sequentially with shared session state.
func (s *SequentialAgent) Run(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
	// Execute child agents in sequence, propagating state between them
	for _, child := range s.SubAgents() {
		// Pass the same request context and queue to maintain shared state
		if err := s.agentExecutor.Execute(ctx, reqCtx, child, queue); err != nil {
			return fmt.Errorf("sequential execution failed at agent %s: %w", child.Name(), err)
		}
	}

	return nil
}

// Interface compliance (compile-time assertions)
var _ core.Agent = (*SequentialAgent)(nil)
