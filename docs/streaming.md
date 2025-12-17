---
layout: doc
title: Streaming
description: Real-time token-by-token output and progressive result delivery in AgentMesh.
permalink: /streaming/
example: streaming
hero:
  title: Streaming
  description: First-class support for streaming values from node execution, enabling real-time token-by-token output for LLM responses and progressive result delivery.
  primary_cta:
    label: View examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/streaming"
    external: true
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Basic Streaming
    url: "#basic-streaming"
  - title: Streaming with LLMs
    url: "#streaming-with-llm-responses"
  - title: Multiple Consumers
    url: "#multiple-stream-consumers"
  - title: Streaming in Agents
    url: "#streaming-in-agents"
  - title: Subgraph Streaming
    url: "#streaming-with-subgraphs"
  - title: Best Practices
    url: "#streaming-best-practices"
  - title: Testing
    url: "#testing-streamed-output"
  - title: Context Cancellation
    url: "#context-cancellation"
---

## Overview {#overview}

Streaming in AgentMesh is built into the `Scope` interface that every node receives. The `Scope` provides a `Stream(value message.Message)` method that allows nodes to emit values during execution, which are delivered to subscribers in real-time.

**Key insight:** Streamed values bypass the BSP state management entirely. They flow directly from nodes to the iterator without going through the Pregel barrier synchronization. This enables real-time delivery while state updates follow the standard superstep-based commit cycle.

<div class="mermaid">
flowchart TB
    subgraph Node["Node Execution"]
        NF["NodeFunc(ctx, scope)"]
        SC["scope.Stream(value)"]
        CMD["return Command"]
    end
    
    subgraph Streaming["Direct Streaming Path"]
        direction TB
        CH["Stream Channel"]
        IT["Iterator (iter.Seq2)"]
        OUT["Real-time Output"]
    end
    
    subgraph BSP["BSP State Path"]
        direction TB
        WB["Write Buffer"]
        BAR["Barrier Commit"]
        CS["Committed State"]
    end
    
    NF --> SC
    NF --> CMD
    
    SC -.->|"immediate"| CH
    CH -.->|"yields"| IT
    IT -.->|"real-time"| OUT
    
    CMD -->|"updates"| WB
    WB -->|"superstep end"| BAR
    BAR -->|"merged"| CS
    
    style Streaming fill:#e1f5fe,stroke:#01579b
    style BSP fill:#fff3e0,stroke:#e65100
    style Node fill:#f3e5f5,stroke:#7b1fa2
</div>

```go
// The Scope interface provides streaming capability
type Scope interface {
    ReadOnlyScope  // Embeds read-only state access
    
    // Stream emits a value directly to subscribers (bypasses BSP)
    Stream(value message.Message)
}

// ReadOnlyScope provides state access and node context
type ReadOnlyScope interface {
    GetValue(key string) (any, bool)
    ManagedValues() *ManagedValueRegistry
    ToMap() map[string]any
    NodeName() string  // Returns the name of the currently executing node
}
```

## Basic Streaming {#basic-streaming}

### Iterator-Based Streaming

AgentMesh uses Go's iterator pattern for streaming. When you run a graph, you receive values as they are streamed via the iterator:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Define your output type
type Output struct {
    Token   string
    Content string
}

// Define state key
var ContentKey = graph.NewKey[string]("content")

func main() {
    // Create a graph
    g := graph.New(ContentKey)
    
    // Add node
    g.Node("generate", generateNode, graph.END)
    g.Start("generate")

    compiled, err := g.Build()
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    
    // Iterate over streamed values
    for output, err := range compiled.Run(ctx, nil) {
        if err != nil {
            log.Fatal(err)
        }
        // Handle each streamed value as it arrives
        fmt.Print(output.Token)
    }
    
    fmt.Println("\nDone!")
}

func generateNode(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    content := ""
    tokens := []string{"Hello", " ", "World", "!"}
    
    for _, token := range tokens {
        content += token
        // Stream each token - immediately available to the iterator
        scope.Stream(Output{Token: token, Content: content})
    }
    
    return graph.Set(ContentKey, content).To(graph.END)
}
```

## Streaming with LLM Responses {#streaming-with-llm-responses}

A common use case is streaming LLM responses token by token. The built-in model nodes handle this automatically:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
    "github.com/hupe1980/agentmesh/pkg/model"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
)

func main() {
    // Create model with streaming enabled
    openaiModel := openai.NewModel()
    executor := model.NewExecutor(openaiModel)
    
    // Create graph
    g := graph.New(agent.MessagesKey)
    
    // Model node with streaming
    modelFn, _ := agent.NewModelNodeFunc(executor,
        agent.WithModelStreaming(true), // Enable streaming
    )
    
    g.Node("model", modelFn, graph.END)
    g.Start("model")
    
    compiled, _ := g.Build()
    
    messages := []message.Message{
        message.NewHumanMessageFromText("Hello!"),
    }
    
    // Stream execution - chunks arrive as they're generated
    for msg, err := range compiled.Run(context.Background(), messages) {
        if err != nil {
            log.Fatal(err)
        }
        
        switch m := msg.(type) {
        case *message.AIMessageChunk:
            // Partial streaming output - print immediately
            fmt.Print(m.String())
        case *message.AIMessage:
            // Final complete message - skip to avoid duplication
        }
    }
}
```

## Multiple Stream Consumers {#multiple-stream-consumers}

Since streaming uses iterators, you process all values in a single loop. To fan out to multiple consumers, process in the loop body:

```go
// Track multiple metrics in the same loop
var allTokens []string
var tokenCount int

for output, err := range compiled.Run(ctx, nil) {
    if err != nil {
        log.Fatal(err)
    }
    
    // Consumer 1: Collect tokens
    allTokens = append(allTokens, output.Token)
    
    // Consumer 2: Count tokens
    tokenCount++
    
    // Consumer 3: Display to user
    fmt.Print(output.Token)
}

fmt.Printf("\nReceived %d tokens\n", tokenCount)
```

