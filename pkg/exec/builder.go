package exec

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Builder provides a fluent API for constructing executable graphs.
// It combines graph construction with executor configuration.
//
// Type parameters:
//   - I: Input type for the runnable
//   - O: Output type for the runnable
type Builder[I, O any] struct {
	graph    *graph.Graph
	executor Executor[I, O]
}

// BuilderOption is a functional option for configuring the Builder.
type BuilderOption[I, O any] func(*Builder[I, O]) error

// NewBuilder creates a new graph builder with the specified executor.
//
// Examples:
//
//	// Default: Pregel executor with message.Message types
//	builder, err := exec.NewBuilder(exec.NewPregelExecutor())
//
//	// Sequential executor with message.Message types
//	builder, err := exec.NewBuilder(exec.NewSequentialExecutor())
//
//	// Custom executor with custom types
//	customExecutor := exec.NewCustomPregelExecutor[MyInput, MyOutput](...)
//	builder, err := exec.NewBuilder(customExecutor)
//
// Usage:
//
//	builder.Node("process", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
//	    return &graph.NodeResult{Updates: map[string]any{"done": true}}, nil
//	})
//	builder.AddEdge(graph.StartNode, "process")
//	builder.AddEdge("process", graph.EndNode)
//	compiled, err := builder.Compile()
func NewBuilder[I, O any](executor Executor[I, O], opts ...BuilderOption[I, O]) (*Builder[I, O], error) {
	// Create a default state manager
	manager := state.NewManager()

	g, err := graph.NewGraph(manager)
	if err != nil {
		return nil, err
	}

	b := &Builder[I, O]{
		graph:    g,
		executor: executor,
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, err
		}
	}

	return b, nil
}

// WithManager sets a custom state manager for the builder.
func WithManager[I, O any](manager *state.Manager) BuilderOption[I, O] {
	return func(b *Builder[I, O]) error {
		g, err := graph.NewGraph(manager)
		if err != nil {
			return err
		}
		b.graph = g
		return nil
	}
}

// Node adds a node to the graph with the given name and run function.
func (b *Builder[I, O]) Node(name string, runFunc func(ctx context.Context, view *state.ReadView) (state.Updates, error)) *Builder[I, O] {
	_ = b.graph.AddNode(graph.NewBaseNode(name, runFunc))
	return b
}

// NodeWithRetry adds a node to the graph with a retry policy.
//
// Example:
//
//	builder.NodeWithRetry("api_call", apiFunc,
//	    graph.NewRetryPolicy().
//	        WithMaxAttempts(5).
//	        WithExponentialBackoff(time.Second, 2.0).
//	        Build())
func (b *Builder[I, O]) NodeWithRetry(name string, runFunc func(ctx context.Context, view *state.ReadView) (state.Updates, error), retryPolicy *graph.RetryPolicy) *Builder[I, O] {
	_ = b.graph.AddNode(graph.NewBaseNodeWithRetry(name, runFunc, retryPolicy))
	return b
}

// SetNodeRetryPolicy sets or updates the retry policy for an existing node.
//
// Example:
//
//	builder.Node("process", processFunc)
//	builder.SetNodeRetryPolicy("process",
//	    graph.NewRetryPolicy().WithMaxAttempts(3).Build())
func (b *Builder[I, O]) SetNodeRetryPolicy(name string, retryPolicy *graph.RetryPolicy) error {
	return b.graph.SetNodeRetryPolicy(name, retryPolicy)
}

// AddEdge adds a directed edge between two nodes.
func (b *Builder[I, O]) AddEdge(from, to string) *Builder[I, O] {
	b.graph.AddEdge(from, to)
	return b
}

// AddConditionalEdges adds conditional routing based on runtime state.
func (b *Builder[I, O]) AddConditionalEdges(from string, condition func(context.Context, *state.ReadView) []string, targets []string) *Builder[I, O] {
	b.graph.AddConditionalEdges(from, condition, targets)
	return b
}

// Graph returns the underlying graph.
func (b *Builder[I, O]) Graph() *graph.Graph {
	return b.graph
}

// Manager returns the graph's state manager.
func (b *Builder[I, O]) Manager() *state.Manager {
	return b.graph.Manager()
}

// Compile compiles the graph into a RunnableGraph using the executor.
func (b *Builder[I, O]) Compile() (*RunnableGraph[I, O], error) {
	runnable, err := CompileGraph(b.graph, b.executor)
	if err != nil {
		return nil, err
	}

	runnableGraph, ok := runnable.(*RunnableGraph[I, O])
	if !ok {
		return nil, fmt.Errorf("CompileGraph returned unexpected type")
	}

	return runnableGraph, nil
}
