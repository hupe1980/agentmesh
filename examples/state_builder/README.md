# Example: State Management

## Overview
Demonstrates state manager setup with typed keys for graph execution. Shows how to register channels for different data patterns.

## Key Concepts
- **State Manager**: Central state management with typed keys
- **Common Patterns**: Counters, flags, lists, maps
- **Type Safety**: Typed keys ensure correct value types
- **Channel Registration**: Register keys before use

## Running
```bash
cd examples/state_builder
go run main.go
```

## Expected Output
```
=== State Management Example ===

Creating state with typed keys...

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

### 1. Define Typed Keys
```go
// Define typed keys for state management
phaseKey := state.NewKey("phase", "initialization")
attemptsKey := state.NewKey("attempts", 0)
validatedKey := state.NewKey("validated", false)
actionLogKey := state.NewListKey[string]("action_log", 0)
taskResultsKey := state.NewKey("task_results", map[string]any{})

// Create state manager and register keys
mgr := state.NewManager()
state.RegisterKey(mgr, phaseKey)
state.RegisterKey(mgr, attemptsKey)
state.RegisterKey(mgr, validatedKey)
state.RegisterKey(mgr, actionLogKey.Key)
state.RegisterKey(mgr, taskResultsKey)

// Create graph with the manager
gph, err := graph.NewGraph(mgr)
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
- [pkg/state](../../pkg/state) - State management and channel types
- [examples/parallel_tasks](../parallel_tasks) - Custom channels
