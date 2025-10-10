package agentmesh

import (
	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/app"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/flow"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/runner"
	"github.com/hupe1980/agentmesh/tool"
)

type (
	// FuncAgentOptions are the options for the function agent.
	FuncAgentOptions = agent.FuncAgentOptions
	// LoopAgentOptions are the options for the loop agent.
	LoopAgentOptions = agent.LoopAgentOptions
	// ParallelAgentOptions are the options for the parallel agent.
	ParallelAgentOptions = agent.ParallelAgentOptions
	// AppOptions are the options for the application.
	AppOptions = app.Options
	// RunnerOptions are the options for the runner.
	RunnerOptions = runner.Options
)

// ModelAgentOptions augments the underlying agent options with a flow selector.
type ModelAgentOptions struct {
	agent.ModelAgentOptions
	FlowSelector core.FlowSelector
}

// NewFuncAgent constructs a func agent using the underlying implementation.
func NewFuncAgent(name string, run agent.RunFunc, optFns ...func(*FuncAgentOptions)) *agent.FuncAgent {
	return agent.NewFuncAgent(name, run, optFns...)
}

// NewModelAgent constructs a model-backed agent with an optional flow selector override.
func NewModelAgent(
	name string,
	model core.Model,
	optFns ...func(*ModelAgentOptions),
) (*agent.ModelAgent, error) {
	opts := ModelAgentOptions{
		ModelAgentOptions: agent.DefaultModelAgentOptions(name),
		FlowSelector:      defaultFlowSelector(),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	if opts.FlowSelector == nil {
		opts.FlowSelector = defaultFlowSelector()
	}

	return agent.NewModelAgent(name, model, opts.FlowSelector, func(o *agent.ModelAgentOptions) {
		*o = opts.ModelAgentOptions
	})
}

// defaultFlowSelector provides a sensible default for most applications.
func defaultFlowSelector() core.FlowSelector {
	return flow.NewDefaultSelector(&flow.Executors{
		AgentExecutor: agent.DefaultAgentExecutor,
		ModelExecutor: model.DefaultModelExecutor,
		ToolExecutor:  tool.NewParallelToolExecutor(4),
	})
}

// NewParallelAgent fan-outs execution across the provided agents.
func NewParallelAgent(
	name string,
	children []core.Agent,
	optFns ...func(*ParallelAgentOptions),
) *agent.ParallelAgent {
	return agent.NewParallelAgent(name, children, optFns...)
}

// NewSequentialAgent composes agents in-order, returning on the first error.
func NewSequentialAgent(name string, children []core.Agent) *agent.SequentialAgent {
	return agent.NewSequentialAgent(name, children)
}

// NewLoopAgent repeatedly invokes the child agent until termination criteria.
func NewLoopAgent(name string, child core.Agent, optFns ...func(*LoopAgentOptions)) *agent.LoopAgent {
	return agent.NewLoopAgent(name, child, optFns...)
}

// NewApp constructs an application with optional configuration overrides.
func NewApp(name string, root core.Agent, optFns ...func(*AppOptions)) core.App {
	opts := app.DefaultOptions

	for _, fn := range optFns {
		fn(&opts)
	}

	return app.New(name, root, func(o *app.Options) {
		*o = opts
	})
}

// NewRunner exposes runner.New so callers can override observability, stores, and runtime behaviour.
func NewRunner(application core.App, optFns ...func(*RunnerOptions)) *runner.Runner {
	opts := runner.DefaultOptions

	for _, fn := range optFns {
		fn(&opts)
	}

	return runner.New(application, func(o *runner.Options) {
		*o = opts
	})
}
