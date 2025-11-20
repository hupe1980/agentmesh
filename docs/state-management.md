---
layout: doc
title: State Management
description: Type-safe updates, state persistence, checkpointing, time travel debugging, and message retention.
permalink: /state-management/
hero:
  title: Manage workflow state
  description: Build type-safe workflows with compile-time guarantees, persist state with checkpointing, and debug with time travel.
  primary_cta:
    label: Type-safe updates
    href: "#type-safe-updates"
  secondary_cta:
    label: View examples →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
sidebar:
  - title: Type-safe updates
    url: "#type-safe-updates"
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

## Type-safe updates {#type-safe-updates}

`UpdateBuilder` provides compile-time type safety for state updates using Go generics. This prevents runtime errors from type mismatches and typos. **All state updates must use UpdateBuilder.**

### Usage

```go
import "github.com/hupe1980/agentmesh/pkg/state"

// Define typed keys
var (
    CounterKey = state.NewKey[int]("counter")
    MessagesKey = state.NewListKey[string]("messages")
)

// ✅ Compile-time type checking
builder.Node("process", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    ub := state.NewUpdateBuilder()
    state.SetUpdate(ub, CounterKey, 42)              // ✅ Type-checked: int
    state.AppendUpdate(ub, MessagesKey, "hello")     // ✅ Type-checked: string
    
    // state.SetUpdate(ub, CounterKey, "wrong")      // ❌ Won't compile!
    // state.AppendUpdate(ub, MessagesKey, 123)      // ❌ Won't compile!
    
    updates, err := ub.Build()  // ✅ Validated (no duplicate keys)
    if err != nil {
        return nil, err
    }
    
    return &graph.NodeResult{
        Updates: updates,
    }, nil
})
```

### Key features

**Compile-time guarantees:**
- Type mismatches caught during compilation
- Typos in key names prevented
- Duplicate key detection

**Operations:**
- `SetUpdate[T]` - Set a single value with type checking
- `AppendUpdate[T]` - Append to lists with type checking
- `Delete()` - Mark keys for deletion
- `SetRaw()` - Escape hatch for dynamic scenarios

**Example:**

```go
ub := state.NewUpdateBuilder()

// Set values
state.SetUpdate(ub, state.NewKey[int]("score"), 100)
state.SetUpdate(ub, state.NewKey[string]("status"), "active")

// Append to lists
state.AppendUpdate(ub, state.NewListKey[string]("tags"), "urgent", "review")

// Delete keys
ub.Delete("old_field")

// Build validates no duplicate keys
updates, err := ub.Build()  // Returns (map[string]any, error)
if err != nil {
    return nil, err
}
```

### Complete example

```go
ub := state.NewUpdateBuilder()
state.SetUpdate(ub, CounterKey, value)
state.AppendUpdate(ub, MessagesKey, msg)
updates, err := ub.Build()
if err != nil {
    return nil, err
}
return updates, nil
```

See [examples/typed_updates](https://github.com/hupe1980/agentmesh/tree/main/examples/typed_updates) for a complete working example.

---

## Checkpointing {#checkpointing}

Checkpointing enables automatic state persistence during graph execution. Every superstep can be saved, allowing you to:

- 🔄 Resume interrupted workflows from the last checkpoint
- 🐛 Debug production issues by replaying exact execution states
- ⏪ Time-travel to any previous superstep
- 📊 Audit agent decisions with complete execution history

### Basic usage

```go
import (
    "github.com/hupe1980/agentmesh/pkg/checkpoint"
    "github.com/hupe1980/agentmesh/pkg/state"
)

// Create manager with checkpointer
mgr := state.NewManager(
    state.WithCheckpointer(checkpoint.NewInMemoryCheckpointer()),
)

// Build graph with manager
builder, err := exec.NewBuilder(exec.WithManager(mgr))

// Enable checkpointing
compiled, err := builder.Compile()

// Execute with run ID for persistence
seq := compiled.Run(ctx, messages,
    graph.WithRunID("workflow-123"),
    graph.WithCheckpointOptions(checkpoint.WithSaveInterval(1), checkpoint.WithAutoRestore(true)),
)

// Resume from checkpoint after failure
seq = compiled.Run(ctx, messages,
    graph.WithRunID("workflow-123"),
    graph.WithCheckpointOptions(checkpoint.WithAutoRestore(true)),
)
```

### Checkpoint contents

Each checkpoint captures:

```go
type Checkpoint struct {
    RunID          string                 // Unique execution ID
    Superstep      int64                  // Iteration number
    State          map[string]any         // Graph state (includes message history via "__messages__" key)
    CompletedNodes []string               // Nodes that completed execution (for monitoring)
    PausedNodes    []string               // Nodes paused for human-in-the-loop
    Metadata       map[string]any         // Custom metadata
}
```

**Note**: Message history is stored in the state under the `__messages__` key, not as a separate field.

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
checkpointer := checkpoint.NewInMemoryCheckpointer()
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
results, err := graph.Collect(compiled.Run(ctx, newMessages,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID("workflow-123"),
    graph.WithResumeFromSuperstep(5),
))
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
results, err := graph.Collect(compiled.Run(ctx, messages,
    graph.WithCheckpointer(checkpointer),
    graph.WithRunID(threadID),
    graph.WithResumeFromSuperstep(8),
))
```

See `examples/time_travel` for a complete demonstration.

---

## Message retention {#message-retention}

Control conversation history to prevent context overflow and manage costs.

### Set message limits

```go
// Create message key with limit (max 50 messages)
var MessagesKey = agent.MessagesKey  // Default: unlimited (0)

// Or create custom limited key
var LimitedMessagesKey = state.NewListKey[message.Message]("__messages__", 50)

// Register with manager
mgr := state.NewManager()
state.RegisterListKey(mgr, LimitedMessagesKey)
```

### Pruning strategies

When limit is reached, oldest messages are removed:

```go
// Current messages: [msg1, msg2, msg3, ..., msg100]
// After adding msg101: [msg2, msg3, ..., msg100, msg101]
```

### Unlimited messages

For workflows that need full history, use 0 as the max size:

```go
// Unlimited message history (default for agent.MessagesKey)
var UnlimitedMessagesKey = state.NewListKey[message.Message]("__messages__", 0)
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
builder.Node("request_approval", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
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
seq := compiled.Run(ctx, messages,
    graph.WithRunID("approval-flow"),
    graph.WithCheckpointer(checkpointer),
)

// Process events until interrupt
for event, err := range seq {
    // Handle events...
}

// Human reviews and provides input
// ...

// Apply human input to state
compiled.ApplyState(map[string]any{
    "approved": true,
    "reviewer": "alice@example.com",
}, nil)

// Resume execution
seq = compiled.Run(ctx, messages,
    graph.WithRunID("approval-flow"),
    graph.WithCheckpointer(checkpointer),
    graph.WithAutoRestore(true),
)
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
