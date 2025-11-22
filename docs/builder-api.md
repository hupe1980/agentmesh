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

// Build a workflow using fluent API
builder.
    AddNodeFunc("process", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
        return map[string]any{"done": true}, nil
    }).
    AddEdge(graph.StartNode, "process").
    AddEdge("process", graph.EndNode)

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

### Conditional Edges

```go
routeKey := state.NewKey("route", "")

builder.
    AddNodeFunc("router", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
        return map[string]any{"route": "left"}, nil
    }).
    AddNodeFunc("left", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
        return map[string]any{"result": "left"}, nil
    }).
    AddNodeFunc("right", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
        return map[string]any{"result": "right"}, nil
    }).
    AddEdge(graph.StartNode, "router").
    AddConditionalEdges("router", func(ctx context.Context, view *state.ReadView) []string {
        route := state.GetFromView(view, routeKey)
        if route == "left" {
            return []string{"left"}
        }
        return []string{"right"}
    }, []string{"left", "right"}).
    AddEdge("left", graph.EndNode).
    AddEdge("right", graph.EndNode)
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

#### `AddNodeFuncWithRetry(name string, runFunc func(context.Context, *state.ReadView) (state.Updates, error), retryPolicy *RetryPolicy) *Builder[I, O]`
Adds a function-based node with automatic retry behavior.

```go
builder.AddNodeFuncWithRetry("api_call", apiFunc,
    graph.NewRetryPolicy().
        WithMaxAttempts(5).
        WithExponentialBackoff(time.Second, 2.0).
        Build())
```

#### `AddEdge(from, to string) *Builder[I, O]`
Adds a directed edge between two nodes.

#### `AddConditionalEdges(from string, condition func(context.Context, *state.ReadView) []string, targets []string) *Builder[I, O]`
Adds conditional routing based on runtime state.

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