## Streaming in Agents {#streaming-in-agents}

### Using WithStreaming Option

The built-in agents (ReAct, Supervisor, RAG) support streaming via the `WithStreaming` option:

```go
// Create agent with streaming enabled
reactAgent, err := agent.NewReAct(
    openai.NewModel(),
    agent.WithTools(weatherTool),
    agent.WithStreaming(true), // Enable streaming
)

// Run and handle streamed output
for msg, err := range reactAgent.Run(ctx, messages) {
    if err != nil {
        log.Fatal(err)
    }
    
    // Distinguish streaming chunks from final messages
    switch m := msg.(type) {
    case *message.AIMessageChunk:
        // Streaming partial output - print immediately
        fmt.Print(m.String())
    case *message.AIMessage:
        // Final complete message (already in state)
        // Skip printing to avoid duplication
    }
}
```

**Key types:**
- `*message.AIMessageChunk` - Streaming partial output, yielded in real-time. NOT added to state.
- `*message.AIMessage` - Final complete message, yielded after streaming completes. Added to state.

### Custom Agent Streaming

For custom agent implementations, use `scope.Stream()` to emit values:

```go
var MessagesKey = graph.NewListKey[message.Message]("messages")

func agentNode(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    // Get current state
    messages, _ := graph.ScopeGetList[message.Message](scope, MessagesKey.Name())
    
    // Stream LLM response chunks as they arrive
    response, err := streamLLMResponse(ctx, messages, func(chunk string) {
        scope.Stream(StreamChunk{Content: chunk})
    })
    if err != nil {
        return graph.Fail(err)
    }
    
    return graph.Set(MessagesKey, []message.Message{response}).To(graph.END)
}
```

## Streaming with Subgraphs {#streaming-with-subgraphs}

Subgraphs can stream values that propagate to the parent graph's iterator:

```go
// Child graph streams its output
childGraph := graph.New(ContentKey)
childGraph.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    scope.Stream(Output{Token: "from child"})
    return graph.To(graph.END)
}, graph.END)
childGraph.Start("process")

// Parent graph includes child as subgraph
parentGraph := graph.New(ContentKey)
parentGraph.Node("start", startNode, "child")
parentGraph.Subgraph("child", childGraph.MustBuild(), mapState, "finish")
parentGraph.Node("finish", finishNode, graph.END)
parentGraph.Start("start")

compiled, _ := parentGraph.Build()

// Iterator receives values from both parent and child nodes
for output, err := range compiled.Run(ctx, nil) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Received:", output.Token)
}
```

## Streaming Best Practices {#streaming-best-practices}

### 1. Type-Safe Streaming

Use specific output types for type safety:

```go
// Good: Specific type with clear semantics
type TokenOutput struct {
    Token     string
    Index     int
    Timestamp time.Time
}

var IndexKey = graph.NewKey[int]("index")

// The type parameter ensures type safety
func node(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    scope.Stream(TokenOutput{Token: "hello", Index: 0, Timestamp: time.Now()})
    return graph.To(graph.END)
}
```

### 2. Incremental Progress

Stream progress updates for long-running operations:

```go
type Progress struct {
    Current int
    Total   int
    Status  string
}

var ProcessedKey = graph.NewKey[int]("processed")

func processNode(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    items := getItems()
    total := len(items)
    
    for i, item := range items {
        scope.Stream(Progress{
            Current: i + 1,
            Total:   total,
            Status:  fmt.Sprintf("Processing %s", item.Name),
        })
        
        processItem(item)
    }
    
    return graph.Set(ProcessedKey, total).To(graph.END)
}
```

### 3. Error Context in Streams

Include error context in streamed values when appropriate:

```go
type Result struct {
    Data  string
    Error string
}

func node(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    for _, item := range items {
        result, err := process(item)
        if err != nil {
            scope.Stream(Result{Error: err.Error()})
            continue
        }
        scope.Stream(Result{Data: result})
    }
    return graph.To(graph.END)
}
```

## Testing Streamed Output {#testing-streamed-output}

Use `testutil.NewTestScopeFromMap` for testing nodes that stream:

```go
func TestStreamingNode(t *testing.T) {
    // Create test scope that captures streamed values
    scope := testutil.NewTestScopeFromMap[Output](map[string]any{"input": "test"})
    
    // Execute node
    cmd, err := myNode(context.Background(), scope)
    require.NoError(t, err)
    
    // Verify streamed values (captured in scope.Streamed)
    assert.Len(t, scope.Streamed, 3)
    assert.Equal(t, "first", scope.Streamed[0].Token)
    assert.Equal(t, "second", scope.Streamed[1].Token)
    assert.Equal(t, "third", scope.Streamed[2].Token)
}
```

## Context Cancellation {#context-cancellation}

Streaming respects context cancellation:

```go
func streamingNode(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
    for i := 0; i < 1000; i++ {
        select {
        case <-ctx.Done():
            // Context cancelled, stop streaming
            return graph.Fail(ctx.Err())
        default:
            scope.Stream(Output{Index: i})
        }
    }
    return graph.To(graph.END)
}

// Usage with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

for output, err := range compiled.Run(ctx, nil) {
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            fmt.Println("Timeout reached")
            break
        }
        log.Fatal(err)
    }
    fmt.Println(output.Index)
}
```

## Related Topics {#related-topics}

- [State Management](state-management.md) - Managing state with Scope
- [Agents](agents.md) - Building agents with streaming support
- [Builder API](builder-api.md) - Fluent API for graph construction
- [Testing](testing.md) - Testing streaming graphs
