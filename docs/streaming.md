---
layout: default
title: Streaming
nav_order: 9
---

# Streaming
{: .no_toc }

AgentMesh provides first-class support for streaming values from node execution, enabling real-time token-by-token output for LLM responses and progressive result delivery.

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

Streaming in AgentMesh is built into the `Scope[O]` interface that every node receives. The `Scope` provides a `Stream(value O)` method that allows nodes to emit values during execution, which are delivered to subscribers in real-time.

**Key insight:** Streamed values bypass the BSP state management entirely. They flow directly from nodes to subscribers without going through the Pregel barrier synchronization. This enables real-time delivery while state updates follow the standard superstep-based commit cycle.

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
        SH["Stream Handlers"]
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
    CH -.->|"fan-out"| SH
    SH -.->|"real-time"| OUT
    
    CMD -->|"updates"| WB
    WB -->|"superstep end"| BAR
    BAR -->|"merged"| CS
    
    style Streaming fill:#e1f5fe,stroke:#01579b
    style BSP fill:#fff3e0,stroke:#e65100
    style Node fill:#f3e5f5,stroke:#7b1fa2
</div>

```go
// The Scope interface provides streaming capability
type Scope[O any] interface {
    ReadOnlyScope  // Embeds read-only state access
    
    // Stream emits a value directly to subscribers (bypasses BSP)
    Stream(value O)
}

// ReadOnlyScope provides state access and node context
type ReadOnlyScope interface {
    GetValue(key string) (any, bool)
    ManagedValues() *ManagedValueRegistry
    ToMap() map[string]any
    NodeName() string  // Returns the name of the currently executing node
}
```

## Basic Streaming

### Setting Up Stream Handling

When running a graph, you can subscribe to streamed values using `WithStreamHandler`:

```go
package main

import (
    "context"
    "fmt"

    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Define your output type
type Output struct {
    Token   string
    Content string
}

func main() {
    // Create a graph with Output type
    g := graph.New[Output](
        graph.NewNode("generate", generateNode),
    )

    ctx := context.Background()
    
    // Run with stream handler
    output, err := g.Run(ctx, nil,
        graph.WithStreamHandler(func(value Output) {
            // Handle each streamed value
            fmt.Print(value.Token)
        }),
    )
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("\n\nFinal: %s\n", output.Content)
}

func generateNode(ctx context.Context, scope graph.Scope[Output]) (*graph.Command, error) {
    content := ""
    tokens := []string{"Hello", " ", "World", "!"}
    
    for _, token := range tokens {
        content += token
        // Stream each token
        scope.Stream(Output{Token: token, Content: content})
    }
    
    return graph.NewCommand(
        graph.WithUpdate("content", content),
    ), nil
}
```

## Streaming with LLM Responses

A common use case is streaming LLM responses token by token:

```go
type ChatOutput struct {
    Chunk   string
    Message Message
}

func chatNode(ctx context.Context, scope graph.Scope[ChatOutput]) (*graph.Command, error) {
    messages, _ := graph.ScopeGetList[Message](scope, "messages")
    
    // Call LLM with streaming
    stream, err := llm.StreamChat(ctx, messages)
    if err != nil {
        return nil, err
    }
    
    var fullResponse strings.Builder
    
    for chunk := range stream.Chunks() {
        fullResponse.WriteString(chunk.Content)
        
        // Stream each chunk to subscribers
        scope.Stream(ChatOutput{
            Chunk: chunk.Content,
            Message: Message{
                Role:    "assistant",
                Content: fullResponse.String(),
            },
        })
    }
    
    return graph.NewCommand(
        graph.WithAppendToList("messages", Message{
            Role:    "assistant",
            Content: fullResponse.String(),
        }),
    ), nil
}
```

## Multiple Stream Consumers

You can have multiple consumers process the same stream:

