package agent

import (
	"context"
	"errors"

	"github.com/hupe1980/agentmesh/core"
)

// ErrRunFuncNil indicates the FuncAgent was constructed without a run function.
var ErrRunFuncNil = errors.New("agent run function is nil")

// FuncAgentOptions holds options for configuring a FuncAgent.
type FuncAgentOptions struct {
	// Human-readable agent description
	Description string
	// Sub-agents managed by this agent
	SubAgents []core.Agent
}

// RunFunc is the signature for a function used by FuncAgent to perform work.
type RunFunc func(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error

// FuncAgent is a minimal agent that delegates Run to a provided function.
// Use this to quickly wrap custom logic into an agent without defining a full type.
type FuncAgent struct {
	*BaseAgent
	run RunFunc
}

// NewFuncAgent constructs a FuncAgent with the given name and run function.
// Children are wired at construction via options; the hierarchy is read-only at runtime.
// The provided function is invoked each time the agent runs. Optional option
// functions can override defaults (e.g., Description).
func NewFuncAgent(name string, run RunFunc, optFns ...func(o *FuncAgentOptions)) *FuncAgent {
	opts := FuncAgentOptions{
		Description: "",
	}

	// Apply options
	for _, fn := range optFns {
		fn(&opts)
	}

	a := &FuncAgent{run: run}
	a.BaseAgent = NewBaseAgent(a, name, opts.Description)
	a.setSubAgents(opts.SubAgents...)

	return a
}

// Run implements core.Agent by delegating to the provided run function.
func (a *FuncAgent) Run(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
	if a.run == nil {
		return ErrRunFuncNil
	}

	return a.run(ctx, reqCtx, queue)
}
