# Human Approval Workflows

This example demonstrates **advanced approval workflows** with conditional guards, structured responses, state edits, and audit trails.

## Features Demonstrated

- ✅ **Conditional Approval Guards** - Only trigger approval when needed (e.g., sensitive keywords)
- ✅ **Structured Approval Responses** - Approve/Reject/Edit/Skip with metadata
- ✅ **State Edits** - Modify state during approval (e.g., redact sensitive data)
- ✅ **Approval History** - Complete audit trail with timestamps and users
- ✅ **Feedback Annotations** - Record approval decisions in message history
- ✅ **Multiple Decision Types** - Handle approvals, rejections, and edits
- ✅ **Automatic Resume** - State edits applied before node execution

## APIs Used

### 1. Approval Guards

```go
guard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
    content := state.GetFromView(view, contentKey)
    if containsSensitiveKeywords(content) {
        return true, "Contains sensitive data", nil
    }
    return false, "", nil  // Auto-continue
}

g.AddInterruptBefore("send_email",
    graph.WithApprovalGuard(guard),
    graph.WithFeedbackAnnotation(true),
    graph.WithApprovalTimeout(10 * time.Minute),
)
```

Guards dynamically decide if approval is needed based on current state. Return `false` to auto-continue without human intervention.

### 2. Approval Responses

```go
approval := &graph.ApprovalResponse{
    Decision:  graph.ApprovalApproved,  // APPROVED, REJECTED, EDIT, SKIP
    Reason:    "Reviewed and approved with disclaimer",
    User:      "alice@example.com",
    Timestamp: time.Now(),
    Edits: state.Updates{
        contentKey.Name(): "Redacted content",  // Optional state edits
    },
    Annotations: map[string]any{
        "department": "security",
        "risk_level": "medium",
    },
}
```

Structured approval decisions with optional state modifications and metadata.

### 3. Resume with Approval

```go
compiled.Run(ctx, messages,
    graph.WithCheckpoint(cp),
    graph.WithApproval("send_email", approval),
    graph.WithCheckpointOptions(
        checkpoint.WithCheckpointer(checkpointer),  // Required for history
    ),
)
```

Resume execution with approval decision. State edits are applied automatically before node execution.

### 4. Access Approval in Nodes

```go
sendNode := &graph.BaseNode{
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        approval := graph.ApprovalFromContext(ctx, "send_email")
        if approval == nil {
            // First execution - no approval yet
            return []string{graph.EndNode}, nil, nil
        }
        
        switch approval.Decision {
        case graph.ApprovalRejected:
            log.Printf("Rejected: %s", approval.Reason)
            return []string{graph.EndNode}, state.Updates{
                sentKey.Name(): false,
            }, nil
        case graph.ApprovalApproved:
            // State edits already applied - proceed
            content := state.GetFromView(view, contentKey)
            sendEmail(content)
            return []string{graph.EndNode}, state.Updates{
                sentKey.Name(): true,
            }, nil
        }
    },
}
```

Nodes check for approval decisions and handle them appropriately.

### 5. Query Approval History

```go
history, _ := checkpointer.GetApprovalHistory(ctx, runID)
for _, record := range history {
    fmt.Printf("Node: %s, Decision: %s, User: %s\n",
        record.NodeName, record.Decision, record.User)
    fmt.Printf("Reason: %s\n", record.Reason)
    fmt.Printf("Timestamp: %v\n", record.Timestamp)
}
```

Access complete audit trail of all approval decisions.

## Workflow

### Scenario 1: Sensitive Content - Approval Required

```
1. Draft email node executes
   └─> State: {content: "Contains security policy...", status: "drafted"}

2. Approval guard evaluates state
   └─> containsSensitiveKeywords() returns true
   └─> Reason: "Contains sensitive data"

3. Checkpoint created with pending approval
   └─> PendingApprovals: [{NodeName: "send_email", Reason: "..."}]

4. User reviews and approves with redaction
   └─> Decision: APPROVED
   └─> Edits: {content: "Redacted content"}
   └─> User: "security-reviewer@example.com"

5. Resume with approval
   └─> State edits applied automatically
   └─> send_email executes with redacted content
   └─> Approval saved to history

6. Final state
   └─> {sent: true, approval_status: "approved_with_edits"}
```

### Scenario 2: Normal Content - Auto-Continue

```
1. Draft email node executes
   └─> State: {content: "Regular update...", status: "drafted"}

2. Approval guard evaluates state
   └─> containsSensitiveKeywords() returns false
   └─> Auto-continue without human intervention

3. send_email executes immediately
   └─> No checkpoint created
   └─> No approval required

4. Final state
   └─> {sent: true, status: "sent"}
```

