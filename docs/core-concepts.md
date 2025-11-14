---
layout: doc
title: Core Concepts
description: Understand graphs, nodes, state, and execution flow in AgentMesh.
permalink: /core-concepts/
hero:
  title: Master the fundamentals
  description: Learn how graphs, nodes, and state work together to create powerful agent workflows.
  primary_cta:
    label: Build your first graph
    href: "#building-graphs"
  secondary_cta:
    label: Architecture deep dive →
    href: "/architecture/"
sidebar:
  - title: Graphs and nodes
    url: "#graphs-and-nodes"
  - title: State management
    url: "#state-management"
  - title: Execution flow
    url: "#execution-flow"
  - title: Messages
    url: "#messages"
  - title: Channels
    url: "#channels"
---

## Graphs and nodes {#graphs-and-nodes}

AgentMesh uses a **directed graph** model where computation flows through connected nodes.

### What is a graph?

A graph consists of:
- **Nodes** - Computational units that process data
- **Edges** - Connections that define execution order
- **State** - Shared context accessible across all nodes

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

builder, err := graph.NewBuilder()
if err != nil {
    return err
}

// Add nodes
builder.Node("fetch", fetchDataFunc)
builder.Node("process", processDataFunc)
builder.Node("save", saveDataFunc)

// Define flow with edges
builder.AddEdge("START", "fetch")
builder.AddEdge("fetch", "process")
builder.AddEdge("process", "save")
builder.AddEdge("save", "END")

// Compile into executable graph
compiled, err := builder.Compile()
if err != nil {
    return err
}
```

### Node functions

Nodes are functions that receive state and return updates:

```go
func processDataFunc(ctx context.Context, state state.Reader) (*graph.NodeResult, error) {
    // Read from state
    data := state.Get("raw_data")
    messages := state.MessagesSnapshot()
    
    // Process...
    processed := transform(data)
    
    // Return updates
    return &graph.NodeResult{
        Updates: map[string]any{
            "processed_data": processed,
            "status": "complete",
        },
        Messages: []message.Message{
            message.NewAIMessageFromText("Processing complete"),
        },
    }, nil
}
```

### Special nodes

- **START** - Entry point (automatically created)
- **END** - Terminal node (marks completion)

### Conditional routing

Dynamically route to different nodes based on results:

```go
builder.AddConditionalEdges("classifier", func(result *graph.NodeResult) []string {
    category := result.Updates["category"].(string)
    switch category {
    case "urgent":
        return []string{"urgent_handler"}
    case "normal":
        return []string{"normal_handler"}
    default:
        return []string{"default_handler"}
    }
})
```

---

## State management {#state-management}

State is shared across all nodes and flows through the graph using **channels**.

### Reading state

Nodes receive immutable state snapshots:

```go
func myNode(ctx context.Context, state state.Reader) (*graph.NodeResult, error) {
    // Read values
    counter := state.Get("counter").(int)
    status := state.Get("status").(string)
    messages := state.MessagesSnapshot()
    
    // Access full state snapshot
    snapshot := state.Snapshot()
    
    return &graph.NodeResult{}, nil
}
```

### Updating state

Nodes update state through `NodeResult`:

```go
return &graph.NodeResult{
    Updates: map[string]any{
        "counter": counter + 1,
        "status": "processing",
        "result": computedValue,
    },
    Messages: []message.Message{
        message.NewAIMessageFromText("Updated successfully"),
    },
}, nil
```

### State builder pattern

Use the fluent StateBuilder API for simpler initialization:

```go
stateBuilder := graph.NewStateBuilder().
    WithUnlimitedMessages().
    WithLastValueChannel("status").
    WithLastValueChannel("counter").
    WithInitialMessages(
        message.NewSystemMessageFromText("You are a helpful assistant"),
    )

compiled, err := builder.Compile(
    graph.WithStateBuilder(stateBuilder),
)
```

See `examples/state_builder` for detailed usage.

---

## Execution flow {#execution-flow}

AgentMesh executes graphs using **Pregel-style bulk synchronous parallel (BSP)** processing.

### Supersteps

Execution proceeds in discrete **supersteps**:

1. **Identify ready nodes** - Nodes with satisfied dependencies
2. **Execute in parallel** - Ready nodes run concurrently
3. **Apply updates** - State changes applied atomically
4. **Repeat** - Until END node or max iterations

```
Superstep 0: [START]
Superstep 1: [node_a, node_b]  ← Parallel execution
Superstep 2: [node_c]
Superstep 3: [END]
```

### Parallel execution

Nodes with the same dependencies execute concurrently:

```go
// These three nodes execute in parallel
builder.AddEdge("START", "fetch_a")
builder.AddEdge("START", "fetch_b")
builder.AddEdge("START", "fetch_c")

