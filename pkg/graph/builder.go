package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// ErrNoCompileFunc is returned when trying to compile a builder without a compile function.
var ErrNoCompileFunc = fmt.Errorf("no compile function registered; use WithCompileFunc option or SetCompileFunc method")

// Builder provides a fluent API for constructing graphs.
//
// DEPRECATED: Use exec.NewBuilder instead, which automatically configures
// compilation without requiring WithCompileFunc. This builder remains for
// backward compatibility but will be removed in a future version.
//
// Recommended migration:
//
//	// Old way (requires WithCompileFunc):
//	builder, _ := graph.NewBuilder[I, O](graph.WithCompileFunc(compileFunc))
//
//	// New way (automatic compilation):
//	builder, _ := exec.NewBuilder(exec.NewPregelExecutor())
//
// Type parameters:
//   - I: Input type for the runnable
//   - O: Output type for the runnable
type Builder[I, O any] struct {
	graph       *Graph
	compileFunc func(*Graph) (Runnable[I, O], error)
}

// BuilderOption is a functional option for configuring the Builder.
type BuilderOption[I, O any] func(*Builder[I, O]) error

// NewBuilder creates a new graph builder with the given options.
//
// DEPRECATED: Use exec.NewBuilder instead for a cleaner API that doesn't
// require WithCompileFunc. See exec.NewBuilder for examples.
func NewBuilder[I, O any](opts ...BuilderOption[I, O]) (*Builder[I, O], error) {
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

// WithCompileFunc sets a custom compile function for the builder.
// This is used to avoid import cycles with the exec package.
//
// Example:
//
//	builder := graph.NewBuilder(graph.WithCompileFunc(compileFunc))
func WithCompileFunc[I, O any](compileFunc func(*Graph) (Runnable[I, O], error)) BuilderOption[I, O] {
	return func(b *Builder[I, O]) error {
		b.compileFunc = compileFunc
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

// Compile compiles the graph into a Runnable using the registered compile function.
// If no compile function is registered, returns an error.
// To set a compile function, use WithCompileFunc option or call SetCompileFunc.
//
// Example:
//
//	import "github.com/hupe1980/agentmesh/pkg/exec"
//	builder.SetCompileFunc(compileFunc)
//	compiled, err := builder.Compile()
func (b *Builder[I, O]) Compile() (Runnable[I, O], error) {
	if b.compileFunc == nil {
		return nil, ErrNoCompileFunc
	}
	return b.compileFunc(b.graph)
}

// SetCompileFunc sets the compile function after builder creation.
func (b *Builder[I, O]) SetCompileFunc(compileFunc func(*Graph) (Runnable[I, O], error)) {
	b.compileFunc = compileFunc
}

// Build returns the underlying graph without compiling.
func (b *Builder[I, O]) Build() *Graph {
	return b.graph
}
