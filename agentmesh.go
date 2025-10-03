package agentmesh

import (
	"context"

	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/flow"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/runner"
	"github.com/hupe1980/agentmesh/tool"
)

// Agent re-exports core.Agent for convenience.
type Agent = core.Agent

// Flow re-exports core.Flow for convenience.
type Flow = core.Flow

// Event re-exports core.Event for convenience.
type Event = core.Event

// Part re-exports core.Part for convenience.
type Part = core.Part

// RunResult re-exports core.RunResult for convenience.
type RunResult = core.RunResult

// Model re-exports core.Model for convenience.
type Model = core.Model

// MemoryStore re-exports core.MemoryStore for convenience.
type MemoryStore = core.MemoryStore

// ArtifactStore re-exports core.ArtifactStore for convenience.
type ArtifactStore = core.ArtifactStore

// SessionStore re-exports core.SessionStore for convenience.
type SessionStore = core.SessionStore

// CredentialStore re-exports core.CredentialStore for convenience.
type CredentialStore = core.CredentialStore

// Instructions re-exports agent.Instructions for convenience.
type Instructions = agent.Instructions

// InstructionsProvider re-exports agent.InstructionsProvider for convenience.
type InstructionsProvider = agent.InstructionsProvider

// InstructionsProviderFunc re-exports agent.InstructionsProviderFunc for convenience.
type InstructionsProviderFunc = agent.InstructionsProviderFunc

// NewInstructionsFromText creates instructions from a literal string.
func NewInstructionsFromText(text string) Instructions { return agent.NewInstructionsFromText(text) }

// NewInstructionsFromProvider creates instructions by delegating to the provided source.
func NewInstructionsFromProvider(p InstructionsProvider) Instructions {
	return agent.NewInstructionsFromProvider(p)
}

// NewInstructionsFromFunc wraps a function that produces instructions at runtime.
func NewInstructionsFromFunc(f func(context.Context, core.ReadonlyContext) (string, error)) Instructions {
	return agent.NewInstructionsFromFunc(f)
}

// NewPartFromText constructs a simple text part.
func NewPartFromText(text string) Part { return core.NewPartFromText(text) }

// NewPartFromFileURI constructs a part referencing a file by URI.
func NewPartFromFileURI(name, uri string) Part { return core.NewPartFromFileURI(name, uri) }

// NewPartFromFunctionCall constructs a part representing a function call.
func NewPartFromFunctionCall(id, name, args string) Part {
	return core.NewPartFromFunctionCall(id, name, args)
}

// NewPartFromFunctionResponse constructs a part capturing a function response payload.
func NewPartFromFunctionResponse(id, name string, response any) Part {
	return core.NewPartFromFunctionResponse(id, name, response)
}

// AgentRunFunc re-exports agent.RunFunc for convenience.
type AgentRunFunc = agent.RunFunc

// FuncAgentOptions configures behaviour for function-backed agents.
type FuncAgentOptions struct {
	agent.FuncAgentOptions
}

