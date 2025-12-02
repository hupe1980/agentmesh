---
layout: doc
title: Streaming
description: Real-time streaming of graph execution in AgentMesh
---

# Streaming

AgentMesh provides built-in support for streaming graph execution results in real-time. This enables responsive UIs, progress monitoring, and handling long-running workflows.

---

## Overview

Streaming in AgentMesh works through:

1. **Iterators**: The `Run()` method returns an iterator (`iter.Seq2`) that yields execution results
2. **Intermediate Updates**: Nodes emit intermediate results via the StreamWriter
3. **Event Processing**: Consumers iterate over results as they become available

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Define state keys
var StatusKey = graph.NewKey("status", "")

// Create graph
g := graph.New[string, string](StatusKey)

g.Node("process", func(ctx context.Context, input graph.NodeInput[string]) (graph.Command, error) {
    // Get the stream writer from context
    streamWriter := graph.GetStreamWriter(ctx)
    
    // Emit intermediate updates
    if streamWriter != nil {
        streamWriter(&graph.NodeResult{
            Updates: map[string]any{"progress": 0.5},
        })
    }
    
    return graph.Set(StatusKey, "complete").ToEnd(), nil
}, graph.END)

g.Start("process")

compiled, _ := g.Build()

// Stream results
for result, err := range compiled.Run(ctx, "start") {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Node: %s, Updates: %v\n", result.Node, result.Updates)
}
```

---

## Core Concepts

### ExecutionResult

Each yielded result contains:

```go
type ExecutionResult[O any] struct {
    Node    string         // Name of the node that produced this result
    Updates map[string]any // State updates from this node
    Output  O              // Final output (on final iteration only)
}
```

### StreamWriter

The StreamWriter allows nodes to emit intermediate results during execution:

```go
type StreamWriter func(*NodeResult)

type NodeResult struct {
    Updates map[string]any
}
```

---

## Basic Streaming

### Processing Results

```go
var CounterKey = graph.NewKey("counter", 0)

g := graph.New[int, int](CounterKey)

g.Node("counter", func(ctx context.Context, input graph.NodeInput[int]) (graph.Command, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    
    for i := 1; i <= 10; i++ {
        time.Sleep(100 * time.Millisecond)
        
        // Emit progress updates
        if streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "progress": i * 10,
                    "step":     i,
                },
            })
        }
    }
    
    return graph.Set(CounterKey, 10).ToEnd(), nil
}, graph.END)

g.Start("counter")

compiled, _ := g.Build()

// Process each result as it arrives
for result, err := range compiled.Run(ctx, 0) {
    if err != nil {
        log.Fatal(err)
    }
    
    if result.Updates != nil {
        fmt.Printf("Progress: %v%%\n", result.Updates["progress"])
    }
}
```

### Collecting All Results

```go
// Process all results using iterator pattern
var results []graph.View
for result, err := range compiled.Run(ctx, input) {
    if err != nil {
        log.Fatal(err)
    }
    results = append(results, result)
}

for _, result := range results {
    // Access state from result view
    status := graph.Get(result, StatusKey)
    fmt.Printf("Status: %s\n", status)
}
```

### Getting Final Result Only

```go
// Get only the last (final) result
result, err := graph.Last(compiled.Run(ctx, input))
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Final output: %v\n", result.Output)
```

---

## Message Graph Streaming

For conversational AI workflows using `MessageGraph`:

```go
g := message.NewGraphBuilder()

g.Node("assistant", func(ctx context.Context, input graph.NodeInput[[]message.Message]) (graph.Command, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    messages := input.State(message.MessagesKey)
    
    // Stream LLM response tokens
    var fullResponse strings.Builder
    
    llmStream := model.StreamChat(ctx, messages)
    for chunk := range llmStream {
        fullResponse.WriteString(chunk.Content)
        
        // Stream each token to the client
        if streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "type":    "token",
                    "content": chunk.Content,
                },
            })
        }
    }
    
    response := message.NewAIMessageFromText(fullResponse.String())
    return graph.Append(message.MessagesKey, response).ToEnd(), nil
}, graph.END)

