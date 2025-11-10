---
layout: doc
title: Streaming & Real-Time Updates
---

# Streaming & Real-Time Updates

AgentMesh provides comprehensive streaming capabilities that enable real-time visibility into graph execution. Unlike simple completion-based APIs, streaming allows you to observe intermediate progress, node-by-node execution, and incremental updates as they happen.

## Overview

AgentMesh supports two complementary streaming patterns:

1. **Graph-Level Streaming**: Observe execution flow between nodes
2. **Node-Level Streaming**: Emit intermediate progress from within nodes

Both patterns work seamlessly together to provide complete visibility into your agent workflows.

---

## Graph-Level Streaming

### Basic Usage

Use `Stream()` instead of `Invoke()` to execute graphs with real-time event streaming:

```go
// Create and compile your graph
compiled, err := builder.Compile()
if err != nil {
    log.Fatal(err)
}

// Start streaming execution
seq := compiled.Run(ctx, messages)

// Process events as they arrive
for stream.Next() {
    event := stream.Current()
    
    // Handle the event
    fmt.Printf("Node: %s\n", event.Node)
    fmt.Printf("Updates: %v\n", event.Updates)
    
    if event.Err != nil {
        fmt.Printf("Error: %v\n", event.Err)
    }
}

// Check for stream errors
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
```

### Stream Events

Each `StreamEvent` contains:

```go
type StreamEvent struct {
    Node     string              // Name of the node that produced this event
    Updates  map[string]any      // State updates (final node result)
    Messages []message.Message   // New messages added to state
    Result   *NodeResult         // Intermediate result (from StreamWriter)
    Err      error              // Error if node failed
}
```

**Key Distinction**:
- `Updates` and `Messages`: Final node results applied to state
- `Result`: Intermediate updates from `StreamWriter` (not applied to state)

### Event Types

1. **Node Start**: `Node != ""`, `Updates == nil`, `Result == nil`
2. **Intermediate Update**: `Node != ""`, `Result != nil` (from StreamWriter)
3. **Node Completion**: `Node != ""`, `Updates != nil`, `Result == nil`
4. **Error**: `Err != nil`

---

## Node-Level Streaming (StreamWriter Pattern)

### The StreamWriter Pattern

Nodes can emit intermediate progress without waiting for completion using the **StreamWriter pattern**:

```go
builder.Node("processor", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Get the stream writer from context
    streamWriter := graph.GetStreamWriter(ctx)
    
    // Process data in chunks
    for i, chunk := range chunks {
        processChunk(chunk)
        
        // Emit intermediate progress
        if streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "progress": fmt.Sprintf("%d/%d", i+1, len(chunks)),
                    "chunk":    chunk,
                },
            })
        }
    }
    
    // Return final result
    return &graph.NodeResult{
        Updates: map[string]any{"status": "complete"},
    }, nil
})
```

### How It Works

1. **Extract StreamWriter**: `streamWriter := graph.GetStreamWriter(ctx)`
2. **Check for nil**: StreamWriter is only available during `Stream()` calls
3. **Emit Updates**: Call `streamWriter(result)` to send intermediate events
4. **Return Final Result**: Node still returns its final `NodeResult` as usual

**Important**: Intermediate updates from StreamWriter are **not applied to graph state**. They are purely for observation and user feedback.

---

## Complete Example

Here's a comprehensive example demonstrating both streaming patterns:

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
)

func main() {
    builder := graph.NewBuilder()
    
    // Node 1: Data processor with intermediate streaming
    builder.Node("data_processor", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
        streamWriter := graph.GetStreamWriter(ctx)
        
        chunks := []string{"chunk1", "chunk2", "chunk3", "chunk4"}
        for i, chunk := range chunks {
            time.Sleep(300 * time.Millisecond)
            
            // Emit intermediate progress
            if streamWriter != nil {
                streamWriter(&graph.NodeResult{
                    Updates: map[string]any{
                        "progress":      fmt.Sprintf("%d/%d", i+1, len(chunks)),
                        "current_chunk": chunk,
                    },
                })
            }
        }
        
        // Return final result (applied to state)
        return &graph.NodeResult{
            Updates: map[string]any{
                "status":       "data_processed",
                "chunks_total": len(chunks),
            },
        }, nil
    })
    
    // Node 2: Multi-step analyzer
    builder.Node("analyzer", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
        streamWriter := graph.GetStreamWriter(ctx)
        
        // Step 1: Validation
        time.Sleep(200 * time.Millisecond)
        if streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "step":       "validation",
                    "validation": "passed",
                },
            })
        }
        
        // Step 2: Quality check
        time.Sleep(200 * time.Millisecond)
        if streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "step":          "quality_check",
                    "quality_score": 0.95,
                },
            })
        }
        
        // Final result
        return &graph.NodeResult{
            Updates: map[string]any{
                "status":   "complete",
                "verified": true,
            },
        }, nil
    })
    
    // Build graph topology
    builder.AddEdge(graph.StartNode, "data_processor")
    builder.AddEdge("data_processor", "analyzer")
    builder.AddEdge("analyzer", graph.EndNode)
    
    compiled, _ := builder.Compile()
    
    // Execute with streaming
    seq := compiled.Run(context.Background(), nil)
    
    // Process events
    for event, err := range seq {
        if err != nil {
            fmt.Printf("❌ Error: %v\n", err)
            continue
        }
        
        // Node started
        if event.Node != "" && event.Result == nil && len(event.Updates) == 0 {
            fmt.Printf("\n📍 Starting node: %s\n", event.Node)
        }
        
        // Intermediate update (from StreamWriter)
        if event.Result != nil {
            fmt.Printf("   ⚡ Progress: %v\n", event.Result.Updates)
        }
        
        // Final update (applied to state)
        if len(event.Updates) > 0 {
            fmt.Printf("   ✅ Completed: %v\n", event.Updates)
        }
    }
}
```

**Output**:
```
📍 Starting node: data_processor
   ⚡ Progress: map[progress:1/4 current_chunk:chunk1]
   ⚡ Progress: map[progress:2/4 current_chunk:chunk2]
   ⚡ Progress: map[progress:3/4 current_chunk:chunk3]
   ⚡ Progress: map[progress:4/4 current_chunk:chunk4]
   ✅ Completed: map[status:data_processed chunks_total:4]

