package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Builder provides a fluent API for constructing graphs.
//
// Use NewBuilder to create graphs with inline functions:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	builder.AddNodeFunc("process", processFunc)
//	builder.AddEdge(graph.StartNode, "process")
//	builder.AddEdge("process", graph.EndNode)
//	compiled, _ := builder.Compile()
//
// Or with custom node types:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	customNode := &MyNode{name: "custom"}
//	builder.AddNode(customNode)
//	builder.AddEdge(graph.StartNode, "custom")
//	builder.AddEdge("custom", graph.EndNode)
//	compiled, _ := builder.Compile()
//
// Type parameters:
//   - I: Input type for the compiled graph
//   - O: Output type for the compiled graph
type Builder[I, O any] struct {
	graph    *Graph
	executor Executor[I, O]
}

// BuilderOption is a functional option for configuring the Builder.
type BuilderOption[I, O any] func(*Builder[I, O]) error

// NewBuilder creates a new graph builder with the specified executor.
//
// Examples:
//
//	// Default: Pregel executor with message.Message types
//	builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
//
//	// Sequential executor with message.Message types
//	builder, err := graph.NewBuilder(graph.NewSequentialExecutor())
//
//	// Custom executor with custom types
//	customExecutor := graph.NewPregelExecutor[MyInput, MyOutput](...)
//	builder, err := graph.NewBuilder(customExecutor)
func NewBuilder[I, O any](executor Executor[I, O], opts ...BuilderOption[I, O]) (*Builder[I, O], error) {
	// Create a default state manager
	manager := state.NewManager()

	graph, err := NewGraph(manager)
	if err != nil {
		return nil, err
	}

	b := &Builder[I, O]{
		graph:    graph,
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
		graph, err := NewGraph(manager)
		if err != nil {
			return err
		}
		b.graph = graph
		return nil
	}
}

// AddNode adds a node to the graph.
// Any errors will be caught during graph compilation in Compile().
//
// Example:
//
//	customNode := &MyNode{name: "custom"}
//	builder.AddNode(customNode)
func (b *Builder[I, O]) AddNode(node Node) *Builder[I, O] {
	_ = b.graph.AddNode(node)
	return b
}

// AddNodeFunc adds a function-based node to the graph with the given name.
// Any errors will be caught during graph compilation in Compile().
//
// Example:
//
//	builder.AddNodeFunc("process", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
//	    // Process logic here
//	    return state.NoUpdate(), nil
//	})
func (b *Builder[I, O]) AddNodeFunc(name string, runFunc func(ctx context.Context, view *state.ReadView) (state.Updates, error)) *Builder[I, O] {
	_ = b.graph.AddNode(NewBaseNode(name, runFunc))
	return b
}

// AddNodeFuncWithRetry adds a function-based node to the graph with a retry policy.
// This is a convenience method for adding a node with automatic retry behavior.
// Any errors will be caught during graph compilation in Compile().
//
// Example:
//
//	builder.AddNodeFuncWithRetry("api_call", apiFunc,
//	    graph.NewRetryPolicy().
//	        WithMaxAttempts(5).
//	        WithExponentialBackoff(time.Second, 2.0).
//	        Build())
func (b *Builder[I, O]) AddNodeFuncWithRetry(name string, runFunc func(ctx context.Context, view *state.ReadView) (state.Updates, error), retryPolicy *RetryPolicy) *Builder[I, O] {
	_ = b.graph.AddNode(NewBaseNodeWithRetry(name, runFunc, retryPolicy))
	return b
}

// SetNodeRetryPolicy sets or updates the retry policy for an existing node.
// Returns an error if the node doesn't exist or doesn't support retry.
//
// Example:
//
//	builder.AddNodeFunc("process", processFunc)
//	builder.SetNodeRetryPolicy("process",
//	    graph.NewRetryPolicy().WithMaxAttempts(3).Build())
func (b *Builder[I, O]) SetNodeRetryPolicy(name string, retryPolicy *RetryPolicy) error {
	node, exists := b.graph.Nodes[name]
	if !exists {
		return fmt.Errorf("node not found: %s", name)
	}

	// Only BaseNode supports setting retry policy after creation
	baseNode, ok := node.(*BaseNode)
	if !ok {
		return fmt.Errorf("node %s does not support setting retry policy", name)
	}

	baseNode.retryPolicy = retryPolicy
	return nil
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
func (b *Builder[I, O]) Graph() *Graph {
	return b.graph
}

// Manager returns the graph's state manager.
func (b *Builder[I, O]) Manager() *state.Manager {
	return b.graph.Manager()
}

// Compile compiles the graph into a Compiled[I,O] using the executor.
// Returns an error if the graph is invalid or compilation fails.
//
// Example:
//
//	compiled, err := builder.Compile()
//	if err != nil {
//	    return fmt.Errorf("compilation failed: %w", err)
//	}
//
//	// Or with options:
//	compiled, err := builder.Compile(graph.WithStrictValidation())
func (b *Builder[I, O]) Compile(opts ...CompileOption) (*Compiled[I, O], error) {
	if b.executor == nil {
		return nil, fmt.Errorf("executor not set - use NewBuilder with an executor")
	}
	return Compile(b.graph, b.executor, opts...)
}
