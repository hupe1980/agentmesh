package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// Builder provides a fluent API for constructing graphs using the Command pattern.
//
// Use NewBuilder to create graphs with Command nodes:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	targets := graph.NewTargetSet("tool", graph.EndNode)
//	builder.AddCommandNode("model", targets,
//	    func(ctx, view) (*graph.Command, error) {
//	        // Process and return Command with routing
//	        if needsTool {
//	            return targets.Goto(targets.Get("tool"), updates), nil
//	        }
//	        return targets.Goto(targets.Get(graph.EndNode), updates), nil
//	    })
//	compiled, _ := builder.Compile()
//
// Or with static routing sugar for simple cases:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	targets := graph.NewTargetSet("next")
//	builder.AddStaticNode("process", targets, processFunc)
//	compiled, _ := builder.Compile()
//
// Or with custom node types:
//
//	builder, _ := graph.NewBuilder(graph.NewMessagePregelExecutor())
//	targets := graph.NewTargetSet(graph.EndNode)
//	customNode := &BaseCommandNode{NodeName: "custom", DeclaredTargets: targets, Fn: myFunc}
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

// AddCommandNode is THE primary way to add nodes to the graph.
// All nodes return Command - this is the unified execution model.
//
// Parameters:
//   - name: Node identifier
//   - targetSet: TargetSet defining ALL POSSIBLE routing destinations
//   - fn: CommandFunc that returns Command with updates + routing
//
// The targets are declarative - node can route to any subset at runtime,
// but must choose from this declared set. Enables:
//  1. Build-time validation (all targets must exist)
//  2. Mermaid visualization (shows all possible paths)
//  3. Type-safe compile-time routing
//
// Example:
//
//	targets := graph.NewTargetSet("tool", graph.EndNode)
//	builder.AddCommandNode("model", targets,
//	    func(ctx, view) (*graph.Command, error) {
//	        messages := view.Get("messages")
//	        response := model.Generate(ctx, messages)
//
//	        if hasToolCalls(response) {
//	            return targets.Goto(targets.Get("tool"),
//	                state.Updates{"messages": append(messages, response)},
//	            ), nil
//	        }
//
//	        return targets.Goto(targets.Get(graph.EndNode),
//	            state.Updates{"messages": append(messages, response)},
//	        ), nil
//	    },
//	)
func (b *Builder[I, O]) AddCommandNode(name string, targetSet *TargetSet, fn CommandFunc) *Builder[I, O] {
	if b.err != nil {
		return b // Short-circuit if previous error
	}
	node := &BaseCommandNode{
		NodeName:        name,
		Fn:              fn,
		DeclaredTargets: targetSet,
	}
	if err := b.graph.AddNode(node); err != nil {
		b.err = fmt.Errorf("AddCommandNode(%s): %w", name, err)
	}
	return b
}

// AddCommandNodeWithRetry adds a Command node with automatic retry on failures.
//
// Example:
//
//	targets := graph.NewTargetSet("a", "b", graph.EndNode)
//	builder.AddCommandNodeWithRetry("router", targets,
//	    func(ctx, view) (*graph.Command, error) {
//	        decision, err := unreliableService.Decide()
//	        if err != nil {
//	            return nil, err // Will be retried
//	        }
//	        return targets.Goto(targets.Get(decision), updates), nil
//	    },
//	    graph.NewRetryPolicy().WithMaxAttempts(5).Build(),
//	)
func (b *Builder[I, O]) AddCommandNodeWithRetry(name string, targetSet *TargetSet, fn CommandFunc, policy *RetryPolicy) *Builder[I, O] {
	if b.err != nil {
		return b // Short-circuit if previous error
	}
	node := &BaseCommandNode{
		NodeName:        name,
		Fn:              fn,
		DeclaredTargets: targetSet,
		Retry:           policy,
	}
	if err := b.graph.AddNode(node); err != nil {
		b.err = fmt.Errorf("AddCommandNodeWithRetry(%s): %w", name, err)
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

// AddStaticNode is syntactic sugar for simple nodes with static routing.
// For nodes with a single target: compute → go to first target → done.
//
// Example:
//
//	targets := graph.NewTargetSet("next")
//	builder.AddStaticNode("process", targets, func(ctx, view) (state.Updates, error) {
//	    result := process(view.Get("input"))
//	    return state.Updates{"output": result}, nil
//	})
//
// Equivalent to:
//
//	builder.AddCommandNode("process", targets, func(ctx, view) (*Command, error) {
//	    result := process(view.Get("input"))
//	    return targets.GotoFirst(state.Updates{"output": result}), nil
//	})
func (b *Builder[I, O]) AddStaticNode(name string, targetSet *TargetSet, compute func(context.Context, *state.ReadView) (state.Updates, error)) *Builder[I, O] {
	// Wrap simple compute function as CommandFunc
	fn := func(ctx context.Context, view *state.ReadView) (*Command, error) {
		updates, err := compute(ctx, view)
		if err != nil {
			return nil, err
		}
		return targetSet.GotoFirst(updates), nil
	}

	return b.AddCommandNode(name, targetSet, fn)
}

// AddStaticNodeWithRetry adds a static node with automatic retry on failures.
// For simple nodes with a single target that need retry logic.
//
// Example:
//
//	targets := graph.NewTargetSet("next")
//	builder.AddStaticNodeWithRetry("api_call", targets,
//	    func(ctx, view) (state.Updates, error) {
//	        result, err := unreliableAPI.Call()
//	        if err != nil {
//	            return nil, err // Will be retried
//	        }
//	        return state.Updates{"result": result}, nil
//	    },
//	    graph.NewRetryPolicy().WithMaxAttempts(3).Build(),
//	)
func (b *Builder[I, O]) AddStaticNodeWithRetry(name string, targetSet *TargetSet, compute func(context.Context, *state.ReadView) (state.Updates, error), policy *RetryPolicy) *Builder[I, O] {
	// Wrap simple compute function as CommandFunc
	fn := func(ctx context.Context, view *state.ReadView) (*Command, error) {
		updates, err := compute(ctx, view)
		if err != nil {
			return nil, err
		}
		return targetSet.GotoFirst(updates), nil
	}

	return b.AddCommandNodeWithRetry(name, targetSet, fn, policy)
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