g.Start("assistant")

compiled, _ := g.Build()

// Stream chat tokens
for result, err := range compiled.Run(ctx, []message.Message{userMsg}) {
    if err != nil {
        log.Fatal(err)
    }
    
    if result.Updates["type"] == "token" {
        fmt.Print(result.Updates["content"]) // Print tokens as they arrive
    }
}
```

---

## Multi-Node Streaming

Stream results across multiple nodes:

```go
var (
    DataKey   = graph.NewListKey[string]("data")
    StatusKey = graph.NewKey("status", "")
)

g := graph.New[string, string](DataKey, StatusKey)

// First node: fetch data
g.Node("fetch", func(ctx context.Context, input graph.NodeInput[string]) (graph.Command, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    
    if streamWriter != nil {
        streamWriter(&graph.NodeResult{
            Updates: map[string]any{"stage": "fetching"},
        })
    }
    
    time.Sleep(1 * time.Second) // Simulate fetch
    
    return graph.Set(StatusKey, "fetched").Append(DataKey, "item1", "item2").To("process"), nil
}, "process")

// Second node: process data
g.Node("process", func(ctx context.Context, input graph.NodeInput[string]) (graph.Command, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    data := graph.GetList(input, DataKey)
    
    for i, item := range data {
        if streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "stage":    "processing",
                    "item":     item,
                    "progress": float64(i+1) / float64(len(data)),
                },
            })
        }
        time.Sleep(500 * time.Millisecond)
    }
    
    return graph.Set(StatusKey, "complete").ToEnd(), nil
}, graph.END)

g.Start("fetch")

compiled, _ := g.Build()

// Track progress across all nodes
for result, err := range compiled.Run(ctx, "start") {
    if err != nil {
        log.Fatal(err)
    }
    
    if stage, ok := result.Updates["stage"]; ok {
        fmt.Printf("[%s] Stage: %s\n", result.Node, stage)
    }
}
```

---

## Context Cancellation

Streaming respects context cancellation:

```go
// Create cancellable context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Iteration stops when context is cancelled
for result, err := range compiled.Run(ctx, input) {
    if err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            fmt.Println("Execution timed out")
        }
        break
    }
    processResult(result)
}
```

---

## Best Practices

### 1. Always Check StreamWriter

```go
func processNode(ctx context.Context, input graph.NodeInput[string]) (graph.Command, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    
    // Always check if streaming is enabled
    if streamWriter != nil {
        streamWriter(&graph.NodeResult{
            Updates: map[string]any{"status": "starting"},
        })
    }
    
    return graph.Set(StatusKey, "done").ToEnd(), nil
}
```

### 2. Throttle High-Frequency Updates

```go
// ❌ Bad - too many events
for i := 0; i < 1000000; i++ {
    if streamWriter != nil {
        streamWriter(&graph.NodeResult{...})  // Don't do this!
    }
}

// ✅ Good - throttle updates
for i := 0; i < 1000000; i++ {
    if i % 1000 == 0 && streamWriter != nil {
        streamWriter(&graph.NodeResult{
            Updates: map[string]any{"progress": i},
        })
    }
}
```

### 3. Use Structured Updates

```go
// ✅ Good - structured, predictable updates
streamWriter(&graph.NodeResult{
    Updates: map[string]any{
        "stage":    "processing",
        "progress": 0.5,
        "message":  "Processing batch 2/4",
    },
})

// ❌ Bad - inconsistent structure
streamWriter(&graph.NodeResult{
    Updates: map[string]any{"status": "working"},
})
streamWriter(&graph.NodeResult{
    Updates: map[string]any{"pct": 50},
})
```

### 4. Emit Meaningful Events

```go
// ✅ Good - provides useful information
streamWriter(&graph.NodeResult{
    Updates: map[string]any{
        "operation":    "database_query",
        "rows_fetched": 1500,
        "duration_ms":  234,
    },
})

