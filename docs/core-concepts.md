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
  - title: Builder vs Graph
    url: "#runnable-interface"
  - title: Graphs and nodes
    url: "#graphs-and-nodes"
  - title: State management
    url: "#state-management"
  - title: Execution flow
    url: "#execution-flow"
  - title: Messages
    url: "#messages"
  - title: Error handling
    url: "#error-handling"
---

> **Type Safety with Generics (Go 1.24+):** AgentMesh provides full compile-time type safety through generic graph types. Use `graph.New[I, O](keys...)` for building graphs and `graph.Build()` to compile them. Use `message.NewGraphBuilder()` for conversational agents.

## Runnable interface {#runnable-interface}

The graph `Run` method is the core abstraction for executable workflows in AgentMesh. Only compiled graphs can be executed - this separation ensures graphs are validated before running.

### Builder vs Graph

AgentMesh separates graph **building** from graph **execution**:

```go
// Builder is for construction - define nodes and edges
b := graph.New[string, string](keys...)
b.Node("fetch", fetchFunc, "process")
b.Start("fetch")

// Graph is the compiled executor - run the workflow
compiled, err := b.Build()  // Validates and compiles
if err != nil {
    return err
}

// Only compiled Graph has Run()
for output, err := range compiled.Run(ctx, input) {
    // process outputs...
}
```

### Interface pattern

```go
// Graph.Run returns an iterator of outputs
func (g *Graph[I, O]) Run(ctx context.Context, input I, opts ...RunOption) iter.Seq2[O, error]
```

**Type parameters:**
- `I` - Input type (e.g., `[]message.Message`, `string`, custom struct)
- `O` - Output type (e.g., `message.Message`, custom result type)

### Common type aliases

For conversational agents, AgentMesh provides:

```go
// GraphBuilder is a builder for message-processing workflows
type GraphBuilder = graph.Builder[[]message.Message, message.Message]

// Graph is an executable message-processing workflow
type Graph = graph.Graph[[]message.Message, message.Message]
```

### Usage example

All agent constructors return `*message.Graph` (already compiled):

```go
import (
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/message"
)

// Agent constructors return *message.Graph (ready to run)
reactAgent, err := agent.NewReAct(model, agent.WithTools(tools...))
if err != nil {
    return err
}

// Execute with iterator pattern - no Build() needed!
for msg, err := range reactAgent.Run(ctx, messages) {
    if err != nil {
        return err
    }
    fmt.Println(msg.Content())
}
```

### Benefits

**Compile-time type safety:**
```go
// ✅ Type-safe: message.Graph accepts []message.Message
reactAgent.Run(ctx, messages)

// ❌ Compile error: won't accept wrong input type
reactAgent.Run(ctx, "invalid input")

// ❌ Compile error: can't run uncompiled builder
b := message.NewGraphBuilder()
b.Run(ctx, messages)  // Error: Builder has no Run method
```

**Easy composition:**
```go
// All agents are *message.Graph - compose freely
worker1, _ := agent.NewReAct(model)
worker2, _ := agent.NewReAct(model)
supervisor, _ := agent.NewSupervisor(model,
    agent.WithWorker("researcher", "Does research", worker1),
    agent.WithWorker("writer", "Writes content", worker2),
)
```

---

## Graphs and nodes {#graphs-and-nodes}

AgentMesh uses a **directed graph** model where computation flows through connected nodes.

### What is a graph?

A graph consists of:
- **Nodes** - Computational units that process data
- **Edges** - Connections that define execution order (declared as node targets)
- **State** - Shared context accessible via typed keys

<div class="mermaid">
flowchart TD
    subgraph Graph["Computation Graph"]
        START((START))
        fetch[fetch node]
        process[process node]
        save[save node]
        END((END))
        
        START --> fetch
        fetch --> process
        process --> save
        save --> END
    end
    
    subgraph State["Shared State"]
        S1["📦 raw_data"]
        S2["📦 processed_data"]
        S3["📦 status"]
    end
    
    fetch -.-> S1
    process -.-> S2
    save -.-> S3
</div>

