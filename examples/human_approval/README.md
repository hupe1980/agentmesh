# Human Approval Example

This example demonstrates **Human-in-the-Loop** workflow with execution interrupts, pending writes, and resume values.

## Features Demonstrated

- ✅ **Interrupt Before Execution** - Pause before critical nodes
- ✅ **Checkpoint Review** - Inspect state at interrupt point
- ✅ **Resume with User Input** - Inject decisions when resuming
- ✅ **Context-Based Access** - Nodes access user input via context
- ✅ **Multiple Scenarios** - Approval, rejection, and editing

## APIs Used

### 1. AddInterruptBefore

```go
g.AddInterruptBefore("send_email")
```

Pauses execution **before** the node runs, creating a checkpoint with current state. The node is marked as paused and won't execute until resumed.

### 2. WithCheckpoint

```go
compiled.Run(ctx, input,
    graph.WithCheckpoint(checkpoint))
```

Resumes execution from a saved checkpoint, restoring:
- State values
- Completed nodes list
- Paused nodes list
- Pending writes (if any)

### 3. WithResumeValue

```go
compiled.Run(ctx, input,
    graph.WithCheckpoint(checkpoint),
    graph.WithResumeValue(map[string]any{
        "approved": true,
        "edited_draft": "...",
    }))
```

Injects user decisions into the resumed execution. Values are accessible in nodes via context.

### 4. ResumeValueFromContext

```go
func (n *Node) Compute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    resumeVals := graph.ResumeValueFromContext(ctx)
    if resumeVals != nil {
        if approved := resumeVals["approved"].(bool); approved {
            // Handle approval
        }
    }
    // Normal execution
}
```

Nodes access user input injected via `WithResumeValue()`.

## Workflow

### Scenario 1: User Approves with Edits

```
1. Draft email node executes
   └─> State: {draft: "...", status: "drafted"}

2. Interrupt triggers before send_email
   └─> Checkpoint created with paused node

3. User reviews checkpoint
   └─> Sees: draft content, status, paused nodes

4. User edits and approves
   └─> Decision: {approved: true, edited_draft: "..."}

5. Resume with user decision
   └─> send_email receives edited draft via context
   └─> Sends edited version

6. Final state
   └─> {sent: true, status: "sent_edited", draft: "edited version"}
```

### Scenario 2: User Rejects

```
1. Draft email node executes
   └─> State: {draft: "...", status: "drafted"}

2. Interrupt triggers before send_email
   └─> Checkpoint created

3. User rejects with reason
   └─> Decision: {approved: false, reason: "..."}

4. Resume with rejection
   └─> send_email receives rejection via context
   └─> Does not send email

5. Final state
   └─> {sent: false, status: "rejected: ..."}
```

## Running the Example

```bash
cd examples/human_approval
go run main.go
```

## Output

```
=== Human-in-the-Loop with Interrupts Example ===
This demonstrates:
  • Interrupting before critical actions
  • User review and approval
  • Resume with user decisions
  • Handling rejection and edits

=== Scenario 1: User Approves with Edits ===

--- Step 1: Running until interrupt ---
→ Drafting email about: Quarterly Report Deadline
⏸️  Execution paused at interrupt point

--- Step 2: User reviewing draft ---

Checkpoint Info:
  Paused nodes: [send_email]
  Completed nodes: [draft_email]

Current State:
  Topic: Quarterly Report Deadline
  Draft:
Dear Team,

This is a reminder about: Quarterly Report Deadline

Best regards
  Status: drafted

--- Step 3: User editing and approving ---
User edited draft:
Dear Team,

[URGENT] This is a critical reminder about: Quarterly Report Deadline
Please submit by EOD Friday.

Best regards

--- Step 4: Resuming with approval ---
→ Resume values received from user
  ✏️  User edited draft - sending edited version
→ Sending email:
Dear Team,
...

✅ Final State:
  Sent: true
  Status: sent_edited
```

## Use Cases

### 1. **Approval Workflows**
- Legal review of generated content
- Manager approval of automated actions
- Compliance verification

### 2. **Content Editing**
- Human editing of AI-generated text
- Refinement of draft outputs
- Translation review and correction

### 3. **Rejection Handling**
- Block inappropriate actions
- Prevent errors before they occur
- Quality control gates

### 4. **Debugging & Testing**
- Inject test values during development
- Verify behavior at specific points
- A/B testing different decisions

## Key Concepts

### Two-Phase Execution

AgentMesh supports a **two-phase commit pattern** for reviewable state changes:

1. **Node Execution Phase**
   - Node computes updates
   - Interrupt detected
   - Updates stored as "pending writes"
   - Checkpoint created (updates NOT applied yet)

2. **Application Phase**
   - User reviews pending writes
   - User decides (approve/reject/edit)
   - Resume with decision
   - Updates applied (or not)

This enables:
- ✅ Review changes before they take effect
- ✅ Rollback uncommitted changes
- ✅ Audit trail of what was reviewed
- ✅ Transactional semantics

### Context-Based Injection

Resume values flow through context, not state:

```go
// ✅ Via context
resumeVals := graph.ResumeValueFromContext(ctx)

// ❌ NOT in state
// state.Get("resume_value") // Wrong!
```

**Why context?**
- Ephemeral (doesn't persist in checkpoints)
- Request-scoped (different per resume)
- Type-safe access point
- Clean separation from business state

### Interrupt vs Pause

- **Interrupt**: Declarative, configured on graph
  ```go
  g.AddInterruptBefore("node")
  g.AddInterruptAfter("node")
  ```

- **Pause**: Imperative, returned from node
  ```go
  return nil, graph.ErrHumanInterrupt
  ```

**Use interrupts for**: Predictable review points  
**Use pause for**: Dynamic conditions detected during execution

## Related Examples

- [`human_pause`](../human_pause) - Original pause API
- [`checkpointing`](../checkpointing) - Checkpoint basics
- [`time_travel`](../time_travel) - Time-travel debugging

## Documentation

- [Checkpointing Documentation](../../docs/checkpointing.md)
- [Architecture Overview](../../README.md#%EF%B8%8F-human-in-the-loop)

## Notes

- Interrupts are checked before/after node execution
- Resume values are optional - nodes work without them
- Checkpoints include metadata about what was interrupted
- Multiple resume attempts are supported
- Backward compatible - existing code continues to work
