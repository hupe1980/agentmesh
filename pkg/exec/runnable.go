package exec

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

var (
	// ErrInvalidInput is returned when input type assertion fails
	ErrInvalidInput = errors.New("invalid input type")

	// ErrInvalidOutput is returned when output type assertion fails
	ErrInvalidOutput = errors.New("invalid output type")
)

// RunnableGraph wraps a CompiledGraph to implement graph.MessageRunnable.
type RunnableGraph struct {
	compiled       *compile.CompiledGraph
	executor       Executor
	runtimeMetrics *RuntimeMetrics
}

// NewRunnable creates a MessageRunnable from a compiled graph and executor.
func NewRunnable(compiled *compile.CompiledGraph, executor Executor) graph.MessageRunnable {
	if executor == nil {
		executor = NewPregelExecutor()
	}
	return &RunnableGraph{
		compiled:       compiled,
		executor:       executor,
		runtimeMetrics: NewRuntimeMetrics(),
	}
}

// Run executes the graph with the given messages.
func (rg *RunnableGraph) Run(ctx context.Context, messages []message.Message, opts ...graph.RunOption) iter.Seq2[state.ExecutionResult, error] {
	return rg.executor.Run(ctx, rg.compiled, messages, opts...)
}

// NewTyped creates a generic typed wrapper around a MessageRunnable.
func NewTyped[I, O any](runnable graph.MessageRunnable) graph.Runnable[I, O] {
	return &typedRunnable[I, O]{
		inner: runnable,
	}
}

// typedRunnable wraps MessageRunnable with generic type parameters.
type typedRunnable[I, O any] struct {
	inner graph.MessageRunnable
}

// Run executes with generic types (type assertion at runtime).
func (tr *typedRunnable[I, O]) Run(ctx context.Context, input I, opts ...graph.RunOption) iter.Seq2[O, error] {
	// Type assert input to []message.Message
	messages, ok := any(input).([]message.Message)
	if !ok {
		return func(yield func(O, error) bool) {
			var zero O
			yield(zero, ErrInvalidInput)
		}
	}

	// Run the inner runnable
	results := tr.inner.Run(ctx, messages, opts...)

	// Convert results to output type
	return func(yield func(O, error) bool) {
		for result, err := range results {
			if err != nil {
				var zero O
				if !yield(zero, err) {
					return
				}
				continue
			}

			output, ok := any(result).(O)
			if !ok {
				// Type conversion failed
				var zero O
				if !yield(zero, ErrInvalidOutput) {
					return
				}
				continue
			}
			if !yield(output, nil) {
				return
			}
		}
	}
}

// CompileGraph bridges the old graph.Compile() API to the new clean architecture.
// This is the main entry point that compiles a graph into an executable MessageRunnable.
//
// Architecture: graph (structure) → compile (topology) → exec (execution)
//
// Following refactoring_summary.md pattern:
// - Main function: ~26 lines (high-level orchestration)
// - Composition over complexity
//
// Example:
//
//	g, _ := graph.NewGraph(stateManager)
//	g.AddNode(modelNode)
//	g.AddEdge(compile.StartNode, "model")
//	runnable, err := exec.CompileGraph(g)
//	results := runnable.Run(ctx, messages)
func CompileGraph(g *graph.Graph, opts ...CompileOption) (graph.MessageRunnable, error) {
	// Setup configuration (SRP: single responsibility - config setup)
	cfg := setupCompilation(opts)

	// Validate graph structure early
	if g == nil {
		return nil, errors.New("graph cannot be nil")
	}

	// Step 1: Compile topology using pkg/compile with validation options
	compiled, err := compile.Compile(g, g.State(), cfg.compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("compilation failed: %w", err)
	}

	// Step 2: Wrap with executor (default to Pregel)
	executor := cfg.executor
	if executor == nil {
		executor = NewPregelExecutor()
	}

	// Step 3: Create runnable wrapper
	return NewRunnable(compiled, executor), nil
}

// CompileOption configures the compilation process.
type CompileOption func(*compileConfig)

// compileConfig holds compilation configuration.
type compileConfig struct {
	executor    Executor
	compileOpts []compile.CompileOption
}