### Building a graph

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Define typed state keys
var (
    RawDataKey       = graph.NewKey[string]("raw_data", "")
    ProcessedDataKey = graph.NewKey[string]("processed_data", "")
    StatusKey        = graph.NewKey[string]("status", "pending")
)

// Create graph with state keys
g := graph.New[string, string](RawDataKey, ProcessedDataKey, StatusKey)

// Add nodes with fluent API - targets are declared inline
g.Node("fetch", fetchDataFunc, "process").
  Node("process", processDataFunc, "save").
  Node("save", saveDataFunc, graph.END).
  Start("fetch")

// Compile into executable graph
compiled, err := g.Build()
if err != nil {
    return err
}

// Run the graph
for output, err := range compiled.Run(ctx, "input data") {
    if err != nil {
        return err
    }
    fmt.Println(output)
}
```

### Node functions

Nodes receive a read-only view of state and return a Command with updates and next targets:

```go
// NodeFunc signature
type NodeFunc func(ctx context.Context, view View) (*Command, error)

// Example node function
func processDataFunc(ctx context.Context, view graph.View) (*graph.Command, error) {
    // Read from state using typed keys
    rawData := graph.Get(view, RawDataKey)
    
    // Process the data
    processed := strings.ToUpper(rawData)
    
    // Return updates and next target using fluent API
    return graph.Set(ProcessedDataKey, processed).
        Set(StatusKey, "processed").
        To("save"), nil
}
```

### Special nodes

- **START** - Entry point (set via `Start()`)
- **END** - Terminal node constant (`graph.END`)

### Conditional routing

Dynamically route to different nodes based on state:

```go
var CategoryKey = graph.NewKey[string]("category", "")

func classifierFunc(ctx context.Context, view graph.View) (*graph.Command, error) {
    category := graph.Get(view, CategoryKey)
    
    switch category {
    case "urgent":
        return graph.To("urgent_handler"), nil
    case "normal":
        return graph.To("normal_handler"), nil
    default:
        return graph.To("default_handler"), nil
    }
}

// Node declares all possible targets
g.Node("classifier", classifierFunc, "urgent_handler", "normal_handler", "default_handler")
```

---

## State management {#state-management}

State is shared across all nodes using **typed keys** for compile-time safety.

<div class="mermaid">
flowchart LR
    subgraph Nodes["Graph Nodes"]
        N1[Node A]
        N2[Node B]
        N3[Node C]
    end
    
    subgraph State["State Keys"]
        direction TB
        C1["📬 messages\n(ListKey)"]
        C2["💾 counter\n(Key)"]
        C3["🏷️ status\n(Key)"]
    end
    
    N1 -->|"read"| State
    N1 -->|"updates"| State
    N2 -->|"read"| State
    N2 -->|"updates"| State
    N3 -->|"read"| State
    N3 -->|"updates"| State
    
    style C1 fill:#3b82f6,stroke:#60a5fa,color:#fff
    style C2 fill:#8b5cf6,stroke:#a78bfa,color:#fff
    style C3 fill:#10b981,stroke:#34d399,color:#fff
</div>

### Defining state keys

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Key[T] - single value, overwrites on update
var (
    CounterKey = graph.NewKey[int]("counter", 0)           // with default
    StatusKey  = graph.NewKey[string]("status", "pending")
)

// ListKey[T] - append-only list
var MessagesKey = graph.NewListKey[message.Message]("messages")
```

### Reading state

Nodes receive immutable state views with typed access:

```go
func myNode(ctx context.Context, view graph.View) (*graph.Command, error) {
    // Type-safe reads - returns the correct type
    counter := graph.Get(view, CounterKey)      // int
    status := graph.Get(view, StatusKey)        // string
    messages := graph.GetList(view, MessagesKey) // []message.Message
    
    return graph.To("next_node"), nil
}
```

### Updating state

Use the fluent Command builder for type-safe updates:

