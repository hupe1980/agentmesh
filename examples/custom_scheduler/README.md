# Custom Scheduler Example

This example demonstrates how to use custom schedulers to control vertex execution order within each superstep.

## Overview

Schedulers determine which vertices execute first when multiple vertices are ready in parallel. AgentMesh provides three built-in schedulers:

1. **TopologicalScheduler** (default): Lexicographic ordering for deterministic execution
2. **PriorityScheduler**: Priority-based execution (high-priority vertices first)
3. **ResourceAwareScheduler**: Resource-aware scheduling (optimize for memory/CPU)

## Running the Example

```bash
go run examples/custom_scheduler/main.go
```

## Expected Output

The example runs three parallel vertices with different schedulers:

```
1. Default TopologicalScheduler:
   ⚡ Executing: high_priority
   ⚡ Executing: low_priority
   ⚡ Executing: medium_priority
   (alphabetical order)

2. PriorityScheduler (high-priority first):
   ⚡ Executing: high_priority
   ⚡ Executing: medium_priority
   ⚡ Executing: low_priority
   (priority order: 100, 50, 10)

3. ResourceAwareScheduler (low-cost first):
   ⚡ Executing: low_priority
   ⚡ Executing: medium_priority
   ⚡ Executing: high_priority
   (cost order: 10, 50, 100)
```

## Use Cases

### TopologicalScheduler
- ✅ Debugging (reproducible execution order)
- ✅ Testing (consistent results)
- ✅ Simple workflows

### PriorityScheduler
- ✅ Critical path optimization (blocking operations first)
- ✅ Cost-based execution (expensive operations early/late)
- ✅ User-defined importance (VIP requests first)

### ResourceAwareScheduler
- ✅ Memory-constrained environments (small tasks first)
- ✅ CPU-bound workloads (distribute load evenly)
- ✅ Mixed workload optimization (I/O vs CPU separation)

## Implementation Details

The example uses a mock graph with three vertices that execute in parallel. By setting `MaxWorkers(1)`, we ensure sequential execution to clearly observe the scheduling order.

### Creating a Custom Scheduler

```go
priorities := map[string]int{
    "critical_node": 100,
    "normal_node":   50,
    "background":    10,
}
scheduler := pregel.NewPriorityScheduler(priorities, 50)

runtime, _ := pregel.NewRuntime(graph,
    pregel.WithScheduler(scheduler),
)
```

### Dynamic Priority Updates

```go
// Adjust priorities during execution
scheduler.SetPriority("urgent_task", 200)
priority := scheduler.GetPriority("urgent_task")
```

## See Also

- [Advanced Patterns Documentation](/docs/advanced.md#custom-schedulers)
- [API Reference: Scheduler interface](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/pregel#Scheduler)
- [Core Concepts: BSP Execution Model](/docs/core-concepts.md#execution-model)
