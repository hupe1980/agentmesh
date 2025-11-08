---
layout: doc
title: State Management
description: State persistence, checkpointing, time travel debugging, and message retention.
permalink: /state-management/
hero:
  title: Manage workflow state
  description: Persist state with checkpointing, debug with time travel, and control message history.
  primary_cta:
    label: Enable checkpointing
    href: "#checkpointing"
  secondary_cta:
    label: View examples →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
sidebar:
  - title: Checkpointing
    url: "#checkpointing"
  - title: Storage backends
    url: "#storage-backends"
  - title: Time travel debugging
    url: "#time-travel-debugging"
  - title: Message retention
    url: "#message-retention"
  - title: Human-in-the-loop
    url: "#human-in-the-loop"
---

## Checkpointing {#checkpointing}

Checkpointing enables automatic state persistence during graph execution. Every superstep can be saved, allowing you to:

- 🔄 Resume interrupted workflows from the last checkpoint
- 🐛 Debug production issues by replaying exact execution states
- ⏪ Time-travel to any previous superstep
- 📊 Audit agent decisions with complete execution history

### Basic usage

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

// Create checkpoint store
store := checkpoint.NewMemory()

// Enable checkpointing
compiled, err := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),  // Save every superstep
)

// Execute with thread ID for persistence
results, err := compiled.Invoke(ctx, messages,
    graph.WithThreadID("workflow-123"),
)

// Resume from checkpoint after failure
results, err = compiled.InvokeFromCheckpoint(ctx, "workflow-123")
```

### Checkpoint contents

Each checkpoint captures:

```go
type Checkpoint struct {
    RunID          string                 // Unique execution ID
    Superstep      int64                  // Iteration number
    Timestamp      time.Time              // Creation time
    State          map[string]any         // Graph state
    Messages       []message.Message      // Conversation history
    CompletedNodes []string               // Finished nodes
    Metadata       map[string]any         // Custom metadata
}
```

### Checkpoint intervals

Control how often checkpoints are saved:

```go
// Save every superstep (most granular)
graph.WithCheckpointInterval(1)

// Save every 5 supersteps (balance performance/recoverability)
graph.WithCheckpointInterval(5)

// Save only at specific points (use checkpoint.Save() manually)
graph.WithCheckpointInterval(0)
```

---

## Storage backends {#storage-backends}

AgentMesh supports multiple checkpoint storage backends.

### Memory (development/testing)

In-memory storage - fast but not persistent across restarts:

```go
store := checkpoint.NewMemory()
```

**Use when:**
- Local development and testing
- Short-lived workflows
- No persistence required

### SQL (production-ready)

SQL-based storage for production use:

```go
import (
    "database/sql"
    "github.com/hupe1980/agentmesh/pkg/checkpoint"
    _ "github.com/lib/pq"  // PostgreSQL driver
)

db, err := sql.Open("postgres", connectionString)
store, err := checkpoint.NewSQL(db, checkpoint.SQLOptions{
    TableName: "agentmesh_checkpoints",
})
```

**Supported databases:**
- PostgreSQL
- MySQL
- SQLite

**Use when:**
- Production workflows
- Long-running processes
- Shared state across instances

### DynamoDB (AWS)

AWS DynamoDB for serverless architectures:

```go
import (
    "github.com/aws/aws-sdk-go/aws/session"
    "github.com/hupe1980/agentmesh/pkg/checkpoint"
)

sess := session.Must(session.NewSession())
store, err := checkpoint.NewDynamoDB(sess, checkpoint.DynamoDBOptions{
    TableName: "agentmesh-checkpoints",
})
```

**Use when:**
- AWS-based infrastructure
- Serverless deployments
- Global distribution needed

### Custom storage

Implement the `CheckpointStore` interface for custom backends:

```go
type CheckpointStore interface {
    SaveCheckpoint(ctx context.Context, threadID string, checkpoint *Checkpoint) error
    LoadCheckpoint(ctx context.Context, threadID string) (*Checkpoint, error)
    ListCheckpoints(ctx context.Context, threadID string) ([]*Checkpoint, error)
    DeleteCheckpoint(ctx context.Context, threadID string, runID string) error
}
```

---

## Time travel debugging {#time-travel-debugging}

Debug workflows by replaying from any superstep.

### List checkpoints

```go
checkpoints, err := store.ListCheckpoints(ctx, "workflow-123")

