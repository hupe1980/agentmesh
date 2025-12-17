---
layout: doc
title: Builder API
description: Fluent interface for constructing computation graphs.
permalink: /builder-api/
example: builder_api
hero:
  title: Graph Builder API
  description: Construct graphs with a fluent, type-safe API.
  primary_cta:
    label: Quick start
    href: "#quick-start"
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph"
    external: true
sidebar:
  - title: Features
    url: "#features"
  - title: Quick Start
    url: "#quick-start"
  - title: Adding Nodes
    url: "#adding-nodes"
  - title: Commands
    url: "#commands"
  - title: Compilation
    url: "#compilation"
---

## Features {#features}

- **Fluent API**: Chain method calls for readable graph construction
- **Type-Safe Keys**: Compile-time type safety for state access with typed keys
- **Command Pattern**: Combine state updates and routing in single expressions
- **Message-Based**: Input is `[]message.Message`, output is `message.Message`

## Quick Start {#quick-start}

### Basic Usage

```go
import (
    "context"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Define typed state keys
var (
    StatusKey = graph.NewKey[string]("status")
    CountKey  = graph.NewKey[int]("count")
)

// Create a graph with state keys
g := graph.New(StatusKey, CountKey)

// Add nodes with fluent API
g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    count := graph.Get(scope, CountKey)
    return graph.Set(StatusKey, "done").
        Set(CountKey, count+1).
        To(graph.END), nil
}, graph.END)

// Set entry point
g.Start("process")

// Compile and run
compiled, err := g.Build()
if err != nil {
    log.Fatal(err)
}

for result, err := range compiled.Run(context.Background(), "input") {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(graph.Get(result, StatusKey)) // "done"
}
```

### Message Graph for Agents

Use `message.NewGraphBuilder()` for agent workflows with built-in message handling:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
)

// Create message graph (auto-includes message.MessagesKey)
g := message.NewGraphBuilder()

// Add agent node
g.Node("agent", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    messages := message.GetMessages(scope)
    
    // Process messages with model...
    response := message.NewAIMessageFromText("Hello!")
    
    return graph.Set(message.MessagesKey, []message.Message{response}).To(graph.END), nil
}, graph.END)

g.Start("agent")

compiled, _ := g.Build()

// Run with messages
input := []message.Message{message.NewHumanMessageFromText("Hi")}
for msg, err := range compiled.Run(context.Background(), input) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(msg.Content())
}
```

### Conditional Routing

```go
var RouteKey = graph.NewKey[string]("route")

g := graph.New(RouteKey)

g.Node("router", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    route := graph.Get(scope, RouteKey)
    if route == "left" {
        return graph.To("left"), nil
    }
    return graph.To("right"), nil
}, "left", "right")

g.Node("left", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    return graph.Set(RouteKey, "went-left").To(graph.END), nil
}, graph.END)

g.Node("right", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    return graph.Set(RouteKey, "went-right").To(graph.END), nil
}, graph.END)

g.Start("router")
```

## Adding Nodes {#adding-nodes}

### Basic Nodes

Add nodes with the `Node()` method:

```go
g.Node("name", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    // Node logic here
    return graph.To("next"), nil
}, "next")
```

The last arguments are the declared targets - all possible nodes this node can route to.

### Nodes with Retry

Add automatic retry behavior with `NodeWithRetry()`:

```go
g.NodeWithRetry("api_call",
    func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
        result, err := callExternalAPI()
        if err != nil {
            return graph.Fail(err) // Will be retried
        }
        return graph.Set(ResultKey, result).To(graph.END), nil
    },
    graph.RetryPolicy{
        MaxAttempts:    5,
        InitialBackoff: time.Second,
        BackoffFactor:  2.0,
    },
    graph.END,
)
```

### Namespaced Nodes

Add namespace-scoped nodes for state isolation:

```go
var (
    AgentStatusKey = graph.NewKey("agent.status", "")
)

agentNS := graph.NewNamespace("agent")

g.Node("agent", graph.WithNamespace(
    func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
        // Can only access keys in "agent.*" namespace
        return graph.Set(AgentStatusKey, "active").To(graph.END), nil
    },
    agentNS,
    false, // includeGlobal
), graph.END)
```

### Subgraphs

Embed compiled graphs as nodes:

```go
// Create and compile subgraph
sub := graph.New(ValueKey)
sub.Node("double", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    val := graph.Get(scope, ValueKey)
    return graph.Set(ValueKey, val*2).To(graph.END), nil
}, graph.END)
sub.Start("double")
compiledSub, _ := sub.Build()