📍 Starting node: analyzer
   ⚡ Progress: map[step:validation validation:passed]
   ⚡ Progress: map[step:quality_check quality_score:0.95]
   ✅ Completed: map[status:complete verified:true]
```

---

## Use Cases

### 1. Progress Bars and Loading States

```go
builder.Node("batch_processor", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    
    for i, item := range items {
        process(item)
        
        if streamWriter != nil {
            percentage := float64(i+1) / float64(len(items)) * 100
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "progress_percent": percentage,
                    "items_processed":  i + 1,
                    "items_total":      len(items),
                },
            })
        }
    }
    
    return &graph.NodeResult{Updates: map[string]any{"status": "complete"}}, nil
})
```

### 2. LLM Token Streaming

```go
builder.Node("llm_call", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    
    // Stream from the model
    seq := model.Generate(ctx, &model.Request{
        Messages: messages,
        Stream:   true,
    })
    
    var fullResponse strings.Builder
    
    // Forward model tokens to graph stream
    for chunk, err := range seq {
        if err != nil {
            return nil, err
        }
        
        // Stream partial response out of the node
        sw.Write(message.NewAIMessageFromText(chunk.Message.String()))
        
        fullResponse.WriteString(chunk.Message.String())
    }
    
    // Return final, complete message
    return &graph.NodeResult{
        Messages: []message.Message{
            message.NewAIMessageFromText(fullResponse.String()),
        },
    }, nil
})
```

### 3. Multi-Stage Processing

```go
builder.Node("pipeline", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    
    stages := []struct {
        name string
        fn   func() error
    }{
        {"Loading data", loadData},
        {"Transforming", transform},
        {"Validating", validate},
        {"Saving", save},
    }
    
    for i, stage := range stages {
        if streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{
                    "current_stage": stage.name,
                    "stage_number":  i + 1,
                    "total_stages":  len(stages),
                },
            })
        }
        
        if err := stage.fn(); err != nil {
            return nil, err
        }
    }
    
    return &graph.NodeResult{Updates: map[string]any{"status": "complete"}}, nil
})
```

---

## Best Practices

### 1. Always Check for nil

```go
// ✅ Good
if streamWriter != nil {
    streamWriter(result)
}

// ❌ Bad - will panic if Stream() not used
streamWriter(result)
```

StreamWriter is only available during `Stream()` calls. Regular `Invoke()` calls don't provide a StreamWriter.

### 2. Don't Stream Excessively

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
        if (event.Result) {
            // Intermediate update
            updateProgress(event.Result.Updates);
        } else if (event.Updates) {
            // Final node result
            updateNodeStatus(event.Node, event.Updates);
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

## Proper Cleanup

For proper cleanup and resource management:

- Always cancel contexts when done
- Close any open streams or connections
- Handle errors and edge cases in streaming logic

---

## Performance Considerations

### Memory Usage

- Each streamed event allocates memory for the `StreamEvent` struct
- Intermediate updates are **not** stored in state (unlike final results)
- For high-frequency streaming, consider throttling or batching

### Network Overhead

When streaming over HTTP:
- Each event requires a network round-trip
- Consider batching small updates
- Use compression for large payloads

### Concurrency

- Streaming is safe for concurrent use
- Multiple goroutines can call `streamWriter` simultaneously
- Events are serialized through the stream channel

---

## Comparison with Invoke()

| Feature | `Invoke()` | `Stream()` |
|---------|-----------|-----------|
| Return Type | Final messages only | Real-time events |
| Progress Visibility | None | Full visibility |
| Overhead | Minimal | Slight overhead for events |
| StreamWriter Available | No | Yes |
| Use Case | Simple scripts, batch jobs | Interactive UIs, monitoring |

---

## Advanced Topics

### Custom Event Types

You can include custom metadata in intermediate updates:

```go
streamWriter(&graph.NodeResult{
    Updates: map[string]any{
        "event_type": "custom_metric",
        "metric_name": "tokens_per_second",
        "value": 125.3,
        "timestamp": time.Now(),
    },
})
```

### Conditional Streaming

```go
func process(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    streamWriter := graph.GetStreamWriter(ctx)
    verbose := s.Get("verbose").(bool)
    
    for i, item := range items {
        process(item)
        
        // Only stream if verbose mode enabled
        if verbose && streamWriter != nil {
            streamWriter(&graph.NodeResult{
                Updates: map[string]any{"processed": item},
            })
        }
    }
    
    return &graph.NodeResult{Updates: map[string]any{"status": "done"}}, nil
}
```

### Error Handling

```go
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
```

---

## See Also

- [Graph Execution](./getting-started.md#execution)
- [Node Implementation](./architecture.md#nodes)
- [State Management](./architecture.md#state)
- [Complete Example](../examples/streaming/main.go)