for _, cp := range checkpoints {
    fmt.Printf("Superstep %d at %v\n", cp.Superstep, cp.Timestamp)
    fmt.Printf("  Completed nodes: %v\n", cp.CompletedNodes)
}
```

### Resume from specific superstep

```go
// Resume from superstep 5
results, err := compiled.InvokeFromSuperstep(ctx, "workflow-123", 5, newMessages)
```

### Debugging workflow

1. **Identify problematic superstep** from logs or errors
2. **List checkpoints** to find the superstep before the issue
3. **Resume execution** from that checkpoint with modifications
4. **Compare results** to understand what changed

**Example:**

```go
// Original execution failed at superstep 10
// Resume from superstep 8 with debug logging enabled
ctx = context.WithValue(ctx, "debug", true)
results, err := compiled.InvokeFromSuperstep(ctx, threadID, 8, messages)
```

See `examples/time_travel` for a complete demonstration.

---

## Message retention {#message-retention}

Control conversation history to prevent context overflow and manage costs.

### Set message limits

```go
// Limit to 100 messages
state := graph.NewStateManager(100)

// Or use StateBuilder
stateBuilder := graph.NewStateBuilder().
    WithMaxMessages(50)

compiled, err := builder.Compile(
    graph.WithStateBuilder(stateBuilder),
)
```

### Pruning strategies

When limit is reached, oldest messages are removed:

```go
// Current messages: [msg1, msg2, msg3, ..., msg100]
// After adding msg101: [msg2, msg3, ..., msg100, msg101]
```

### Unlimited messages

For workflows that need full history:

```go
stateBuilder := graph.NewStateBuilder().
    WithUnlimitedMessages()
```

**When to use:**
- Short conversations (< 50 messages)
- Analysis that needs full context
- When using external message storage

**When to limit:**
- Long-running conversations
- Cost-sensitive applications (token usage)
- Fixed context window models

See `examples/message_retention` for pruning strategies.

---

## Human-in-the-loop {#human-in-the-loop}

Pause execution for human approval or input.

### Interrupt execution

```go
builder.Node("request_approval", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    return &graph.NodeResult{
        Updates: map[string]any{
            "status": "awaiting_approval",
            "data": sensitiveData,
        },
        Interrupt: true,  // Pause here
    }, nil
})
```

### Resume with input

```go
// Initial execution pauses at approval node
results, err := compiled.Invoke(ctx, messages,
    graph.WithThreadID("approval-flow"),
)

// Human reviews and provides input
// ...

// Resume execution with approval
results, err = compiled.Resume(ctx, "approval-flow", map[string]any{
    "approved": true,
    "reviewer": "alice@example.com",
})
```

### Use cases

- **Approval workflows** - Manager approval before taking action
- **Data validation** - Human verification of extracted data
- **Content review** - Review AI-generated content before publishing
- **Interactive debugging** - Pause and inspect state during development

See `examples/human_pause` for a complete workflow.

---

## Best practices

### Checkpoint management

**Do:**
- Set appropriate checkpoint intervals (balance performance vs recoverability)
- Use meaningful thread IDs (workflow-{id}, user-{id}-session-{id})
- Clean up old checkpoints periodically
- Test recovery paths regularly

**Don't:**
- Checkpoint every superstep in high-frequency workflows (performance impact)
- Store sensitive data in checkpoints without encryption
- Keep checkpoints indefinitely (storage costs)

### Message retention

**Guidelines:**
- Start with 100 messages and adjust based on needs
- Monitor token usage and adjust limits
- Consider summarization for long conversations
- Use unlimited only when necessary

### Time travel debugging

**Tips:**
- Add metadata to checkpoints for easier identification
- Use structured logging to correlate logs with supersteps
- Test time travel in development before relying on it
- Document expected superstep behavior for complex workflows

---

## Next steps

- **[Checkpointing Guide](/checkpointing/)** - Deep dive into checkpoint lifecycle
- **[Streaming](/streaming/)** - Real-time execution updates
- **[Examples](https://github.com/hupe1980/agentmesh/tree/main/examples)** - Checkpointing, time travel, and human pause examples