```go
func myNode(ctx context.Context, view graph.View) (*graph.Command, error) {
    counter := graph.Get(view, CounterKey)
    
    // Fluent, type-safe updates
    return graph.Set(CounterKey, counter + 1).
        Set(StatusKey, "processing").
        To("next_node"), nil
}

// For list keys, use Append
func addMessageNode(ctx context.Context, view graph.View) (*graph.Command, error) {
    newMsg := message.NewAIMessage(message.NewTextPart("Hello!"))
    
    return graph.Append(MessagesKey, newMsg).
        To("next_node"), nil
}
```

### MessageGraph convenience

For conversational agents, use `message.NewGraphBuilder()` which automatically includes the messages key:

```go
// Creates a graph with MessagesKey pre-registered
g := message.NewGraphBuilder()

// MessagesKey is available
g.Node("chat", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    messages := message.GetMessages(view)
    // ... process messages
    return graph.Append(message.MessagesKey, response).To(graph.END), nil
}, graph.END)
```

---

## Execution flow {#execution-flow}

AgentMesh executes graphs using **Pregel-style bulk synchronous parallel (BSP)** processing.

### Supersteps

Execution proceeds in discrete **supersteps**:

1. **Identify ready nodes** - Nodes with satisfied dependencies
2. **Execute in parallel** - Ready nodes run concurrently
3. **Apply updates** - State changes applied atomically
4. **Repeat** - Until END node or max iterations

<div class="mermaid">
sequenceDiagram
    participant E as Executor
    participant S as State
    participant N1 as Node A
    participant N2 as Node B
    participant N3 as Node C

    rect rgb(30, 41, 59)
    Note over E,N3: Superstep 0
    E->>S: Initialize state
    S-->>E: Ready nodes: [START]
    end

    rect rgb(30, 41, 59)
    Note over E,N3: Superstep 1 (Parallel)
    E->>N1: Execute node_a
    E->>N2: Execute node_b
    N1-->>S: Updates {counter: 1}
    N2-->>S: Updates {status: "done"}
    S->>S: Apply atomically
    end

    rect rgb(30, 41, 59)
    Note over E,N3: Superstep 2
    E->>N3: Execute node_c
    N3-->>S: Updates {result: "..."}
    end

    rect rgb(30, 41, 59)
    Note over E,N3: Superstep 3
    E->>E: Reach END node
    end
</div>

### Parallel execution

Nodes can fan out to multiple parallel tasks:

```go
g := graph.New[string, string](ResultKey)

// Entry node fans out to three parallel tasks
g.Node("start", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.To("fetch_a", "fetch_b", "fetch_c"), nil
}, "fetch_a", "fetch_b", "fetch_c")

// Each fetch task routes to aggregator
g.Node("fetch_a", fetchAFunc, "aggregator").
  Node("fetch_b", fetchBFunc, "aggregator").
  Node("fetch_c", fetchCFunc, "aggregator").
  Node("aggregator", aggregateFunc, graph.END).
  Start("start")
```

### Cycles and loops

Unlike DAG-based systems, AgentMesh supports **cycles** for iterative workflows:

```go
var (
    DraftKey    = graph.NewKey[string]("draft", "")
    IterationKey = graph.NewKey[int]("iteration", 0)
)

func writerFunc(ctx context.Context, view graph.View) (*graph.Command, error) {
    iteration := graph.Get(view, IterationKey)
    draft := generateDraft(iteration)
    
    return graph.Set(DraftKey, draft).
        Set(IterationKey, iteration + 1).
        To("evaluator"), nil
}

func evaluatorFunc(ctx context.Context, view graph.View) (*graph.Command, error) {
    draft := graph.Get(view, DraftKey)
    iteration := graph.Get(view, IterationKey)
    
    if isGoodEnough(draft) || iteration >= 5 {
        return graph.To(graph.END), nil
    }
    // Loop back for refinement
    return graph.To("writer"), nil
}

g := graph.New[string, string](DraftKey, IterationKey)
g.Node("writer", writerFunc, "evaluator").
  Node("evaluator", evaluatorFunc, graph.END, "writer"). // declares both targets
  Start("writer")
```

### Max iterations

Prevent infinite loops with run options:

```go
for output, err := range compiled.Run(ctx, input, 
    graph.WithMaxIterations(10),
) {
    // ...
}
```

