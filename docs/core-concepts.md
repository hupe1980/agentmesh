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
  - title: Runnable interface
    url: "#runnable-interface"
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

> **Type Safety with Generics (Go 1.24+):** AgentMesh now provides full compile-time type safety through generic compilation. Use `builder.Compile()` for the common case, or `graph.Compile[I, O](builder)` for custom input/output types. See [GENERICS.md](https://github.com/hupe1980/agentmesh/blob/main/GENERICS.md) for details.

## Runnable interface {#runnable-interface}

The `Runnable[I, O]` interface is the core abstraction for executable components in AgentMesh. All agents, graphs, and tools implement this interface, enabling type-safe composition and testability.

### Interface definition

```go
type Runnable[I, O any] interface {
    Run(ctx context.Context, input I, opts ...RunOption) iter.Seq2[O, error]
}
```

**Type parameters:**
- `I` - Input type (e.g., `[]message.Message`, `map[string]any`, `string`)
- `O` - Output type (e.g., `state.ExecutionResult`, `message.Message`)

### Common type aliases

For convenience, AgentMesh provides type aliases for common use cases:

```go
// MessageRunnable processes message sequences (most common)
type MessageRunnable = Runnable[[]message.Message, state.ExecutionResult]

// StateRunnable processes arbitrary state maps
type StateRunnable = Runnable[map[string]any, state.ExecutionResult]

// StringRunnable processes text input/output
type StringRunnable = Runnable[string, string]
```

### Usage example

All agent constructors return `MessageRunnable`:

```go
// Agent constructors return MessageRunnable interface
var agent graph.MessageRunnable
agent, err := agent.NewReActAgent(model, agent.WithTools(tools...))

// Execute with type-safe interface
for result, err := range agent.Run(ctx, messages) {
    if err != nil {
        return err
    }
    // Process result
}
```

### Benefits

**Compile-time type safety:**
```go
// ✅ Type-safe: MessageRunnable accepts []message.Message
agent.Run(ctx, messages)

// ❌ Compile error: won't accept wrong input type
agent.Run(ctx, "invalid input")
```

**Easy mocking for tests:**
```go
type mockAgent struct {
    responses []state.ExecutionResult
}

func (m *mockAgent) Run(ctx context.Context, input []message.Message, opts ...RunOption) iter.Seq2[state.ExecutionResult, error] {
    return func(yield func(state.ExecutionResult, error) bool) {
        for _, resp := range m.responses {
            if !yield(resp, nil) {
                return
            }
        }
    }
}

// Use mock in tests
var agent graph.MessageRunnable = &mockAgent{...}
```

**Composition:**
```go
// All components share the same interface
var agent1 graph.MessageRunnable = agent.NewReActAgent(model)
var agent2 graph.MessageRunnable = agent.NewSupervisorAgent(model)
compiled, _ := builder.Compile()

// Swap implementations without changing client code
```

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

Nodes are functions that receive a read-only state view and return updates:

```go
// Define typed keys
var (
    RawDataKey       = state.NewKey("raw_data", "")
    ProcessedDataKey = state.NewKey("processed_data", "")
    StatusKey        = state.NewKey("status", "")
    MessagesKey      = state.MessagesKey
)

func processDataFunc(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Read from state using typed keys
    data := state.GetFromView(view, RawDataKey)
    messages := state.GetFromView(view, MessagesKey)
    
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

Nodes receive immutable state views with typed key access:

```go
// Define typed keys
var (
    CounterKey  = state.NewKey("counter", 0)
    StatusKey   = state.NewKey("status", "")
    MessagesKey = state.MessagesKey
)

func myNode(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Read values using typed keys
    counter := state.GetFromView(view, CounterKey)
    status := state.GetFromView(view, StatusKey)
    messages := state.GetFromView(view, MessagesKey)
    
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
var (
    DraftKey    = state.NewKey("draft", "")
    FeedbackKey = state.NewKey("feedback", "")
)

builder.Node("writer", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    draft := generateDraft()
    return &graph.NodeResult{
        Updates: map[string]any{"draft": draft},
        NextNodes: []string{"evaluator"},
    }, nil
})

builder.Node("evaluator", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    draft := state.GetFromView(view, DraftKey)
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