// ❌ Bad - not useful
streamWriter(&graph.NodeResult{
    Updates: map[string]any{"x": 1},
})
```

---

## Integration with UI Frameworks

### React/Web Frontend

```typescript
// Frontend code to consume stream
async function executeGraph(messages: Message[]) {
    const response = await fetch('/api/graph/stream', {
        method: 'POST',
        body: JSON.stringify({ messages }),
    });
    
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    
    while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        
        const chunk = decoder.decode(value);
        const event = JSON.parse(chunk);
        
        // Update UI based on event
        if (event.Updates) {
            updateProgress(event.Updates);
        }
    }
}
```

### HTTP Server (SSE)

```go
func streamHandler(w http.ResponseWriter, r *http.Request) {
    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming not supported", http.StatusInternalServerError)
        return
    }

    // Execute graph and stream events
    seq := compiled.Run(r.Context(), messages)

    // Forward events to SSE
    for event, err := range seq {
        if err != nil {
            // Handle error, maybe send an SSE error event
            break
        }
        
        data, _ := json.Marshal(event)
        fmt.Fprintf(w, "data: %s\n\n", data)
        flusher.Flush()
    }
}
```

---

## Performance Considerations

### Memory Usage

- Each execution result allocates memory for the `ExecutionResult` struct
- Results are yielded via iterator - minimal memory overhead
- For high-frequency updates, consider throttling or batching

### Iterator Benefits

- **Lazy Evaluation**: Results processed on-demand
- **Low Memory**: Only current result in memory
- **Early Termination**: Break from loop to stop execution
- **Context Cancellation**: Respects context cancellation immediately

### Concurrency

- The iterator pattern is inherently sequential for the consumer
- Internal execution (e.g., Pregel workers) runs in parallel
- StreamWriter can be called from multiple goroutines safely
- Results are serialized through the iterator

---

## Execution Patterns

All graph execution uses the same `Run()` method that returns an iterator:

```go
// Process all results
for result, err := range compiled.Run(ctx, messages) {
    if err != nil {
        return err
    }
    handleResult(result)
}

// Collect all results manually
var results []message.Message
for msg, err := range compiled.Run(ctx, messages) {
    if err != nil {
        return err
    }
    results = append(results, msg)
}

// Get only the last result
var lastMsg message.Message
for msg, err := range compiled.Run(ctx, messages) {
    if err != nil {
        return err
    }
    lastMsg = msg
}
```

---

## Advanced Topics

### Custom Event Types

You can include custom metadata in intermediate updates:

```go
streamWriter(&graph.NodeResult{
    Updates: map[string]any{
        "event_type":  "custom_metric",
        "metric_name": "tokens_per_second",
        "value":       125.3,
        "timestamp":   time.Now(),
    },
})
```

### Conditional Streaming

```go
var VerboseKey = graph.NewKey("verbose", false)

g.Node("process", func(ctx context.Context, input graph.NodeInput[any]) (graph.Command, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    verbose := graph.Get(input, VerboseKey)
    
    for _, item := range items {
        processItem(item)
        
        // Only stream if verbose mode enabled
        if verbose && streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{"processed": item},
            })
        }
    }
    
    return graph.Set(StatusKey, "done").ToEnd(), nil
}, graph.END)
```

### Error Handling

```go
g.Node("processor", func(ctx context.Context, input graph.NodeInput[any]) (graph.Command, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    
    // Errors in intermediate updates don't stop execution
    if streamWriter != nil {
        streamWriter(&graph.NodeResult{
            Updates: map[string]any{
                "warning": "Rate limit approaching",
            },
        })
    }
    
    // Returning error stops execution
    if criticalError != nil {
        return nil, fmt.Errorf("critical: %w", criticalError)
    }
    
    return graph.Set(StatusKey, "done").ToEnd(), nil
}, graph.END)
```

---

## See Also

- [Graph Execution](./getting-started.md#execution)
- [Node Implementation](./architecture.md#nodes)
- [State Management](./state-management.md)
- [Complete Example](../examples/streaming/main.go)