// All converge to aggregator
builder.AddEdge("fetch_a", "aggregator")
builder.AddEdge("fetch_b", "aggregator")
builder.AddEdge("fetch_c", "aggregator")
```

### Cycles and loops

Unlike DAG-based systems, AgentMesh supports **cycles** for iterative workflows:

```go
builder.Node("writer", func(ctx context.Context, state state.Reader) (*graph.NodeResult, error) {
    draft := generateDraft()
    return &graph.NodeResult{
        Updates: map[string]any{"draft": draft},
        NextNodes: []string{"evaluator"},
    }, nil
})

builder.Node("evaluator", func(ctx context.Context, state state.Reader) (*graph.NodeResult, error) {
    draft := state.Get("draft")
    if isGoodEnough(draft) {
        return &graph.NodeResult{NextNodes: []string{"END"}}, nil
    }
    // Loop back to writer for refinement
    return &graph.NodeResult{
        Updates: map[string]any{"feedback": "improve clarity"},
        NextNodes: []string{"writer"},
    }, nil
})
```

### Max iterations

Prevent infinite loops:

```go
compiled, err := builder.Compile(
    graph.WithMaxIterations(10),
)
```

---

## Messages {#messages}

Messages represent conversation turns between users, AI, and tools.

### Message types

```go
import "github.com/hupe1980/agentmesh/pkg/message"

// Human input
humanMsg := message.NewHumanMessageFromText("What's the weather?")

// AI response
aiMsg := message.NewAIMessageFromText("It's sunny and 72°F")

// System prompt
systemMsg := message.NewSystemMessageFromText("You are a helpful assistant")

// Tool call
toolCall := message.ToolCall{
    ID:   "call_123",
    Name: "get_weather",
    Arguments: map[string]any{"location": "Paris"},
}
aiWithTool := message.NewAIMessage(message.NewTextPart("Let me check"), toolCall)

// Tool result
toolMsg := message.NewToolMessage("call_123", "Sunny, 22°C")
```

### Message parts

Messages can contain multiple parts:

```go
aiMsg := message.NewAIMessage(
    message.NewTextPart("Here's the weather"),
    message.NewImagePart(imageURL),
)

// Access parts
for _, part := range aiMsg.Parts() {
    switch p := part.(type) {
    case message.TextPart:
        fmt.Println("Text:", p.Text)
    case message.ImagePart:
        fmt.Println("Image:", p.URL)
    }
}
```

---

## Channels {#channels}

Channels control how state updates are applied.

### Channel Interface Design

AgentMesh uses a three-tier interface hierarchy for channels:

1. **`channel.Channel`** - User-facing interface with safe operations:
   - `Name()` - Get channel identifier
   - `Read(ctx)` - Read current value
   - `Write(ctx, value)` - Write using channel-specific semantics

2. **`channel.VersionedChannel`** - Internal runtime operations (extends Channel):
   - `Version()` - Cache invalidation tracking
   - `Snapshot(ctx)` - Point-in-time state capture
   - `Clone()` - Deep copy for checkpointing

3. **`channel.ResettableChannel`** - Admin operations (extends Channel):
   - `Reset(ctx)` - **Dangerous**: Clear state (use only between graph runs)

**For users**: Interact only with the base `Channel` interface (Read/Write). The runtime handles internal operations automatically.

### TopicChannel

Accumulates values in a list (append-only):

```go
state := graph.NewStateManager(100)
state.AddChannel(channel.NewTopicChannel("messages", 100))

// Updates append to the list
result := &graph.NodeResult{
    Messages: []message.Message{newMessage}, // Appends
}
```

**Use cases**: Conversation history, event logs, audit trails

### LastValueChannel

Stores only the most recent value (overwrite):

```go
state.AddChannel(channel.NewLastValueChannel("status"))

// Updates overwrite previous value
result := &graph.NodeResult{
    Updates: map[string]any{
        "status": "complete", // Overwrites previous status
    },
}
```

**Use cases**: Current state, flags, counters

### BinaryOpChannel

Merges values using custom operators:

```go
// Sum channel for counters
state.AddChannel(channel.NewBinaryOpChannel("total", func(a, b any) any {
    return a.(int) + b.(int)
}))

// Max channel for tracking peaks
state.AddChannel(channel.NewBinaryOpChannel("max_value", func(a, b any) any {
    if a.(int) > b.(int) {
        return a
    }
    return b
}))

// Updates are combined
result := &graph.NodeResult{
    Updates: map[string]any{
        "total": 10,      // Will be summed with existing value
        "max_value": 100, // Will keep maximum value
    },
}
```

**Use cases**: Aggregations, statistics, accumulations

---

## Next steps

- **[Agents](/agents/)** - Build ReAct, Supervisor, and RAG agents
- **[Tools](/tools/)** - Create function tools for agent capabilities
- **[Checkpointing](/checkpointing/)** - State persistence and time travel debugging
- **[Streaming](/streaming/)** - Real-time execution events
- **[Callbacks](/callbacks/)** - Intercept model and tool calls
- **[Observability](/observability/)** - OpenTelemetry metrics and tracing
- **[Architecture](/architecture/)** - Understand Pregel BSP internals
