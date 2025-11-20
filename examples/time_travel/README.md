# Example: Time Travel

## Overview
Demonstrates time-travel debugging using the checkpoint API. Shows how to inspect historical state at any superstep and compare different execution runs for debugging purposes.

## Key Concepts
- **Historical State Inspection**: View state at any superstep
- **Execution Replay**: Re-run from specific points
- **State Comparison**: Compare runs side-by-side
- **Debugging**: Trace how state evolved over time

## Running
```bash
cd examples/time_travel
go run main.go
```

## Expected Output
```
=== Time-Travel Debugging Example ===

Run 1: Starting value = 5
  [double] 5 → 10
  [add_ten] 10 → 20
  [multiply_three] 20 → 60
Final: 60

Run 2: Starting value = 10
  [double] 10 → 20
  [add_ten] 20 → 30
  [multiply_three] 30 → 90
Final: 90

=== Time-Travel Inspection ===

Run 1 History:
  Superstep 0: value = 5
  Superstep 1: value = 10   (after double)
  Superstep 2: value = 20   (after add_ten)
  Superstep 3: value = 60   (after multiply_three)

Run 2 History:
  Superstep 0: value = 10
  Superstep 1: value = 20   (after double)
  Superstep 2: value = 30   (after add_ten)
  Superstep 3: value = 90   (after multiply_three)

=== Comparing at Superstep 2 ===
  Run 1: value = 20
  Run 2: value = 30
  Difference: 10
```

## Code Walkthrough

### 1. Run with Checkpointing
```go
checkpointer := checkpoint.NewInMemoryCheckpointer()

result, _ := graph.Last(compiled.Run(ctx, nil,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID("run-1"),
    graph.WithInput(map[string]any{"value": 5}),
    graph.WithCheckpointOptions(
        checkpoint.WithSaveInterval(1), // Save every superstep
    ),
))
```

### 2. List Checkpoints
```go
checkpoints, _ := checkpointer.List(ctx, "run-1")
for _, ckpt := range checkpoints {
    fmt.Printf("Superstep %d: value = %v\n", 
        ckpt.Superstep, ckpt.State["value"])
}
```

### 3. Load Specific Superstep
```go
ckpt, _ := checkpointer.LoadAtSuperstep(ctx, "run-1", 2)
fmt.Printf("State at superstep 2: %v\n", ckpt.State)
```

### 4. Compare Runs
```go
ckpt1, _ := checkpointer.LoadAtSuperstep(ctx, "run-1", 2)
ckpt2, _ := checkpointer.LoadAtSuperstep(ctx, "run-2", 2)

val1 := ckpt1.State["value"].(int)
val2 := ckpt2.State["value"].(int)
fmt.Printf("Difference: %d\n", val2-val1)
```

## Use Cases

### Bug Investigation
```go
// Find where bug was introduced
for i := 0; i <= maxSuperstep; i++ {
    ckpt, _ := checkpointer.LoadAtSuperstep(ctx, runID, i)
    if isInvalidState(ckpt.State) {
        fmt.Printf("Bug first appeared at superstep %d\n", i)
        break
    }
}
```

### A/B Testing
```go
// Compare two algorithm runs
ckptA, _ := checkpointer.LoadAtSuperstep(ctx, "algorithm-a", final)
ckptB, _ := checkpointer.LoadAtSuperstep(ctx, "algorithm-b", final)

scoreA := ckptA.State["score"].(float64)
scoreB := ckptB.State["score"].(float64)
fmt.Printf("Winner: %s (%.2f vs %.2f)\n", 
    winner(scoreA, scoreB), scoreA, scoreB)
```

### State Evolution Analysis
```go
// Track how a value changed
checkpoints, _ := checkpointer.List(ctx, runID)
for _, ckpt := range checkpoints {
    value := ckpt.State["metric"].(float64)
    fmt.Printf("Step %d: %.2f\n", ckpt.Superstep, value)
}
```

## What This Example Teaches
- ✅ Historical state inspection
- ✅ Checkpoint-based debugging
- ✅ Execution replay
- ✅ State comparison across runs
- ✅ Root cause analysis

## Advanced Patterns

### Bisect Debugging
```go
// Binary search for bug location
left, right := 0, maxSuperstep
for left < right {
    mid := (left + right) / 2
    ckpt, _ := checkpointer.LoadAtSuperstep(ctx, runID, mid)
    
    if hasBug(ckpt.State) {
        right = mid
    } else {
        left = mid + 1
    }
}
fmt.Printf("Bug introduced at superstep %d\n", left)
```

### Diff Visualization
```go
func diffStates(ckpt1, ckpt2 *checkpoint.Checkpoint) {
    for key := range ckpt1.State {
        if ckpt1.State[key] != ckpt2.State[key] {
            fmt.Printf("  %s: %v → %v\n", 
                key, ckpt1.State[key], ckpt2.State[key])
        }
    }
}
```

## Next Steps
- Build debugging dashboards
- Create state diff visualizations
- Implement automated regression detection
- See **examples/checkpointing** for more checkpoint patterns

## See Also
- [pkg/checkpoint](../../pkg/checkpoint) - Checkpoint API
- [examples/checkpointing](../checkpointing) - Checkpoint basics
- [examples/observability](../observability) - Production monitoring