// setupCompilation extracts configuration setup (SRP: single responsibility).
// Follows refactoring_summary.md pattern: extract setup logic.
func setupCompilation(opts []CompileOption) *compileConfig {
	cfg := &compileConfig{
		executor: nil, // Will use default Pregel if not provided
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WithExecutor sets the executor to use for graph execution.
func WithExecutor(executor Executor) CompileOption {
	return func(cfg *compileConfig) {
		cfg.executor = executor
	}
}

// WithValidation sets custom validation options for graph compilation.
func WithValidation(opts compile.ValidationOptions) CompileOption {
	return func(cfg *compileConfig) {
		cfg.compileOpts = append(cfg.compileOpts, compile.WithValidation(opts))
	}
}

// WithStrictValidation enables strict validation mode.
// This enforces:
//   - No unreachable nodes
//   - No dead-end nodes
//   - No cycles
//   - Required START and END connections
func WithStrictValidation() CompileOption {
	return func(cfg *compileConfig) {
		cfg.compileOpts = append(cfg.compileOpts, compile.WithStrictValidation())
	}
}

// WithoutValidation disables validation (use with caution).
// Only use this for trusted graphs or when validation overhead is unacceptable.
func WithoutValidation() CompileOption {
	return func(cfg *compileConfig) {
		cfg.compileOpts = append(cfg.compileOpts, compile.WithoutValidation())
	}
}

// NewBuilder creates a new graph builder with CompileGraph pre-configured.
// This allows using the fluent builder API with automatic compilation.
//
// Example:
//
//	builder, err := exec.NewBuilder()
//	builder.Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
//	    return &graph.NodeResult{Updates: map[string]any{"done": true}}, nil
//	})
//	builder.AddEdge(graph.StartNode, "process")
//	builder.AddEdge("process", graph.EndNode)
//	compiled, err := builder.Compile()
func NewBuilder(opts ...graph.BuilderOption) (*graph.Builder, error) {
	// Create a wrapper that matches the expected signature
	compileFunc := func(g *graph.Graph) (graph.MessageRunnable, error) {
		return CompileGraph(g)
	}

	// Add the compile function to the options
	allOpts := append([]graph.BuilderOption{graph.WithCompileFunc(compileFunc)}, opts...)
	return graph.NewBuilder(allOpts...)
}

// Introspection methods - delegate to the compiled graph

// GetNodes returns a sorted list of all node names.
func (rg *RunnableGraph) GetNodes() []string {
	return rg.compiled.GetNodes()
}

// GetNodeInfo returns detailed information about a specific node.
func (rg *RunnableGraph) GetNodeInfo(name string) (*compile.NodeInfo, error) {
	return rg.compiled.GetNodeInfo(name)
}

// GetTopology returns a comprehensive view of the graph structure.
func (rg *RunnableGraph) GetTopology() *compile.Topology {
	return rg.compiled.GetTopology()
}

// GetMetrics returns static graph metrics (node counts, edges, complexity).
// For runtime metrics (superstep, completed nodes), use GetRuntimeMetrics().
func (rg *RunnableGraph) GetMetrics() *compile.Metrics {
	return rg.compiled.GetMetrics()
}

// GetNodeDependencies returns dependency information for a specific node.
func (rg *RunnableGraph) GetNodeDependencies(name string) (*compile.NodeDependencies, error) {
	return rg.compiled.GetNodeDependencies(name)
}

// MermaidFlowchart generates a Mermaid flowchart representation.
func (rg *RunnableGraph) MermaidFlowchart(direction string) string {
	return rg.compiled.MermaidFlowchart(direction)
}

// GetRuntimeMetrics returns current execution metrics.
// This includes superstep number, completed/paused/active nodes, etc.
func (rg *RunnableGraph) GetRuntimeMetrics() *RuntimeMetrics {
	return rg.runtimeMetrics
}

// State returns the state manager for accessing graph state.
// This allows reading and modifying the graph's state directly.
//
// Example:
//
//	state := runnable.State()
//	value := state.Get[string](myKey)
//	state.Set(ctx, myKey, "value")
//	snapshot := state.Snapshot()
func (rg *RunnableGraph) State() *state.State {
	return rg.compiled.State
}

// ApplyState applies state updates to the graph's state manager.
// This is useful for resuming execution with modified state.
//
// Example:
//
//	updates := state.Updates{}
//	state.SetInUpdates(updates, approvedKey, true)
//	state.SetInUpdates(updates, userInputKey, "proceed")
//	runnable.ApplyState(ctx, updates)
func (rg *RunnableGraph) ApplyState(ctx context.Context, updates state.Updates) error {
	if err := rg.compiled.State.ApplyUpdates(ctx, updates); err != nil {
		return fmt.Errorf("failed to apply state updates: %w", err)
	}
	return nil
}

// CurrentSuperstep returns the current superstep number from runtime metrics.
// Returns 0 if execution hasn't started yet.
func (rg *RunnableGraph) CurrentSuperstep() int64 {
	snapshot := rg.runtimeMetrics.Snapshot()
	return snapshot.CurrentSuperstep
}
