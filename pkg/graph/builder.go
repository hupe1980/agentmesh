package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Builder provides a fluent API for constructing graphs with tuple-based nodes.
//
// Use NewBuilder to create graphs with NodeFunc:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	builder.AddNodeFunc("model", []string{"tool", graph.END},
//	    func(ctx, view) ([]string, state.Updates, error) {
//	        // Process and return tuple (targets, updates, error)
//	        if needsTool {
//	            return []string{"tool"}, updates, nil
//	        }
//	        return []string{graph.END}, updates, nil
//	    })
//	compiled, _ := builder.Compile()
//
// Or with static routing sugar for simple cases:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	builder.AddStaticNode("process", []string{"next"}, processFunc)
//	compiled, _ := builder.Compile()
//
// Or with custom node types:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	customNode := &BaseNode{NodeName: "custom", DeclaredTargets: []string{graph.END}, Fn: myFunc}
//	builder.AddNode(customNode)
//	compiled, _ := builder.Compile()
//
// Type parameters:
//   - I: Input type for the compiled graph
//   - O: Output type for the compiled graph
type Builder[I, O any] struct {
	graph    *Graph
	executor Executor[I, O]
	err      error // Accumulated error from builder operations
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
//
//	// With custom state manager
//	customManager := state.NewManager(state.WithCheckpointer(cp, "run-123"))
//	builder, err := graph.NewBuilder(executor, graph.WithManager[In, Out](customManager))
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
//
// Example:
//
//	manager := state.NewManager(
//	    state.WithCheckpointer(checkpointer, "run-123"),
//	    state.WithMaxSnapshotsLimit(100),
//	)
//	builder, err := graph.NewBuilder(executor, graph.WithManager[In, Out](manager))
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

// WithInterruptBefore sets nodes to interrupt before execution.
//
// Example:
//
//	builder, err := graph.NewBuilder(executor,
//	    graph.WithInterruptBefore[In, Out]("human_approval", "critical_step"),
//	)
func WithInterruptBefore[I, O any](nodes ...string) BuilderOption[I, O] {
	return func(b *Builder[I, O]) error {
		b.graph.InterruptBefore = append(b.graph.InterruptBefore, nodes...)
		return nil
	}
}

// WithInterruptAfter sets nodes to interrupt after execution.
//
// Example:
//
//	builder, err := graph.NewBuilder(executor,
//	    graph.WithInterruptAfter[In, Out]("model", "tool"),
//	)
func WithInterruptAfter[I, O any](nodes ...string) BuilderOption[I, O] {
	return func(b *Builder[I, O]) error {
		b.graph.InterruptAfter = append(b.graph.InterruptAfter, nodes...)
		return nil
	}
}

// AddNode adds a custom node implementation to the graph.
// Errors are accumulated and returned during Compile().
//
// Example:
//
//	customNode := &MyNode{name: "custom"}
//	builder.AddNode(customNode)
func (b *Builder[I, O]) AddNode(node Node) *Builder[I, O] {
	if b.err != nil {
		return b // Short-circuit if previous error
	}
	if err := b.graph.AddNode(node); err != nil {
		b.err = fmt.Errorf("AddNode(%s): %w", node.Name(), err)
	}
	return b
}

// AddNodeFunc adds a node with the simplified tuple-based API.
// This is THE primary way to add nodes to the graph.
//
// Parameters:
//   - name: Node identifier
//   - targets: ALL POSSIBLE routing destinations (e.g., []string{"tool", graph.END})
//   - fn: NodeFunc that returns (targets, updates, error) tuple
//
// The targets are declarative - node can route to any subset at runtime,
// but must choose from this declared set. Enables:
//  1. Build-time validation (all targets must exist)
//  2. Mermaid visualization (shows all possible paths)
//  3. Simple, idiomatic Go API
//
// Example:
//
//	builder.AddNodeFunc("model", []string{"tool", graph.END},
//	    func(ctx, view) ([]string, state.Updates, error) {
//	        messages := view.Get("messages")
//	        response := model.Generate(ctx, messages)
//
//	        if hasToolCalls(response) {
//	            return []string{"tool"}, state.Updates{
//	                "messages": append(messages, response),
//	            }, nil
//	        }
//
//	        return []string{graph.END}, state.Updates{
//	            "messages": append(messages, response),
//	        }, nil
//	    },
//	)
func (b *Builder[I, O]) AddNodeFunc(name string, targets []string, fn NodeFunc) *Builder[I, O] {
	if b.err != nil {
		return b // Short-circuit if previous error
	}
	node := &BaseNode{
		NodeName:        name,
		Fn:              fn,
		DeclaredTargets: targets,
	}
	if err := b.graph.AddNode(node); err != nil {
		b.err = fmt.Errorf("AddNodeFunc(%s): %w", name, err)
	}
	return b
}

// AddNodeFuncWithRetry adds a node with automatic retry on failures.
//
// Example:
//
//	builder.AddNodeFuncWithRetry("router", []string{"a", "b", graph.END},
//	    func(ctx, view) ([]string, state.Updates, error) {
//	        decision, err := unreliableService.Decide()
//	        if err != nil {
//	            return nil, nil, err // Will be retried
//	        }
//	        return []string{decision}, updates, nil
//	    },
//	    graph.NewRetryPolicy().WithMaxAttempts(5).Build(),
//	)
func (b *Builder[I, O]) AddNodeFuncWithRetry(name string, targets []string, fn NodeFunc, policy *RetryPolicy) *Builder[I, O] {
	if b.err != nil {
		return b // Short-circuit if previous error
	}
	node := &BaseNode{
		NodeName:        name,
		Fn:              fn,
		DeclaredTargets: targets,
		Retry:           policy,
	}
	if err := b.graph.AddNode(node); err != nil {
		b.err = fmt.Errorf("AddNodeFuncWithRetry(%s): %w", name, err)
	}
	return b
}

// SetEntryPoint declares which node(s) should execute first.
// The entry point nodes will be automatically connected from the start node.
//
// Example:
//
//	builder.SetEntryPoint("input_handler")
//	// or multiple entry points for parallel start
//	builder.SetEntryPoint("worker1", "worker2", "worker3")
func (b *Builder[I, O]) SetEntryPoint(targets ...string) *Builder[I, O] {
	if b.err != nil {
		return b // Short-circuit if previous error
	}
	// Add edges from START to entry point nodes
	for _, target := range targets {
		if err := b.graph.SetEntryPoint(target); err != nil {
			b.err = fmt.Errorf("SetEntryPoint(%s): %w", target, err)
			return b
		}
	}
	return b
}

// AddStaticNode is syntactic sugar for simple nodes with static routing to the first target.
// For nodes that always route to the same place: compute → go to first target → done.
//
// Example:
//
//	builder.AddStaticNode("process", []string{"next"},
//	    func(ctx, view) (state.Updates, error) {
//	        result := process(view.Get("input"))
//	        return state.Updates{"output": result}, nil
//	    })
//
// Equivalent to:
//
//	builder.AddNodeFunc("process", []string{"next"},
//	    func(ctx, view) ([]string, state.Updates, error) {
//	        result := process(view.Get("input"))
//	        return []string{"next"}, state.Updates{"output": result}, nil
//	    })
func (b *Builder[I, O]) AddStaticNode(name string, targets []string, compute func(context.Context, state.ReadView) (state.Updates, error)) *Builder[I, O] {
	if len(targets) == 0 {
		b.err = fmt.Errorf("AddStaticNode(%s): targets cannot be empty", name)
		return b
	}
	// Wrap simple compute function as NodeFunc
	fn := func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
		updates, err := compute(ctx, view)
		if err != nil {
			return nil, nil, err
		}
		return []string{targets[0]}, updates, nil // Always route to first target
	}

	return b.AddNodeFunc(name, targets, fn)
}

