# Example: Parallel Tasks

## Overview
Demonstrates parallel execution patterns in AgentMesh. Shows how to execute independent tasks concurrently for improved performance using BSP (Bulk Synchronous Parallel) execution model.

## Key Concepts
- **Parallel Execution**: Independent nodes run concurrently
- **Fan-Out Pattern**: One node → multiple parallel nodes
- **Fan-In Pattern**: Multiple nodes → one merge node
- **BinaryOpChannel**: Custom reducer for merging results
- **Concurrency Control**: WithMaxConcurrency option

## Running
```bash
cd examples/parallel_tasks
go run main.go
```

## Expected Output
```
=== Parallel Tasks Example ===

[Superstep 1] Starting 3 parallel tasks...
  task_a: Processing dataset A... (2s)
  task_b: Processing dataset B... (2s)
  task_c: Processing dataset C... (2s)
→ All tasks running in parallel

[Superstep 1 Complete] Duration: 2.1s
  (Sequential would take: 6s)

[Superstep 2] Merging results...
  Combined: {a: 100, b: 200, c: 300}

Total execution time: 2.5s
Speedup: 2.4x
```

## Code Walkthrough

### 1. Create Custom Reducer
```go
func mergeMapReducer(oldValue, newValue any) any {
    oldMap, _ := oldValue.(map[string]any)
    newMap, _ := newValue.(map[string]any)
    
    merged := make(map[string]any)
    for k, v := range oldMap {
        merged[k] = v
    }
    for k, v := range newMap {
        merged[k] = v
    }
    return merged
}
```

### 2. Configure State with Reducer
## Usage

```go
state := graph.NewStateManager(0)
state.AddChannel(state.NewTopicChannel("messages", 100))

### 3. Create Parallel Nodes
```go
g.AddNode(&graph.BaseCommandNode{
    NodeName:        "task_a",
    DeclaredTargets: []string{"merge"},
    Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
        time.Sleep(2 * time.Second) // Simulate work
        updates := map[string]any{
            "results": map[string]any{"a": 100},
        }
        return graph.Goto(updates, "merge"), nil
    },
})

// task_b and task_c are similar, each Goto("merge") with their own updates
```

### 4. Entry Point and Fan-Out
```go
// Single entry node that starts the parallel tasks
g.AddNode(&graph.BaseCommandNode{
    NodeName:        "start",
    DeclaredTargets: []string{"task_a", "task_b", "task_c"},
    Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
        // Fan-out is modeled by command targets; no manual AddEdge calls
        return graph.Goto(nil, "task_a"), nil
    },
})

g.SetEntryPoint("start")
```

### 5. Add Merge Node for Fan-In
```go
g.AddNode(&graph.BaseCommandNode{
    NodeName:        "merge",
    DeclaredTargets: []string{graph.EndNode},
    Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
        results := state.GetFromView(view, resultsKey)
        fmt.Printf("Merged results: %v\n", results)
        return graph.End(nil), nil
    },
})
```

### 6. Control Concurrency
```go
compiled, _ := builder.Compile(
    graph.WithMaxConcurrency(3), // Run 3 nodes in parallel
)
```

## Execution Flow

```
         start
        /  |  \
      /    |    \
  task_a task_b task_c  ← Superstep 1 (parallel)
      \    |    /
        \  |  /
         merge            ← Superstep 2
```

## Pregel BSP Model

### Superstep 1
- All tasks with no pending dependencies execute in parallel
- Each task updates shared state independently
- BinaryOpChannel merges concurrent updates

### Superstep 2
- Merge node waits for all parallel tasks to complete
- Processes combined results
- Continues to next stage

## Channel Types for Parallel Execution

### TopicChannel (Accumulate)
```go
state.AddChannel(state.NewTopicChannel("logs", 0))
// All parallel writes are collected in a list
```

### BinaryOpChannel (Custom Merge)
```go
state.AddChannel(channel.NewBinaryOpChannel("results", mergeFunc))
// Custom function merges concurrent updates
```

### LastValueChannel (Overwrite)
```go
state.AddChannel(channel.NewLastValueChannel("status"))
// Last write wins (not recommended for parallel tasks)
```

## What This Example Teaches
- ✅ Parallel task execution
- ✅ BSP synchronization model
- ✅ Custom result merging
- ✅ Concurrency control
- ✅ Performance optimization

## Performance Comparison

### Sequential Execution
```
task_a (2s) → task_b (2s) → task_c (2s) = 6s total
```

### Parallel Execution
```
task_a (2s) ┐
task_b (2s) ├─ Run together = 2s total
task_c (2s) ┘
```

## Production Considerations

### Concurrency Limits
```go
// Limit concurrent tasks (prevent resource exhaustion)
compiled, _ := builder.Compile(
    graph.WithMaxConcurrency(10), // Max 10 parallel nodes
)
```

### Error Handling
```go
// Parallel tasks with retry
builder.AddNodeFunc("task_a", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    result, err := performWork()
    if err != nil {
        return nil, fmt.Errorf("task_a failed: %w", err)
    }
    return result, nil
})
```

### Resource Management
```go
// Use semaphore for external resource limits
sem := make(chan struct{}, 5) // Max 5 concurrent API calls

builder.AddNodeFunc("api_call", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    sem <- struct{}{}        // Acquire
    defer func() { <-sem }() // Release
    
    return callAPI()
})
```

## Next Steps
- Implement parallel data processing pipelines
- Add error handling for parallel tasks
- Optimize concurrency limits for your workload
- See **examples/subgraph** for nested parallel execution

## See Also
- [pkg/graph](../../pkg/graph) - Graph execution model
- [pkg/state](../../pkg/state) - State management and channel types
- [examples/subgraph](../subgraph) - Complex workflows