// NewFuncAgent creates an agent backed by a simple AgentRunFunc implementation.
func NewFuncAgent(name string, run AgentRunFunc, optFns ...func(o *FuncAgentOptions)) *agent.FuncAgent {
	opts := FuncAgentOptions{
		FuncAgentOptions: agent.DefaultFuncAgentOptions(),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return agent.NewFuncAgent(name, run, func(o *agent.FuncAgentOptions) {
		*o = opts.FuncAgentOptions
	})
}

// ModelAgentOptions augments the underlying agent options with a flow selector.
type ModelAgentOptions struct {
	agent.ModelAgentOptions
	FlowSelector core.FlowSelector
}

// NewModelAgent constructs a model-backed agent with optional configuration.
func NewModelAgent(name string, model Model, optFns ...func(o *ModelAgentOptions)) (*agent.ModelAgent, error) {
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

// ParallelAgentOptions wraps defaults for parallel orchestration.
type ParallelAgentOptions struct {
	agent.ParallelAgentOptions
}

// NewParallelAgent fan-outs execution across the provided agents.
func NewParallelAgent(name string, children []Agent, optFns ...func(o *ParallelAgentOptions)) *agent.ParallelAgent {
	opts := ParallelAgentOptions{
		ParallelAgentOptions: agent.DefaultParallelAgentOptions(),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return agent.NewParallelAgent(name, children, func(o *agent.ParallelAgentOptions) {
		*o = opts.ParallelAgentOptions
	})
}

// NewSequentialAgent composes agents in-order, returning on the first error.
func NewSequentialAgent(name string, children []Agent) *agent.SequentialAgent {
	return agent.NewSequentialAgent(name, children)
}

// LoopAgentOptions wraps defaults for loop agents.
type LoopAgentOptions struct {
	agent.LoopAgentOptions
}

// NewLoopAgent repeatedly invokes the child agent until termination criteria.
func NewLoopAgent(name string, child Agent, optFns ...func(o *LoopAgentOptions)) *agent.LoopAgent {
	opts := LoopAgentOptions{
		LoopAgentOptions: agent.DefaultLoopAgentOptions(),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return agent.NewLoopAgent(name, child, func(o *agent.LoopAgentOptions) {
		*o = opts.LoopAgentOptions
	})
}

// RunnerOptions exposes runner configuration through the façade.
type RunnerOptions struct {
	runner.Options
}

// NewRunner exposes runner.New so callers can override observability, stores,
// and other runtime behaviour while staying within the façade.
func NewRunner(appName string, ag Agent, optFns ...func(o *RunnerOptions)) *runner.Runner {
	opts := RunnerOptions{
		Options: runner.DefaultOptions,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return runner.New(appName, ag, func(o *runner.Options) {
		*o = opts.Options
	})
}

// App wraps a runner and exposes convenience helpers for one-off invocations.
type App struct {
	runner *runner.Runner
}

// NewApp constructs an App with the given agent and runner options.
func NewApp(appName string, ag Agent, optFns ...func(o *RunnerOptions)) *App {
	return &App{runner: NewRunner(appName, ag, optFns...)}
}

// Runner returns the underlying runner for advanced scenarios.
func (a *App) Runner() *runner.Runner { return a.runner }

// Close releases resources owned by the underlying runner.
func (a *App) Close() error { return a.runner.Close() }

// RunOptions augments core.RunOptions with any App-specific settings.
type RunOptions struct {
	core.RunOptions
}

// Run executes the wrapped runner and returns the streaming channel.
func (a *App) Run(
	ctx context.Context,
	userID, sessionID string,
	parts []Part,
	optFns ...func(o *RunOptions),
) (string, <-chan RunResult, error) {
	opts := RunOptions{
		RunOptions: runner.DefaultRunOptions,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return a.runner.Run(ctx, userID, sessionID, parts, func(o *core.RunOptions) {
		*o = opts.RunOptions
	})
}

// RunFinal blocks until completion and returns the final assistant event.
func (a *App) RunFinal(
	ctx context.Context,
	userID, sessionID string,
	parts []Part,
	optFns ...func(o *RunOptions),
) (string, *Event, error) {
	opts := RunOptions{
		RunOptions: runner.DefaultRunOptions,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return runner.RunFinal(ctx, a.runner, userID, sessionID, parts, func(o *core.RunOptions) {
		*o = opts.RunOptions
	})
}

// RunFinalText blocks until completion and returns only the final assistant text.
func (a *App) RunFinalText(
	ctx context.Context,
	userID, sessionID string,
	parts []Part,
	optFns ...func(o *RunOptions),
) (string, string, error) {
	opts := RunOptions{
		RunOptions: runner.DefaultRunOptions,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return runner.RunFinalText(ctx, a.runner, userID, sessionID, parts, func(o *core.RunOptions) {
		*o = opts.RunOptions
	})
}
