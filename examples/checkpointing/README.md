# Example: Checkpointing

## Overview
Demonstrates checkpoint save/restore functionality for fault-tolerant workflows. Shows how to persist execution state and resume from any point.

## Key Concepts
- **Automatic Checkpointing**: Save state after each superstep
- **Checkpoint History**: View all saved checkpoints
- **State Restoration**: Resume from any checkpoint
- **Failure Recovery**: Continue execution after interruption
- **Time-Travel Debugging**: Inspect historical state

## Running
```bash
cd examples/checkpointing
go run main.go
```

## Expected Output
```
=== Checkpoint Demo ===

This demo shows:
1. Automatic checkpoint saving after each superstep
2. Viewing checkpoint history
3. Time-travel debugging (loading past states)
4. Resuming from a checkpoint (simulated failure recovery)

=== Part 1: Workflow with Automatic Checkpointing ===
Run ID: demo-workflow-123
Checkpoints will be saved automatically after every superstep

[Superstep 1] Executing node: step1
→ Checkpoint saved at superstep 1

[Superstep 2] Executing node: step2
→ Checkpoint saved at superstep 2

[Superstep 3] Executing node: step3
→ Checkpoint saved at superstep 3

=== Part 2: Checkpoint History ===
Listing all checkpoints for run ID: demo-workflow-123

Checkpoint 1:
  Superstep: 1
  Timestamp: 2025-11-07 10:30:15
  State: {"value": 10}

Checkpoint 2:
  Superstep: 2
  Timestamp: 2025-11-07 10:30:16
  State: {"value": 20}

Checkpoint 3:
  Superstep: 3
  Timestamp: 2025-11-07 10:30:17
  State: {"value": 30}

=== Part 3: Time-Travel (Load State from Superstep 2) ===
Loaded checkpoint at superstep 2
State: {"value": 20}

=== Part 4: Resume from Checkpoint (Failure Recovery) ===
Simulating workflow that failed at superstep 2
Resuming from last checkpoint...
[Superstep 3] Continuing from checkpoint
✓ Workflow completed successfully
```

## Code Walkthrough

### 1. Create Checkpointer
```go
checkpointer := checkpoint.NewInMemoryCheckpointer()
// or for production:
// checkpointer := sql.NewPostgreSQLCheckpointer(connString)
```

### 2. Configure Graph with Checkpointer
```go
g := graph.New[string, string](keys...)
g.WithCheckpointer(checkpointer, "workflow-123")
compiled, _ := g.Build()

// Run with checkpoint interval
for _, err := range compiled.Run(ctx, input,
    graph.WithCheckpointInterval(1),  // Save every superstep
) {
    if err != nil {
        log.Fatal(err)
    }
}
```

### 3. Load Checkpoint
```go
ckpt, _ := checkpointer.Load(ctx, "workflow-123")
fmt.Printf("Last checkpoint at superstep %d\n", ckpt.Superstep)
```

### 4. Resume from Checkpoint
```go
// Resume using the Resume method
for _, err := range compiled.Resume(ctx, "workflow-123") {
    if err != nil {
        log.Fatal(err)
    }
}

// Or resume with a specific checkpoint
cp, _ := checkpointer.Load(ctx, "workflow-123")
for _, err := range compiled.Resume(ctx, "workflow-123",
    graph.WithCheckpoint(cp),
) {
    if err != nil {
        log.Fatal(err)
    }
}
```

## Checkpoint Structure

Each checkpoint contains:
```go
type Checkpoint struct {
    RunID          string                 // Workflow identifier
    Superstep      int64                  // BSP superstep number
    State          map[string]any         // Full graph state (includes message history via "__messages__" key)
    CompletedNodes []string               // Nodes that completed execution (for monitoring)
    PausedNodes    []string               // Nodes paused for human-in-the-loop workflows
    Metadata       map[string]any         // Custom metadata
}
```

**Note**: Message history is stored in the state under the `__messages__` key, not as a separate field. This ensures a single source of truth for all state data.

## Storage Backends

### In-Memory (Development/Testing)
```go
checkpointer := checkpoint.NewInMemoryCheckpointer()
```

### SQL (Production)
```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint/sql"

// SQLite
checkpointer := sql.NewSQLiteCheckpointer("checkpoints.db")

// PostgreSQL
checkpointer := sql.NewPostgreSQLCheckpointer(connString)

// MySQL
checkpointer := sql.NewMySQLCheckpointer(connString)
```

### DynamoDB (AWS Production)
```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint/dynamodb"

checkpointer := dynamodb.NewCheckpointer(dynamoClient, "checkpoints-table")
```

## What This Example Teaches
- ✅ Fault-tolerant workflow execution
- ✅ Automatic state persistence
- ✅ Checkpoint history inspection
- ✅ Resuming after failures
- ✅ Time-travel debugging

## Next Steps
- Try different storage backends
- Implement custom metadata storage
- See **examples/time_travel** for advanced debugging
- See **examples/human_pause** for human-in-the-loop patterns

## See Also
- [pkg/checkpoint](../../pkg/checkpoint) - Checkpointer interface
- [pkg/checkpoint/sql](../../pkg/checkpoint/sql) - SQL implementations
- [pkg/checkpoint/dynamodb](../../pkg/checkpoint/dynamodb) - DynamoDB implementation
- [examples/time_travel](../time_travel) - Time-travel debugging
