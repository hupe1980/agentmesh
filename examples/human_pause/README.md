# Example: Human Pause

## Overview
Demonstrates human-in-the-loop workflows where execution pauses for human approval before continuing. Essential for critical decisions and compliance requirements.

## Key Concepts
- **Human Approval**: Pause execution for user input
- **Conditional Resume**: Continue based on human decision
- **State Persistence**: Checkpoint during pause
- **Workflow Control**: Human oversight of agent actions

## Running
```bash
cd examples/human_pause
go run main.go
```

## Expected Output
```
Step 1: research
  Researched 'Impact of AI on climate change'
  Summarized findings

Step 2: draft
  Generated draft report

⏸️  PAUSE: Waiting for human approval
  Current draft: "AI can significantly impact climate change..."
  
👤 Human Decision: approve | reject | edit
> approve

✓ Human approved - continuing...

Step 3: publish
  Report published successfully

Workflow complete!
```

## Code Walkthrough

### 1. Create Pause Node
```go
builder.Node("human_review", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    draft, _ := s.Get("draft").(string)
    
    fmt.Printf("⏸️  Pausing for review\n")
    fmt.Printf("Draft: %s\n", draft)
    
    // Mark as paused (checkpoint will preserve this)
    return &graph.NodeResult{
        Pause: true,
        Updates: map[string]any{
            "status": "awaiting_approval",
        },
    }, nil
})
```

### 2. Resume with Input
```go
// Later, when human provides input:
compiled.Invoke(ctx, nil,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID(runID),
    graph.WithResumeFromPause(),
    graph.WithInput(map[string]any{
        "human_input": "approved",
    }),
)
```

### 3. Process Human Decision
```go
builder.Node("process_approval", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    decision, _ := s.Get("human_input").(string)
    
    if decision == "approved" {
        return &graph.NodeResult{
            Updates: map[string]any{"status": "approved"},
        }, nil
    }
    
    return nil, fmt.Errorf("rejected by human")
})
```

## Workflow Patterns

### Approval Gate
```
research → draft → [PAUSE: human review] → publish
```

### Decision Branch
```
analyze → [PAUSE: choose strategy] → strategy_a | strategy_b
```

### Iterative Refinement
```
generate → [PAUSE: feedback] → refine → [PAUSE: feedback] → finalize
```

## What This Example Teaches
- ✅ Human-in-the-loop workflows
- ✅ Execution pause and resume
- ✅ State persistence during pause
- ✅ Conditional workflow continuation
- ✅ Human approval gates

## Production Implementation

### Web API Integration
```go
// Pause and return to user
result := compiled.Invoke(ctx, messages,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID(sessionID),
)

if result.Paused {
    // Send to user for approval
    return http.StatusAccepted, map[string]any{
        "status": "paused",
        "runID": sessionID,
        "pendingDecision": result.PendingData,
    }
}
```

### Resume from API
```go
// When user responds
func handleApproval(w http.ResponseWriter, r *http.Request) {
    runID := r.FormValue("runID")
    decision := r.FormValue("decision")
    
    compiled.Invoke(ctx, nil,
        graph.WithCheckpointer(checkpointer),
        graph.WithRunID(runID),
        graph.WithResumeFromPause(),
        graph.WithInput(map[string]any{
            "human_decision": decision,
        }),
    )
}
```

### Timeout Handling
```go
// Auto-resume after timeout
go func() {
    time.Sleep(5 * time.Minute)
    
    // Check if still paused
    ckpt, _ := checkpointer.Load(ctx, runID)
    if len(ckpt.PausedNodes) > 0 {
        // Auto-approve or reject
        resumeWithDefault(runID, "timeout")
    }
}()
```

## Next Steps
- Integrate with web UI for approvals
- Add timeout mechanisms
- Implement approval workflows
- See **examples/checkpointing** for state persistence

## See Also
- [pkg/checkpoint](../../pkg/checkpoint) - State persistence
- [pkg/graph](../../pkg/graph) - Pause/resume API
- [examples/checkpointing](../checkpointing) - Checkpoint basics