---

## Messages {#messages}

Messages represent conversation turns between users, AI, and tools.

### Message types

```go
import "github.com/hupe1980/agentmesh/pkg/message"

// Human input (simple text)
humanMsg := message.NewHumanMessageFromText("What's the weather?")

// AI response (simple text)
aiMsg := message.NewAIMessageFromText("It's sunny and 72°F")

// System prompt (simple text)
systemMsg := message.NewSystemMessageFromText("You are a helpful assistant")

// For multi-part messages, use Parts slice
multiPart := message.NewHumanMessage([]message.Part{
    message.TextPart{Text: "Describe this image:"},
    message.FilePart{MimeType: "image/png", File: message.FileURI{URI: imageURL}},
})

// Tool call
toolCall := message.ToolCall{
    ID:        "call_123",
    Name:      "get_weather",
    Type:      "function",
    Arguments: `{"location":"Paris"}`,  // JSON string
}
aiWithTool := message.NewAIMessage(
    []message.Part{message.TextPart{Text: "Let me check"}},
    message.WithToolCalls(toolCall),
)

// Tool result
toolMsg := message.NewToolMessage("call_123", "Sunny, 22°C")
```

### Message parts

Messages can contain multiple parts:

```go
aiMsg := message.NewAIMessage([]message.Part{
    message.TextPart{Text: "Here's the weather"},
    message.FilePart{
        MimeType: "image/png",
        File:     message.FileURI{URI: imageURL},
    },
})

// Access parts
for _, part := range aiMsg.Parts() {
    switch p := part.(type) {
    case message.TextPart:
        fmt.Println("Text:", p.Text)
    case message.FilePart:
        fmt.Println("File:", p.Name, p.MimeType)
    }
}
```

---

## Error handling {#error-handling}

AgentMesh uses **sentinel errors** with `errors.Is()` support for programmatic error checking.

### Sentinel errors

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

for output, err := range compiled.Run(ctx, input) {
    if err != nil {
        switch {
        case errors.Is(err, graph.ErrNotBuilt):
            log.Error("Graph not compiled - call Build() first")
            
        case errors.Is(err, graph.ErrNoEntryPoint):
            log.Error("No entry point set - call Start()")
            
        case errors.Is(err, graph.ErrNodeNotFound):
            log.Error("Referenced node doesn't exist")
            
        case errors.Is(err, graph.ErrDuplicateNode):
            log.Error("Node name already used")
            
        case errors.Is(err, graph.ErrDuplicateKey):
            log.Error("State key name already registered")
            
        default:
            return err
        }
    }
}
```

### Available sentinel errors

| Error | Description |
|-------|-------------|
| `ErrNoEntryPoint` | No entry point defined (call `Start()`) |
| `ErrNodeNotFound` | Node not found in graph |
| `ErrDuplicateNode` | Duplicate node name |
| `ErrDuplicateKey` | Duplicate state key name |
| `ErrInvalidTarget` | Invalid target node reference |
| `ErrNotBuilt` | Graph not built (call `Build()` first) |

### InterruptError

For human-in-the-loop workflows:

```go
var interruptErr *graph.InterruptError
if errors.As(err, &interruptErr) {
    fmt.Printf("Interrupted at node %s (before=%v)\n", 
        interruptErr.NodeName, interruptErr.Before)
    
    // Resume with approval
    for output, err := range compiled.Run(ctx, input,
        graph.WithApproval(interruptErr.NodeName, &graph.ApprovalResponse{
            Decision: graph.ApprovalApproved,
        }),
    ) {
        // ...
    }
}
```

---

## Next steps

- **[Agents](/agents/)** - Build ReAct, Supervisor, and RAG agents
- **[Tools](/tools/)** - Create function tools for agent capabilities
- **[Checkpointing](/checkpointing/)** - State persistence and time travel debugging
- **[Streaming](/streaming/)** - Real-time execution events
- **[Observability](/observability/)** - OpenTelemetry metrics and tracing
- **[Architecture](/architecture/)** - Understand Pregel BSP internals
