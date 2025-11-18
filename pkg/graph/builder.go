package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Builder provides a fluent API for constructing graphs.
// This is an internal type used by exec.NewBuilder().
//
// Use exec.NewBuilder to create graphs:
//
//	builder, _ := exec.NewBuilder(exec.NewPregelExecutor())
//
// Type parameters:
//   - I: Input type for the runnable
//   - O: Output type for the runnable
type Builder[I, O any] struct {
	graph *Graph
}

// BuilderOption is a functional option for configuring the Builder.
type BuilderOption[I, O any] func(*Builder[I, O]) error

// NewBuilderInternal creates a new graph builder with the given options.
// This is internal API used by exec.NewBuilder() - do not call directly.
// Use exec.NewBuilder() instead for the public API.
func NewBuilderInternal[I, O any](opts ...BuilderOption[I, O]) (*Builder[I, O], error) {
	// Create a default state manager
	manager := state.NewManager()

	graph, err := NewGraph(manager)
	if err != nil {
		return nil, err
	}

	b := &Builder[I, O]{
		graph: graph,
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

// Node adds a node to the graph with the given name and run function.
// Any errors will be caught during graph compilation in Build().
func (b *Builder[I, O]) Node(name string, runFunc func(ctx context.Context, view *state.ReadView) (*NodeResult, error)) *Builder[I, O] {
	// Errors are validated during graph compilation
	_ = b.graph.AddNode(&Node{
		Name:    name,
		RunFunc: runFunc,
	})
	return b
}

// NodeWithRetry adds a node to the graph with a retry policy.
// This is a convenience method for adding a node with automatic retry behavior.
// Any errors will be caught during graph compilation in Build().
//
// Example:
//
//	builder.NodeWithRetry("api_call", apiFunc,
//	    graph.NewRetryPolicy().
//	        WithMaxAttempts(5).
//	        WithExponentialBackoff(time.Second, 2.0).
//	        Build())
func (b *Builder[I, O]) NodeWithRetry(name string, runFunc func(ctx context.Context, view *state.ReadView) (*NodeResult, error), retryPolicy *RetryPolicy) *Builder[I, O] {
	// Errors are validated during graph compilation
	_ = b.graph.AddNode(&Node{
		Name:        name,
		RunFunc:     runFunc,
		RetryPolicy: retryPolicy,
	})
	return b
}

// SetNodeRetryPolicy sets or updates the retry policy for an existing node.
// Returns an error if the node doesn't exist.
//
// Example:
//
//	builder.Node("process", processFunc)
//	builder.SetNodeRetryPolicy("process",
//	    graph.NewRetryPolicy().WithMaxAttempts(3).Build())
func (b *Builder[I, O]) SetNodeRetryPolicy(name string, retryPolicy *RetryPolicy) error {
	node, exists := b.graph.Nodes[name]
	if !exists {
		return fmt.Errorf("node not found: %s", name)
	}
	node.RetryPolicy = retryPolicy
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
