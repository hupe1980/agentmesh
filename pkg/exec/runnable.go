package exec

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// RunnableGraph wraps a CompiledGraph to implement graph.Runnable.
// Type parameters:
//   - I: Input type accepted by the executor
//   - O: Output type produced by the executor
type RunnableGraph[I, O any] struct {
	compiled       *compile.CompiledGraph
	executor       Executor[I, O]
	runtimeMetrics *RuntimeMetrics
}

// NewRunnable creates a Runnable from a compiled graph and executor.
// Type parameters:
//   - I: Input type accepted by the executor
//   - O: Output type produced by the executor
func NewRunnable[I, O any](compiled *compile.CompiledGraph, executor Executor[I, O]) graph.Runnable[I, O] {
	return &RunnableGraph[I, O]{
		compiled:       compiled,
		executor:       executor,
		runtimeMetrics: NewRuntimeMetrics(),
	}
}

// Run executes the graph with the given input.
func (rg *RunnableGraph[I, O]) Run(ctx context.Context, input I, opts ...graph.RunOption) iter.Seq2[O, error] {
	return rg.executor.Run(ctx, rg.compiled, input, opts...)
}

// CompileGraph compiles a graph into an executable Runnable.
// Fully generic - works with any input and output types.
//
// Architecture: graph (structure) → compile (topology) → exec (execution)
//
// Type parameters:
//   - I: Input type for the executor
//   - O: Output type for the executor
//
// Examples:
//
//	// Default: messages in, messages out (Pregel executor)
//	runnable, err := exec.CompileGraph(g, exec.NewPregelExecutor())
//
//	// Sequential execution
//	runnable, err := exec.CompileGraph(g, exec.NewSequential())
//
//	// Custom types
//	customExecutor := NewCustomExecutor[MyInput, MyOutput]()
//	runnable, err := exec.CompileGraph(g, customExecutor)
//
//	// With validation options
//	runnable, err := exec.CompileGraph(g, exec.NewPregelExecutor(), exec.WithStrictValidation())
func CompileGraph[I, O any](g *graph.Graph, executor Executor[I, O], opts ...CompileOption) (graph.Runnable[I, O], error) {
	// Validate inputs
	if g == nil {
		return nil, errors.New("graph cannot be nil")
	}
	if executor == nil {
		return nil, errors.New("executor cannot be nil")
	}

	// Setup configuration
	cfg := setupCompilation(opts)

	// Compile topology using pkg/compile with validation options
	compiled, err := compile.Compile(g, g.Manager(), cfg.compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("compilation failed: %w", err)
	}

	// Create runnable wrapper
	return NewRunnable(compiled, executor), nil
}

// CompileOption configures the compilation process.
type CompileOption func(*compileConfig)

// compileConfig holds compilation configuration.
type compileConfig struct {
	compileOpts []compile.CompileOption
}

// setupCompilation extracts configuration setup (SRP: single responsibility).
// Follows refactoring_summary.md pattern: extract setup logic.
func setupCompilation(opts []CompileOption) *compileConfig {
	cfg := &compileConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
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
// Fully generic - type parameters are inferred from the executor.
//
// Type parameters:
//   - I: Input type for the executor
//   - O: Output type for the executor
//
// Examples:
//
//	// Default: Pregel executor with message.Message types
//	builder, err := exec.NewBuilder(exec.NewPregelExecutor())
//
//	// Sequential executor with message.Message types
//	builder, err := exec.NewBuilder(exec.NewSequential())
//
//	// Custom executor with custom types
//	customExecutor := NewCustomExecutor[MyInput, MyOutput]()
//	builder, err := exec.NewBuilder(customExecutor)
//
// Usage:
//
//	builder.Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
//	    return &graph.NodeResult{Updates: map[string]any{"done": true}}, nil
//	})
//	builder.AddEdge(graph.StartNode, "process")
//	builder.AddEdge("process", graph.EndNode)
//	compiled, err := builder.Compile()
func NewBuilder[I, O any](executor Executor[I, O], opts ...graph.BuilderOption[I, O]) (*graph.Builder[I, O], error) {
	// Create a wrapper that uses the provided executor
	compileFunc := func(g *graph.Graph) (graph.Runnable[I, O], error) {
		return CompileGraph(g, executor)
	}

	// Add the compile function to the options
	allOpts := append([]graph.BuilderOption[I, O]{graph.WithCompileFunc[I, O](compileFunc)}, opts...)
	return graph.NewBuilder[I, O](allOpts...)
}

// Introspection methods - delegate to the compiled graph

// GetNodes returns a sorted list of all node names.
func (rg *RunnableGraph[I, O]) GetNodes() []string {
	return rg.compiled.GetNodes()
}

// GetNodeInfo returns detailed information about a specific node.
func (rg *RunnableGraph[I, O]) GetNodeInfo(name string) (*compile.NodeInfo, error) {
	return rg.compiled.GetNodeInfo(name)
}

// GetTopology returns a comprehensive view of the graph structure.
func (rg *RunnableGraph[I, O]) GetTopology() *compile.Topology {
	return rg.compiled.GetTopology()
}

// GetMetrics returns static graph metrics (node counts, edges, complexity).
// For runtime metrics (superstep, completed nodes), use GetRuntimeMetrics().
func (rg *RunnableGraph[I, O]) GetMetrics() *compile.Metrics {
	return rg.compiled.GetMetrics()
}

// GetNodeDependencies returns dependency information for a specific node.
func (rg *RunnableGraph[I, O]) GetNodeDependencies(name string) (*compile.NodeDependencies, error) {
	return rg.compiled.GetNodeDependencies(name)
}

// MermaidFlowchart generates a Mermaid flowchart representation.
func (rg *RunnableGraph[I, O]) MermaidFlowchart(direction string) string {
	return rg.compiled.MermaidFlowchart(direction)
}

// GetRuntimeMetrics returns current execution metrics.
// This includes superstep number, completed/paused/active nodes, etc.
func (rg *RunnableGraph[I, O]) GetRuntimeMetrics() *RuntimeMetrics {
	return rg.runtimeMetrics
}

// Manager returns the graph's state manager for direct access to state values.
//
// Example:
//
//	value := state.GetFromManager[string](rg.Manager(), myKey)
//	state.SetInManager(ctx, rg.Manager(), myKey, "value")
//	snapshot, err := rg.Manager().Snapshot(ctx, nil)
func (rg *RunnableGraph[I, O]) Manager() *state.Manager {
	return rg.compiled.Manager
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
func (rg *RunnableGraph[I, O]) ApplyState(ctx context.Context, updates state.Updates) error {
	if err := state.ApplyUpdates(ctx, rg.compiled.Manager, updates); err != nil {
		return fmt.Errorf("failed to apply state updates: %w", err)
	}
	return nil
}

// CurrentSuperstep returns the current superstep number from runtime metrics.
// Returns 0 if execution hasn't started yet.
func (rg *RunnableGraph[I, O]) CurrentSuperstep() int64 {
	snapshot := rg.runtimeMetrics.Snapshot()
	return snapshot.CurrentSuperstep
}
