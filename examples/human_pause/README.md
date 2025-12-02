# Example: Human Pause

## Overview
Demonstrates human-in-the-loop workflows where execution pauses for human input before continuing. Essential for interactive workflows, approvals, and human oversight.

## Key Concepts
- **InterruptBefore**: Pause execution before a specific node
- **Checkpointing**: Save state automatically when paused
- **State Updates**: Modify checkpoint state with human input
- **WithApproval**: Bypass interrupt on resume
- **WithCheckpoint**: Resume from saved state

## Running
```bash
go run examples/human_pause/main.go
```

## Expected Output
```
=== Human Pause Example ===
  Demonstrates pausing for human input and resuming

--- First Run (will pause for input) ---
  [ask] Preparing question for human...

  ⏸️  Paused before: wait_for_answer
     Checkpoint saved automatically

  [Human provides answer: 'Paris']

--- Resuming with Human Input ---
  [ask] Preparing question for human...
  [wait_for_answer] Answer received: Paris
  [process_answer] Processing answer: Paris
  [process_answer] ✓ Correct!

  ✓ Workflow completed!

  Human pause pattern:
    1. InterruptBefore(node) - pause before a node
    2. Checkpoint saved automatically
    3. Load checkpoint, update state with human input
    4. WithApproval(node) - bypass interrupt on resume
    5. WithCheckpoint(cp) - resume from saved state
```

## Code Walkthrough

### 1. Define State Keys
```go
var (
    questionKey = graph.NewKey("question", "")
    answerKey   = graph.NewKey("answer", "")
)
```

### 2. Create Checkpointer and Graph
```go
checkpointer := checkpoint.NewInMemoryCheckpointer()
runID := "pause-run-001"

g := graph.New[any, any](questionKey, answerKey)

// Configure checkpointer
g.WithCheckpointer(checkpointer, runID)
```

### 3. Add Interrupt Before Node
```go
// Pause execution before wait_for_answer
g.InterruptBefore("wait_for_answer")
```

### 4. First Run (Pauses)
```go
for _, err := range compiled.Run(ctx, nil, graph.WithRunID(runID)) {
    if err != nil {
        var intErr *graph.InterruptError
        if errors.As(err, &intErr) {
            fmt.Printf("Paused before: %s\n", intErr.NodeName)
            break // Checkpoint saved automatically
        }
        log.Fatal(err)
    }
}
```

### 5. Resume with Human Input
```go
// Load saved checkpoint
savedCheckpoint, _ := checkpointer.Load(ctx, runID)

// Create approval to bypass interrupt
approval := &graph.ApprovalResponse{
    Decision:  graph.ApprovalApproved,
    Reason:    "Human provided input",
    Timestamp: time.Now(),
}

// Resume execution with state updates
for _, err := range compiled.Run(ctx, nil,
    graph.WithRunID(runID),
    graph.WithCheckpoint(savedCheckpoint),
    graph.WithStateUpdates(map[string]any{
        answerKey.Name(): "Paris",
    }),
    graph.WithApproval("wait_for_answer", approval),
) {
    // ...
}
```

## API Reference

### InterruptBefore
```go
g.InterruptBefore("node_name")  // Pause before this node executes
```

### WithCheckpointer
```go
g.WithCheckpointer(checkpointer, runID)  // Enable automatic checkpointing
```

### WithCheckpoint
```go
graph.WithCheckpoint(cp)  // Resume from a saved checkpoint
```

### WithStateUpdates
```go
graph.WithStateUpdates(map[string]any{  // Inject values when resuming
    "answer": "Paris",
    "approved": true,
})
```

### WithApproval
```go
graph.WithApproval("node_name", approval)  // Bypass interrupt for node
```

### ApprovalResponse
```go
approval := &graph.ApprovalResponse{
    Decision:  graph.ApprovalApproved,  // or ApprovalRejected
    Reason:    "Human approved",
    User:      "user@example.com",
    Timestamp: time.Now(),
}
```

## Workflow Patterns

### Q&A Flow
```
ask → [PAUSE: wait_for_answer] → process_answer
```

### Approval Gate
```
prepare → [PAUSE: approve] → execute
```

### Iterative Refinement
```
draft → [PAUSE: review] → revise → [PAUSE: review] → publish
```

## What This Example Teaches
- ✅ Pausing execution for human input
- ✅ Automatic checkpoint saving
- ✅ Modifying state during pause
- ✅ Resuming with approval
- ✅ Human-in-the-loop patterns

## See Also
- [examples/human_approval](../human_approval) - Approval workflows
- [examples/checkpointing](../checkpointing) - Checkpoint basics
- [pkg/checkpoint](../../pkg/checkpoint) - Checkpointer API
