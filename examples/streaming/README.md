# Example: Streaming

## Overview
Demonstrates real-time streaming execution in AgentMesh. Shows how to get live updates during graph execution and stream LLM responses token-by-token for better user experience.

## Key Concepts
- **Stream**: Real-time event channel for graph execution
- **StreamChunk**: Token-level updates from LLM model streaming
- **Event Types**: NodeStart, NodeComplete, NodeError, GraphComplete
- **Proper Cleanup**: Always call stream.Close() or stream.Cancel()

## Prerequisites
```bash
export OPENAI_API_KEY="sk-..."
```

## Running
```bash
cd examples/streaming
go run main.go
```

## Expected Output
```
=== Graph Streaming Example ===

[Event] NodeStart: analyze
  Superstep: 1
  Timestamp: 10:30:15.123

[Streaming] analyze → "Based"
[Streaming] analyze → " on"
[Streaming] analyze → " the"
[Streaming] analyze → " data"
[Streaming] analyze → ","
[Streaming] analyze → " the"
[Streaming] analyze → " trend"
[Streaming] analyze → " is"
[Streaming] analyze → " positive"
[Streaming] analyze → "."

[Event] NodeComplete: analyze
  Duration: 1.234s
  Output: "Based on the data, the trend is positive."

[Event] NodeStart: summarize
  Superstep: 2

[Streaming] summarize → "Summary"
[Streaming] summarize → ":"
[Streaming] summarize → " Positive"
[Streaming] summarize → " trend"

[Event] NodeComplete: summarize
  Duration: 0.567s

[Event] GraphComplete
  Total Duration: 1.801s
  Supersteps: 2
  ✓ Execution successful
```

## Code Walkthrough

### 1. Build Graph with Streaming Model
```go
model := openai.NewModel(client, openai.WithStreaming(true))
```

### 2. Start Streaming Execution
```go
seq := compiled.Run(ctx, messages)
```

### 3. Process Events
```go
for event, err := range seq {
    if err != nil {
        // handle error
    }
    // process event
        fmt.Printf("→ Starting: %s\n", e.NodeName)
    
    case *graph.NodeCompleteEvent:
        fmt.Printf("✓ Completed: %s (%.2fs)\n", 
            e.NodeName, e.Duration.Seconds())
    
    case *graph.NodeErrorEvent:
        fmt.Printf("❌ Error in %s: %v\n", e.NodeName, e.Error)
    
    case *graph.GraphCompleteEvent:
        fmt.Printf("Graph completed in %.2fs\n", e.Duration.Seconds())
    }
}
```

### 4. Handle Token Streaming
```go
case *graph.NodeCompleteEvent:
    if e.StreamChunks != nil {
        for chunk := range e.StreamChunks {
            fmt.Printf("[Token] %s", chunk.Text)
        }
        fmt.Println()
    }
```

## Event Types

### NodeStartEvent
```go
type NodeStartEvent struct {
    NodeName   string
    Superstep  int
    Timestamp  time.Time
}
```

### NodeCompleteEvent
```go
type NodeCompleteEvent struct {
    NodeName     string
    Duration     time.Duration
    Updates      map[string]any
    StreamChunks <-chan model.StreamChunk  // Token stream
}
```

### NodeErrorEvent
```go
type NodeErrorEvent struct {
    NodeName string
    Error    error
    Retrying bool
}
```

### GraphCompleteEvent
```go
type GraphCompleteEvent struct {
    Duration   time.Duration
    Supersteps int
    FinalState map[string]any
}
```

## Proper Cleanup

### Context Cancellation
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

seq := compiled.Run(ctx, messages)
```

## What This Example Teaches
- ✅ Real-time execution monitoring
- ✅ Token-by-token LLM streaming
- ✅ Event-driven progress tracking
- ✅ Proper resource cleanup
- ✅ User experience optimization

## Use Cases

### Live UI Updates
```go
for event := range stream.Events() {
    updateProgressBar(event)
    showTokensInRealTime(event)
}
```

### Execution Monitoring
```go
for event := range stream.Events() {
    logToMetrics(event)
    checkTimeout(event)
}
```

### Early Termination
```go
for event := range stream.Events() {
    if shouldStop(event) {
        stream.Cancel()
        break
    }
}
```

## Next Steps
- Implement real-time UI updates
- Add streaming to chat applications
- Combine with checkpointing for long workflows
- See **examples/observability** for metrics integration

## See Also
- [pkg/graph](../../pkg/graph) - Stream API
- [pkg/model](../../pkg/model) - Model streaming interface
- [examples/observability](../observability) - Metrics and tracing