// AddStaticNodeWithRetry adds a static node with automatic retry on failures.
// For simple nodes with static routing to the first target that need retry logic.
//
// Example:
//
//	builder.AddStaticNodeWithRetry("api_call", []string{"next"},
//	    func(ctx, view) (state.Updates, error) {
//	        result, err := unreliableAPI.Call()
//	        if err != nil {
//	            return nil, err // Will be retried
//	        }
//	        return state.Updates{"result": result}, nil
//	    },
//	    graph.NewRetryPolicy().WithMaxAttempts(3).Build(),
//	)
func (b *Builder[I, O]) AddStaticNodeWithRetry(name string, targets []string, compute func(context.Context, state.ReadView) (state.Updates, error), policy *RetryPolicy) *Builder[I, O] {
	if len(targets) == 0 {
		b.err = fmt.Errorf("AddStaticNodeWithRetry(%s): targets cannot be empty", name)
		return b
	}
	// Wrap simple compute function as NodeFunc
	fn := func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
		updates, err := compute(ctx, view)
		if err != nil {
			return nil, nil, err
		}
		return []string{targets[0]}, updates, nil // Always route to first target
	}

	return b.AddNodeFuncWithRetry(name, targets, fn, policy)
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
// Returns any accumulated builder errors or compilation errors.
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
	// Return accumulated builder errors first
	if b.err != nil {
		return nil, fmt.Errorf("builder error: %w", b.err)
	}
	if b.executor == nil {
		return nil, fmt.Errorf("executor not set - use NewBuilder with an executor")
	}
	return Compile(b.graph, b.executor, opts...)
}