// Embed in parent
parent := graph.New(ValueKey)
parent.Subgraph("doubler", compiledSub, graph.END)
parent.Start("doubler")
```

## Commands {#commands}

Commands combine state updates with routing in a single fluent expression.

### Setting Values

```go
// Set single value
return graph.Set(StatusKey, "done").To("next"), nil

// Set multiple values
return graph.Set(StatusKey, "done").
    Set(CountKey, 42).
    Set(ValidKey, true).
    To("next"), nil
```

### Appending to Lists

```go
// Append single item
return graph.Set(TagsKey, []string{"new-tag"}).To("next"), nil

// Append multiple items
return graph.Set(MessagesKey, []Message{msg1, msg2, msg3}).To("next"), nil
```

### Routing

```go
// Route to single target
return graph.To("next"), nil

// Route to multiple targets (parallel execution)
return graph.To("worker1", "worker2", "worker3"), nil

// Route to END
return graph.To(graph.END), nil
```

### Error Handling

```go
// Signal failure
return graph.Fail(err), nil

// Conditional failure
if err != nil {
    return graph.Fail(err), nil
}
return graph.To("next"), nil
```

### Interrupts

```go
// Pause for human intervention
return graph.Set(StatusKey, "awaiting_approval").Interrupt(), nil
```

## Compilation {#compilation}

### Basic Compilation

```go
compiled, err := g.Build()
if err != nil {
    log.Fatal(err)
}
```

### With Checkpointing

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

checkpointer := checkpoint.NewInMemory()

compiled, err := g.Build(
    graph.WithCheckpointer(checkpointer),
)
```

### With Callbacks

```go
compiled, err := g.Build(
    graph.WithCallbacks(myCallbackHandler),
)
```

## Execution {#execution}

### Streaming Results

```go
for result, err := range compiled.Run(ctx, input) {
    if err != nil {
        log.Printf("Error: %v", err)
        continue
    }
    // Access state values from result view
    status := graph.Get(result, StatusKey)
    fmt.Println(status)
}
```

### With Options

```go
for result := range compiled.Run(ctx, input,
    graph.WithRunID("workflow-123"),
    graph.WithCheckpointInterval(1),
    graph.WithMaxSteps(100),
) {
    // Process results
}
```

## API Reference

### Graph Creation

| Function | Description |
|----------|-------------|
| `graph.New(keys...)` | Create graph with state keys |
| `message.NewGraphBuilder(keys...)` | Create message-based graph for agents |

### Graph Methods

| Method | Description |
|--------|-------------|
| `g.Node(name, fn, targets...)` | Add node with function and declared targets |
| `g.NodeWithRetry(name, fn, policy, targets...)` | Add node with retry policy |
| `g.NamespacedNode(name, ns, fn, targets...)` | Add namespace-scoped node |
| `g.Subgraph(name, compiled, targets...)` | Embed subgraph as node |
| `g.Start(name)` | Set entry point |
| `g.Build(opts...)` | Compile graph |

### State Keys

| Function | Description |
|----------|-------------|
| `graph.NewKey[T](name)` | Create typed single-value key |
| `graph.NewListKey[T](name)` | Create typed list key |
| `graph.Get(scope, key)` | Read value from view |
| `graph.GetList(scope, key)` | Read list from view |

### Commands

| Function | Description |
|----------|-------------|
| `graph.Set(key, val)` | Set value (reducer handles merge semantics) |
| `graph.To(targets...)` | Route to targets |
| `graph.Fail(err)` | Signal failure |
| `cmd.Interrupt()` | Pause for human intervention |

### Execution

| Function | Description |
|----------|-------------|
| `compiled.Run(ctx, input, opts...)` | Returns `iter.Seq2[ReadOnlyScope, error]` for streaming results |

## Examples

See the [builder_api example](https://github.com/hupe1980/agentmesh/tree/main/examples/builder_api) for a complete working example.

## Architecture

The Builder API provides a clean interface for graph construction:

```
graph.New(keys...)
    │
    ├── g.Node("name", fn, targets...)
    ├── g.NodeWithRetry(...)
    ├── g.NamespacedNode(...)
    ├── g.Subgraph(...)
    │
    └── g.Build(opts...)
            │
            └── *Compiled
                    │
                    └── Run(ctx, input, opts...) → iter.Seq2[ReadOnlyScope, error]
```

The graph handles the full lifecycle from construction to compilation to execution.
