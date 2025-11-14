# Example: State Builder

## Overview
Demonstrates StateBuilder for simplified state initialization. Shows how to use a fluent API to create graph state with common channel patterns.

## Key Concepts
- **StateBuilder**: Fluent API for state creation
- **Common Patterns**: Messages, counters, flags, lists, maps
- **Convenience Methods**: Simplified channel configuration
- **Type Safety**: Builder ensures correct channel types

## Running
```bash
cd examples/state_builder
go run main.go
```

## Expected Output
```
=== StateBuilder Example ===

Creating state with common patterns...

[init] Initializing...
  phase: initialization → processing
  attempts: 0 → 1
  action_log: [Initialized]

[process] Processing...
  attempts: 1 → 2
  validated: false → true
  action_log: [Initialized, Processed]
  task_results: {task1: done}

[finalize] Finalizing...
  phase: processing → completed
  action_log: [Initialized, Processed, Finalized]

Final State:
  phase: completed
  attempts: 2
  validated: true
  action_log: [Initialized, Processed, Finalized]
  task_results: {task1: done, final: success}
```

## Code Walkthrough

### 1. Build State with Fluent API
```go
state := graph.NewStateBuilder().
    WithMessages(50).                          // Message history (max 50)
    WithLastValue("phase", "initialization").  // Simple value
    WithCounter("attempts").                   // Integer counter
    WithFlag("validated").                     // Boolean flag
    WithList("action_log").                    // List/array
    WithMap("task_results").                   // Map/object
    Build()
```

### 2. Use in Graph
```go
gph := graph.NewGraph(state)

gph.AddNode(&graph.Node{
    Name: "init",
    RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
        return &graph.NodeResult{
            Updates: map[string]any{
                "phase": "processing",
                "attempts": 1,
                "action_log": []string{"Initialized"},
            },
        }, nil
    },
})
```

## Builder Methods

### WithMessages(max int)
```go
// Message history with retention limit
.WithMessages(100)  // Keep last 100 messages
.WithMessages(0)    // Unlimited messages
```

### WithLastValue(key string, initial any)
```go
// Simple value (last write wins)
.WithLastValue("status", "pending")
.WithLastValue("count", 0)
.WithLastValue("data", map[string]any{})
```

### WithCounter(key string)
```go
// Integer counter (atomic increment)
.WithCounter("attempts")
.WithCounter("requests")

// Update: increments by 1
Updates: map[string]any{"attempts": 1}
```

### WithFlag(key string)
```go
// Boolean flag (default: false)
.WithFlag("completed")
.WithFlag("validated")

// Update: set to true
Updates: map[string]any{"completed": true}
```

### WithList(key string)
```go
// Append-only list
.WithList("action_log")
.WithList("errors")

// Update: appends items
Updates: map[string]any{
    "action_log": []string{"item1", "item2"},
}
```

### WithMap(key string)
```go
// Key-value map (merge on update)
.WithMap("task_results")
.WithMap("metadata")

// Update: merges keys
Updates: map[string]any{
    "task_results": map[string]any{
        "task1": "done",
    },
}
```

## What This Example Teaches
- ✅ Simplified state initialization
- ✅ Fluent API patterns
- ✅ Common channel types
- ✅ Type-safe state building
- ✅ Convenience methods

### Before: Manual Channel Setup

```go
state := graph.NewStateManager(50)
state.AddChannel(channel.NewTopicChannel("messages", 50))

## Next Steps
- Use builder in your workflows
- Create custom builder extensions
- Combine with other patterns
- See **examples/parallel_tasks** for advanced state usage

## See Also
- [pkg/graph](../../pkg/graph) - StateBuilder API
- [pkg/channel](../../pkg/channel) - Channel types
- [examples/parallel_tasks](../parallel_tasks) - Custom channels
