# Graph Builder API

The Builder API provides a fluent interface for constructing computation graphs in AgentMesh. It wraps the lower-level graph construction API with a more convenient, chainable interface.

## Features

- **Fluent API**: Chain method calls for readable graph construction
- **Automatic Compilation**: Use `exec.NewBuilder()` for built-in compilation support
- **Flexible Options**: Configure state management, history size, and more
- **Type-Safe**: Full Go type safety with compile-time checks

## Quick Start

### Basic Usage

```go
import (
    "context"
    "github.com/hupe1980/agentmesh/pkg/exec"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/state"
)

// Create a builder with automatic compilation
builder, err := exec.NewBuilder()
if err != nil {
    log.Fatal(err)
}

// Build a workflow using fluent API
builder.
    Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
        return &graph.NodeResult{
            Updates: map[string]any{"done": true},
        }, nil
    }).
    AddEdge(graph.StartNode, "process").
    AddEdge("process", graph.EndNode)

// Compile and run
compiled, err := builder.Compile()
if err != nil {
    log.Fatal(err)
}

for range compiled.Run(context.Background(), messages) {
}
```

### With Custom Options

```go
// Create with custom state manager
stateManager, _ := state.NewStateManager(100) // 100 history size
builder, err := exec.NewBuilder(graph.WithStateManager(stateManager))

// Or use WithMaxHistorySize helper
builder, err := exec.NewBuilder(graph.WithMaxHistorySize(100))
```

### Conditional Edges

```go
builder.
    Node("router", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
        return &graph.NodeResult{
            Updates: map[string]any{"route": "left"},
        }, nil
    }).
    Node("left", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
        return &graph.NodeResult{Updates: map[string]any{"result": "left"}}, nil
    }).
    Node("right", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
        return &graph.NodeResult{Updates: map[string]any{"result": "right"}}, nil
    }).
    AddEdge(graph.StartNode, "router").
    AddConditionalEdges("router", func(ctx context.Context, view *state.ReadView) []string {
        route := state.GetFromView(view, RouteKey)
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

#### `exec.NewBuilder(opts ...graph.BuilderOption) (*graph.Builder, error)`
Creates a builder with `CompileGraph` pre-configured. This is the recommended way to create a builder.

#### `graph.NewBuilder(opts ...graph.BuilderOption) (*graph.Builder, error)`
Creates a builder without automatic compilation support. You must call `exec.CompileGraph()` manually.

### Builder Options

- **`graph.WithStateManager(sm state.StateManager)`**: Use a custom state manager
- **`graph.WithMaxHistorySize(maxSize int)`**: Set maximum history size for state
- **`graph.WithCompileFunc(func(*graph.Graph) (graph.MessageRunnable, error))`**: Set custom compile function

### Builder Methods

All methods return `*Builder` for method chaining:

#### `Node(name string, runFunc func(context.Context, state.Writer) (*graph.NodeResult, error)) *Builder`
Adds a node to the graph.

#### `AddEdge(from, to string) *Builder`
Adds a directed edge between two nodes.

#### `AddConditionalEdges(from string, condition func(context.Context, state.Reader) []string, targets []string) *Builder`
Adds conditional routing based on runtime state.

#### `Compile() (graph.MessageRunnable, error)`
Compiles the graph into a runnable. Only available if created with `exec.NewBuilder()`.

#### `Compile() (graph.MessageRunnable, error)`
Alias for `Compile()` for API compatibility.

#### `Build() *Graph`
Returns the underlying graph without compiling.

#### `Graph() *Graph`
Returns the underlying graph (alias for `Build()`).

#### `StateManager() state.StateManager`
Returns the graph's state manager for accessing state after execution.

## Migration from Old Builder API

If you have code using the old Builder API:

```go
// Old API
builder, _ := graph.NewBuilder()
builder.Node("process", processFunc)
builder.AddEdge(graph.StartNode, "process")
compiled, _ := builder.Compile()
```

Simply change `graph.NewBuilder()` to `exec.NewBuilder()`:

```go
// New API
builder, _ := exec.NewBuilder()
builder.Node("process", processFunc)
builder.AddEdge(graph.StartNode, "process")
compiled, _ := builder.Compile()  // or Compile()
```

## Examples

See the [builder_api example](../examples/builder_api) for a complete working example.

## Architecture

The Builder API follows Phase 2 architecture:
- `pkg/graph` - Graph structure and Builder
- `pkg/compile` - Topology compilation
- `pkg/exec` - Execution and integration

To avoid import cycles, `exec.NewBuilder()` provides the integration point where the Builder is configured with `exec.CompileGraph()`.
