# Graph Builder API

The Builder API provides a fluent interface for constructing computation graphs in AgentMesh.

## Features

- **Fluent API**: Chain method calls for readable graph construction
- **Executor-Based**: Configure with PregelExecutor or SequentialExecutor
- **Flexible Options**: Configure state management through builder options
- **Type-Safe**: Full Go type safety with compile-time checks

## Quick Start

### Basic Usage

```go
import (
    "context"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/state"
    "github.com/hupe1980/agentmesh/pkg/message"
)

// Create a builder with MessagePregelExecutor (most common)
builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())
if err != nil {
    log.Fatal(err)
}

// Build a workflow using command nodes
g.AddNode(&graph.BaseCommandNode{
    NodeName:        "process",
    DeclaredTargets: []string{graph.EndNode},
    Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
        return graph.End(map[string]any{"done": true}), nil
    },
})

g.SetEntryPoint("process")

// Compile and run
compiled, err := builder.Compile()
if err != nil {
    log.Fatal(err)
}

messages := []message.Message{message.NewHumanMessageFromText("Hello")}
for range compiled.Run(context.Background(), messages) {
}
```

### With Custom State Manager

```go
// Create with custom state manager
customManager := state.NewManager()
builder, err := graph.NewBuilder(
    graph.NewMessagePregelExecutor(),
    graph.WithManager[[]message.Message, message.Message](customManager),
)
```

### Conditional Routing

```go
routeKey := state.NewKey("route", "")

g.AddNode(&graph.BaseCommandNode{
    NodeName:        "router",
    DeclaredTargets: []string{"left", "right"},
    Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
        route := state.GetFromView(view, routeKey)
        if route == "left" {
            return graph.Goto(map[string]any{"route": "left"}, "left"), nil
        }
        return graph.Goto(map[string]any{"route": "right"}, "right"), nil
    },
})

g.AddNode(&graph.BaseCommandNode{
    NodeName:        "left",
    DeclaredTargets: []string{graph.EndNode},
    Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
        return graph.End(map[string]any{"result": "left"}), nil
    },
})

g.AddNode(&graph.BaseCommandNode{
    NodeName:        "right",
    DeclaredTargets: []string{graph.EndNode},
    Fn: func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
        return graph.End(map[string]any{"result": "right"}), nil
    },
})

g.SetEntryPoint("router")
```

## API Reference

### Creating a Builder

#### `graph.NewBuilder[I, O any](executor Executor[I,O], opts ...BuilderOption[I,O]) (*Builder[I, O], error)`

Creates a builder with the specified executor. This is the primary way to create graphs.

```go
// With MessagePregelExecutor (most common - for agent workflows)
builder, err := graph.NewBuilder(graph.NewMessagePregelExecutor())

// With SequentialExecutor (for debugging)
builder, err := graph.NewBuilder(graph.NewSequentialExecutor())

// With custom types
builder, err := graph.NewBuilder(graph.NewPregelExecutor[MyInput, MyOutput]())
```

### Builder Options

Options can be passed to `NewBuilder`:

- **`graph.WithManager[I, O](manager *state.Manager)`**: Use a custom state manager

### Builder Methods

All methods return `*Builder[I, O]` for method chaining:

#### `AddNode(node Node) *Builder[I, O]`
Adds a custom node implementation to the graph.

```go
customNode := &MyCustomNode{name: "custom"}
builder.AddNode(customNode)
```

#### `AddNodeFunc(name string, runFunc func(context.Context, *state.ReadView) (state.Updates, error)) *Builder[I, O]`
Adds a function-based node to the graph.

```go
builder.AddNodeFunc("process", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    return map[string]any{"done": true}, nil
})
```

#### `AddCommandNode(name string, targetSet *TargetSet, fn CommandFunc) *Builder[I, O]`
Adds a Command node to the graph (recommended for most use cases).

```go
targets := graph.NewTargetSet("success", "failure", graph.EndNode)
builder.AddCommandNode("process", targets,
    func(ctx context.Context, view *state.ReadView) (*graph.Command, error) {
        // Your logic here
        return targets.Goto("success", state.Updates{"done": true}), nil
    })
```

#### `AddCommandNodeWithRetry(name string, targetSet *TargetSet, fn CommandFunc, retryPolicy *RetryPolicy) *Builder[I, O]`
Adds a Command node with automatic retry behavior.

```go
targets := graph.NewTargetSet(graph.EndNode)
builder.AddCommandNodeWithRetry("api_call", targets, apiFunc,
    graph.NewRetryPolicy().
        WithMaxAttempts(5).
        WithExponentialBackoff(time.Second, 2.0).
        Build())
```

#### `SetEntryPoint(target string) error`
Sets the entry point of the graph (the first node to execute).

```go
g.SetEntryPoint("start_node")
```

**Note**: Graph construction now uses command-based routing. Nodes declare their targets via `DeclaredTargets` and use `graph.Goto()` or `graph.End()` commands for dynamic routing.

#### `Compile(opts ...CompileOption) (*Compiled[I, O], error)`
Compiles the graph into an executable workflow.

#### `Graph() *Graph`
Returns the underlying graph structure.

#### `Manager() *state.Manager`
Returns the graph's state manager for accessing state after execution.

## Examples

See the [builder_api example](../examples/builder_api) for a complete working example.

## Architecture

The Builder API provides a clean interface for graph construction:

- **`Builder[I, O]`**: Generic builder parameterized by input and output types
- **`Executor[I, O]`**: Handles graph execution strategy (Pregel BSP or Sequential)
- **`Compiled[I, O]`**: The result of compilation, ready for execution

The Builder handles the full lifecycle from construction to compilation to execution, with the Executor determining how nodes are executed (parallel BSP or sequential).
