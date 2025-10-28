package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

var (
	// ErrRoutingSelectorNil indicates the selector function was not provided.
	ErrRoutingSelectorNil = errors.New("routing selector is nil")
	// ErrRoutingNoAgents is returned when the agent has no children to evaluate.
	ErrRoutingNoAgents = errors.New("routing agent has no sub-agents")
	// ErrRoutingInvalidChoice signals the selector returned an unknown child.
	ErrRoutingInvalidChoice = errors.New("routing selector returned invalid agent")
)

// RoutingFunc inspects the incoming context and returns the name of the child to run.
type RoutingFunc func(ctx context.Context, roCtx core.ReadonlyContext, agents []core.Agent) (string, error)

// RoutingAgentOptions configures description and execution behavior.
type RoutingAgentOptions struct {
	Description   string
	AgentExecutor core.AgentExecutor
}

// DefaultRoutingAgentOptions returns baseline defaults for routing agents.
func DefaultRoutingAgentOptions() RoutingAgentOptions {
	return RoutingAgentOptions{
		Description:   "",
		AgentExecutor: DefaultAgentExecutor,
	}
}

// RoutingAgent selects a single child at runtime based on a selector function.
type RoutingAgent struct {
	*BaseAgent
	selector      RoutingFunc
	agentExecutor core.AgentExecutor
}

// NewRoutingAgent wires the child agents and stores the selection function.
func NewRoutingAgent(
	name string,
	agents []core.Agent,
	selector RoutingFunc,
	optFns ...func(o *RoutingAgentOptions),
) *RoutingAgent {
	opts := DefaultRoutingAgentOptions()

	for _, fn := range optFns {
		fn(&opts)
	}

	if opts.AgentExecutor == nil {
		opts.AgentExecutor = DefaultAgentExecutor
	}

	a := &RoutingAgent{
		selector:      selector,
		agentExecutor: opts.AgentExecutor,
	}

	a.BaseAgent = NewBaseAgent(a, name, opts.Description)
	if len(agents) > 0 {
		if err := a.AddSubAgents(agents...); err != nil {
			panic(err)
		}
	}

	return a
}

// Run evaluates the selector and delegates execution to the chosen child.
func (a *RoutingAgent) Run(ctx context.Context, reqCtx core.RequestContext, writer core.EventWriter) error {
	if a.selector == nil {
		return ErrRoutingSelectorNil
	}

	subAgents := a.SubAgents()
	if len(subAgents) == 0 {
		return ErrRoutingNoAgents
	}

	selectedName, err := a.selector(ctx, reqCtx, subAgents)
	if err != nil {
		return fmt.Errorf("routing selection failed: %w", err)
	}

	if selectedName == "" {
		return ErrRoutingInvalidChoice
	}

	var target core.Agent
	for _, candidate := range subAgents {
		if candidate.Name() == selectedName {
			target = candidate
			break
		}
	}

	if target == nil {
		name := selectedName
		if name == "" {
			name = "<unnamed>"
		}

		return fmt.Errorf("%w: %s", ErrRoutingInvalidChoice, name)
	}

	return a.agentExecutor.Execute(ctx, reqCtx, target, writer)
}

var _ core.Agent = (*RoutingAgent)(nil)