### Scenario 3: Rejection

```
1. Draft email node executes
   └─> State: {content: "Problematic content", status: "drafted"}

2. Approval guard triggers
   └─> Checkpoint with pending approval

3. User rejects
   └─> Decision: REJECTED
   └─> Reason: "Content violates policy"
   └─> User: "compliance@example.com"

4. Resume with rejection
   └─> send_email handles rejection gracefully
   └─> Does not send email
   └─> Approval saved to history

5. Final state
   └─> {sent: false, rejection_reason: "Content violates policy"}
```

## Running the Example

```bash
cd examples/human_approval
go run main.go
```

## Output

```
=== Approval Workflows Example ===

--- Scenario 1: Sensitive content (requires approval) ---
→ Generated: Contains security policy updates
⚠️  Approval required: Contains sensitive data

User approved with edits
Applying state edits from approval...
✅ Email sent (with edits)

--- Scenario 2: Normal content (auto-continue) ---
→ Generated: Team meeting at 3pm
✅ Auto-approved (no sensitive content)
✅ Email sent

--- Scenario 3: Rejection handling ---
→ Generated: Contains inappropriate content
⚠️  Approval required: Contains sensitive data

User rejected
❌ Sending cancelled: Content violates policy

--- Approval History ---
Run ID: run_20240115_143022
Total approvals processed: 2

Approval #1:
  Node: send_email
  Decision: APPROVED
  User: reviewer@example.com
  Reason: Reviewed and approved with disclaimer
  Timestamp: 2024-01-15 14:30:25
  State edits: 1 field(s) modified
  Annotations: map[department:security risk_level:medium]

Approval #2:
  Node: send_email
  Decision: REJECTED
  User: reviewer@example.com
  Reason: Content violates policy
  Timestamp: 2024-01-15 14:30:26
  State edits: none
  Annotations: map[reason:inappropriate]

✅ Example complete
```

## Use Cases

### 1. **Conditional Approval Workflows**
- Auto-approve routine content, require review for sensitive topics
- Threshold-based approvals (e.g., spending limits)
- Risk-based conditional gates

### 2. **Content Moderation**
- Automatic filtering with human review for edge cases
- Legal/compliance review of generated content
- Quality control with redaction capabilities

### 3. **State Correction During Approval**
- Redact sensitive information
- Fix errors in generated content
- Apply policy-compliant modifications

### 4. **Audit Trails**
- Complete history of approval decisions
- Track who approved what and when
- Compliance reporting and forensics

### 5. **Multi-Stage Approvals**
- Sequential reviews across departments
- Escalation workflows
- Consensus-based decision making

## Key Concepts

### Conditional Guards

Guards dynamically determine if approval is needed:

```go
guard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
    // Inspect current state
    content := state.GetFromView(view, contentKey)
    
    // Apply business rules
    if needsReview(content) {
        return true, "Reason for requiring approval", nil
    }
    
    // Auto-continue without approval
    return false, "", nil
}
```

**Benefits:**
- ✅ Reduces unnecessary human intervention
- ✅ Dynamic decision-making based on runtime state
- ✅ Efficient workflows (only pause when needed)
- ✅ Clear audit trail of why approval was required

### Structured Approval Responses

Four decision types with rich metadata:

1. **APPROVED**: Proceed with execution
2. **REJECTED**: Block execution with reason
3. **EDIT**: Apply state modifications before proceeding
4. **SKIP**: Skip the node entirely

Each response includes:
- Decision type
- Reason/justification
- User identifier
- Timestamp
- Optional state edits
- Custom annotations

### State Edits During Approval

Modify state during the approval process:

```go
approval := &graph.ApprovalResponse{
    Decision: graph.ApprovalApproved,
    Edits: state.Updates{
        contentKey.Name(): "Redacted content",  // Applied automatically
        statusKey.Name():  "reviewed",
    },
}
```

**Edits are applied BEFORE node execution**, so the node sees the corrected state.

### Approval History Persistence

Complete audit trail stored in checkpoints:

```go
type ApprovalMetadata struct {
    PendingApprovals map[string]*PendingApproval  // Current pending
    ApprovalHistory  []*ApprovalRecord            // Historical record
}
```

Query history for compliance, debugging, or analytics:

```go
history, _ := checkpointer.GetApprovalHistory(ctx, runID)
for _, record := range history {
    log.Printf("%s: %s by %s at %v",
        record.NodeName, record.Decision, record.User, record.Timestamp)
}
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