```go
// Create consumers
var allTokens []string
var tokenCount int

output, err := g.Run(ctx, nil,
    graph.WithStreamHandler(func(value Output) {
        // Consumer 1: Collect tokens
        allTokens = append(allTokens, value.Token)
    }),
    graph.WithStreamHandler(func(value Output) {
        // Consumer 2: Count tokens
        tokenCount++
    }),
    graph.WithStreamHandler(func(value Output) {
        // Consumer 3: Display to user
        fmt.Print(value.Token)
    }),
)
```

## Streaming in Agents

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

For custom agent implementations:

```go
func agentNode(ctx context.Context, scope graph.Scope[StreamChunk]) (*graph.Command, error) {
    // Get current state
    messages, _ := graph.ScopeGetList[Message](scope, "messages")
    
    // Stream LLM response
    response, err := streamLLMResponse(ctx, messages, func(chunk string) {
        scope.Stream(StreamChunk{Content: chunk})
    })
    if err != nil {
        return nil, err
    }
    
    return graph.NewCommand(
        graph.WithAppendToList("messages", response),
    ), nil
}
```

## Streaming with Subgraphs

Subgraphs can stream values that propagate to the parent graph:

```go
// Child graph streams its output
childGraph := graph.New[Output](
    graph.NewNode("process", func(ctx context.Context, scope graph.Scope[Output]) (*graph.Command, error) {
        scope.Stream(Output{Token: "from child"})
        return graph.End(), nil
    }),
)

// Parent graph includes child as subgraph
parentGraph := graph.New[Output](
    graph.NewNode("start", startNode),
    graph.NewSubgraph("child", childGraph, mapState),
    graph.NewNode("finish", finishNode),
)

// Stream handler receives values from both parent and child nodes
output, err := parentGraph.Run(ctx, nil,
    graph.WithStreamHandler(func(value Output) {
        fmt.Println("Received:", value.Token)
    }),
)
```

## Streaming Best Practices

### 1. Type-Safe Streaming

Use specific output types for type safety:

```go
// Good: Specific type with clear semantics
type TokenOutput struct {
    Token     string
    Index     int
    Timestamp time.Time
}

// The type parameter ensures type safety
func node(ctx context.Context, scope graph.Scope[TokenOutput]) (*graph.Command, error) {
    scope.Stream(TokenOutput{Token: "hello", Index: 0, Timestamp: time.Now()})
    return graph.End(), nil
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

func processNode(ctx context.Context, scope graph.Scope[Progress]) (*graph.Command, error) {
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
    
    return graph.NewCommand(
        graph.WithUpdate("processed", total),
    ), nil
}
```

### 3. Error Context in Streams

Include error context in streamed values when appropriate:

```go
type Result struct {
    Data  string
    Error string
}

func node(ctx context.Context, scope graph.Scope[Result]) (*graph.Command, error) {
    for _, item := range items {
        result, err := process(item)
        if err != nil {
            scope.Stream(Result{Error: err.Error()})
            continue
        }
        scope.Stream(Result{Data: result})
    }
    return graph.End(), nil
}
```

## Testing Streamed Output

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

## Context Cancellation

Streaming respects context cancellation:

```go
func streamingNode(ctx context.Context, scope graph.Scope[Output]) (*graph.Command, error) {
    for i := 0; i < 1000; i++ {
        select {
        case <-ctx.Done():
            // Context cancelled, stop streaming
            return nil, ctx.Err()
        default:
            scope.Stream(Output{Index: i})
        }
    }
    return graph.End(), nil
}

// Usage with timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

output, err := g.Run(ctx, nil,
    graph.WithStreamHandler(func(value Output) {
        fmt.Println(value.Index)
    }),
)
```

## Related Topics

- [State Management](state-management.md) - Managing state with Scope
- [Agents](agents.md) - Building agents with streaming support
- [Builder API](builder-api.md) - Fluent API for graph construction
- [Testing](testing.md) - Testing streaming graphs
