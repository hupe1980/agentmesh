---
layout: doc
title: Checkpointing
description: Save and restore graph execution state for resilience and debugging.
permalink: /checkpointing/
example: checkpointing
hero:
  title: Checkpointing & State Persistence
  description: Save execution state for resumption, time-travel debugging, and audit trails.
  primary_cta:
    label: Enable checkpointing
    href: "#automatic-checkpointing"
  secondary_cta:
    label: View example →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/checkpointing"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Checkpoint Lifecycle
    url: "#checkpoint-lifecycle"
  - title: Backends
    url: "#backends"
  - title: Time Travel
    url: "#time-travel"
  - title: Security
    url: "#security"
---

## Overview {#overview}

Checkpointing in AgentMesh enables **automatic state persistence** during graph execution. Every superstep (iteration) can be saved, allowing you to:

- 🔄 **Resume** interrupted workflows from the last checkpoint
- 🐛 **Debug** production issues by replaying exact execution states
- ⏪ **Time-travel** to any previous superstep for analysis
- 📊 **Audit** agent decisions with complete execution history
- 🔀 **Branch** from checkpoints to test alternative paths

{: .important }
> **RunID is required for checkpointing.** When using checkpointing, you must provide `WithRunID()` with a stable identifier. This ensures checkpoints can be properly saved and resumed. Auto-generated UUIDs would create new checkpoint streams instead of resuming existing ones.

---

## Checkpoint Lifecycle

### 1. Automatic Checkpointing

When enabled, AgentMesh automatically saves state after each superstep:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/checkpoint"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
)

// Define state keys
var StatusKey = graph.NewKey[string]("status")

// Create graph
g := message.NewGraphBuilder(StatusKey)

g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(StatusKey, "done").To(graph.END), nil
}, graph.END)

g.Start("process")

// Create checkpointer and configure graph
checkpointer := checkpoint.NewInMemory()
runID := "workflow-123"
g.WithCheckpointer(checkpointer, runID)

// Build graph
compiled, err := g.Build()
if err != nil {
    log.Fatal(err)
}

// Execute with automatic checkpointing
messages := []message.Message{
    message.NewHumanMessageFromText("Start workflow"),
}

for _, err := range compiled.Run(ctx, messages,
    graph.WithCheckpointInterval(1),  // Save every superstep
) {
    if err != nil {
        log.Fatal(err)
    }
}
```

### 2. Checkpoint Contents

Each checkpoint captures complete execution state:

```go
type Checkpoint struct {
    RunID          string                 // Unique execution identifier
    Superstep      int64                  // Iteration number (0, 1, 2, ...)
    Timestamp      time.Time              // When checkpoint was created
    State          map[string]any         // Graph state snapshot
    PendingWrites  []PendingWrite         // Uncommitted state updates
    Committed      bool                   // Whether PendingWrites already applied
    CompletedNodes []string               // Nodes that finished execution
    PausedNodes    []string               // Nodes paused for human input
    ApprovalMetadata *ApprovalMetadata    // Pending approvals and history
    Metadata       map[string]any         // Custom execution metadata
}

type PendingWrite struct {
    NodeName  string    // Vertex that produced the write
    Channel   string    // State channel being updated
    Value     any       // Buffered value
    Timestamp time.Time // When the write was produced
}

Each pending write therefore captures **who** made the change, **what** channel it targets, and **when** it was staged. Approval dashboards surface that provenance so reviewers can make per-node decisions before the update is committed.
```

### 3. Superstep Progression

Understanding how supersteps map to execution:

```
Superstep 0: Initial state (before any node execution)
Superstep 1: After first batch of nodes completes
Superstep 2: After second batch completes
Superstep 3: After third batch completes
...
Superstep N: Final state (all nodes complete)
```

**Example Workflow**:

```go
var StatusKey = graph.NewKey[string]("status")

g := graph.New[string, string](StatusKey)

g.Node("start", startFunc, "process")
g.Node("process", processFunc, "finish")
g.Node("finish", finishFunc, graph.END)

g.Start("start")

// Execution produces 4 checkpoints:
// - Superstep 0: Initial state
// - Superstep 1: After "start" completes
// - Superstep 2: After "process" completes  
// - Superstep 3: After "finish" completes (final)
```

### 4. Resume from Checkpoint

Continue execution from the last saved state:

```go
checkpointer := checkpoint.NewInMemory()
runID := "workflow-123"

// Configure and build graph with checkpointer
g.WithCheckpointer(checkpointer, runID)
compiled, _ := g.Build()

// First execution (gets interrupted)
for _, err := range compiled.Run(ctx, messages,
    graph.WithCheckpointInterval(1),
) {
    // Handle results
}

// Later: Resume from last checkpoint
for _, err := range compiled.Resume(ctx, runID) {
    if err != nil {
        log.Fatal(err)
    }
    // Continues from where it left off
}
```

---

## Transactional Semantics

AgentMesh implements a **two-phase commit protocol** to ensure transactional consistency between checkpoints and state updates:

### How It Works

```
Phase 1: Collect updates from nodes (buffered, not applied)
Phase 2: Save checkpoint with PendingWrites → Apply updates to state
```

This guarantees that:
- ✅ **Atomicity**: State updates only applied after checkpoint is safely persisted
- ✅ **Consistency**: Checkpoint always reflects uncommitted state
- ✅ **Crash Recovery**: If crash occurs between save and apply, PendingWrites are replayed

### Crash Recovery Scenarios

**Scenario 1: Crash BEFORE checkpoint save**
```
❌ Updates never saved → Resume from previous superstep → Re-execute node
✅ No data loss, no inconsistency
```

**Scenario 2: Crash AFTER checkpoint save, BEFORE updates applied**
```
✅ Checkpoint contains PendingWrites
✅ Resume applies PendingWrites to state
✅ Continues from next superstep
```

**Scenario 3: Crash AFTER updates applied**
```
✅ Normal checkpoint resume
✅ Continue from next superstep
```

---

## Storage Backends {#backends}

AgentMesh supports multiple checkpoint storage backends:

### Memory (Development)

**Best for**: Testing, development, short-lived workflows

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

checkpointer := checkpoint.NewInMemory()

compiled, _ := g.Build(graph.WithCheckpointer(checkpointer))

for result := range compiled.Run(ctx, input,
    graph.WithRunID("test-run"),
    graph.WithCheckpointInterval(1),
) {
    // Process results
}
```

**Characteristics**:
- ✅ Zero setup - works out of the box
- ✅ Fastest performance
- ✅ Perfect for unit tests
- ⚠️ Data lost when process exits
- ⚠️ No persistence across restarts

### DynamoDB (Cloud Production)

**Best for**: AWS deployments, serverless, distributed systems

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/dynamodb"
    checkpointdb "github.com/hupe1980/agentmesh/pkg/checkpoint/dynamodb"
)

// Load AWS config
cfg, _ := config.LoadDefaultConfig(ctx)
client := dynamodb.NewFromConfig(cfg)

// Create DynamoDB checkpointer
checkpointer := checkpointdb.NewCheckpointer(client,
    checkpointdb.WithTableName("agentmesh-checkpoints"),
)

// Auto-create table if needed
err := checkpointer.CreateTable(ctx)

compiled, _ := g.Build(graph.WithCheckpointer(checkpointer))

for result := range compiled.Run(ctx, input,
    graph.WithRunID("workflow-123"),
    graph.WithCheckpointInterval(1),
) {
    // Process results
}
```

**Characteristics**:
- ✅ Fully managed (no servers to maintain)
- ✅ Automatic scaling
- ✅ Multi-region replication available
- ⚠️ Network latency overhead
- ⚠️ AWS costs (per-request pricing)

### SQL (On-Premise Production)

**Best for**: Self-hosted deployments, existing SQL infrastructure

```go
import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    checkpointsql "github.com/hupe1980/agentmesh/pkg/checkpoint/sql"
)

// SQLite
db, _ := sql.Open("sqlite3", "./checkpoints.db")
checkpointer, _ := checkpointsql.NewSQLiteCheckpointer(ctx, db)

// PostgreSQL
db, _ := sql.Open("postgres", "postgres://user:pass@localhost/agentmesh")
checkpointer, _ := checkpointsql.NewPostgreSQLCheckpointer(ctx, db)

// MySQL
db, _ := sql.Open("mysql", "user:pass@tcp(localhost:3306)/agentmesh")
checkpointer, _ := checkpointsql.NewMySQLCheckpointer(ctx, db)

compiled, _ := g.Build(graph.WithCheckpointer(checkpointer))
```

**Characteristics**:
- ✅ Works with existing databases
- ✅ Full ACID compliance
- ✅ Familiar tooling
- ⚠️ Requires database management

---

## Time Travel {#time-travel}

Debug workflows by replaying from any superstep.

### List Checkpoints

```go
checkpoints, err := checkpointer.List(ctx, "workflow-123")

for _, cp := range checkpoints {
    fmt.Printf("Superstep %d at %v\n", cp.Superstep, cp.Timestamp)
    fmt.Printf("  Completed nodes: %v\n", cp.CompletedNodes)
}
```

### Resume from Specific Checkpoint

```go
// Load a specific checkpoint and resume from it
cp, _ := checkpointer.Load(ctx, "workflow-123")

for _, err := range compiled.Resume(ctx, "workflow-123",
    graph.WithCheckpoint(cp),
) {
    // Execution resumes from superstep 5
}
```

### Debugging Workflow

1. **Identify problematic superstep** from logs or errors
2. **List checkpoints** to find the superstep before the issue
3. **Resume execution** from that checkpoint with modifications
4. **Compare results** to understand what changed

**Example**:

```go
// Original execution failed at superstep 10
// Load checkpoint and resume with debug logging enabled
cp, _ := checkpointer.Load(ctx, runID)
ctx = context.WithValue(ctx, "debug", true)

for _, err := range compiled.Resume(ctx, runID,
    graph.WithCheckpoint(cp),
) {
    if err != nil {
        log.Fatal(err)
    }
    // Debug and analyze
}
```

---

## Security {#security}

### Checkpoint Encryption

Encrypt sensitive checkpoint data:

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

// Generate or load 32-byte encryption key
key := []byte("your-32-byte-encryption-key-here")

// Create encrypted checkpointer
baseCheckpointer := checkpoint.NewInMemory()
encryptedCheckpointer := checkpoint.NewEncrypted(baseCheckpointer, key)

compiled, _ := g.Build(graph.WithCheckpointer(encryptedCheckpointer))
```

**Characteristics**:
- Uses AES-256-GCM encryption
- Key must be exactly 32 bytes
- Works with any storage backend

### Checkpoint Signing

Verify checkpoint integrity:

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

// Secret for HMAC signing
secret := []byte("your-signing-secret")

// Create signed checkpointer
baseCheckpointer := checkpoint.NewInMemory()
signedCheckpointer := checkpoint.NewSigned(baseCheckpointer, secret)

compiled, _ := g.Build(graph.WithCheckpointer(signedCheckpointer))
```

**Characteristics**:
- Uses HMAC-SHA256 for signing
- Detects tampering or corruption
- Works with any storage backend

### Combined Security

Use both encryption and signing:

```go
encryptionKey := []byte("your-32-byte-encryption-key-here")
signingSecret := []byte("your-signing-secret")

baseCheckpointer := checkpoint.NewInMemory()

// Layer 1: Encryption
encrypted := checkpoint.NewEncrypted(baseCheckpointer, encryptionKey)

// Layer 2: Signing
signedAndEncrypted := checkpoint.NewSigned(encrypted, signingSecret)

compiled, _ := g.Build(graph.WithCheckpointer(signedAndEncrypted))
```

---

## Best Practices

### Checkpoint Interval

Choose appropriate intervals based on your workload:

```go
// High-value workflows: Save every superstep
graph.WithCheckpointInterval(1)

// Performance-sensitive: Save every 5 supersteps
graph.WithCheckpointInterval(5)

// Manual control: Save only at specific points
graph.WithCheckpointInterval(0)  // Then use checkpoint.Save() manually
```

### Run ID Management

Use meaningful, stable run IDs:

```go
// ✅ Good: Meaningful identifiers
graph.WithRunID("order-processing-" + orderID)
graph.WithRunID("user-" + userID + "-session-" + sessionID)
graph.WithRunID("workflow-" + workflowType + "-" + timestamp)

// ❌ Bad: Random UUIDs (can't resume)
graph.WithRunID(uuid.New().String())
```

### Cleanup

Periodically clean up old checkpoints:

```go
// List checkpoints older than 7 days
checkpoints, _ := checkpointer.List(ctx, "")

for _, cp := range checkpoints {
    if time.Since(cp.Timestamp) > 7*24*time.Hour {
        checkpointer.Delete(ctx, cp.RunID)
    }
}
```

### Monitoring

Track checkpoint performance:

```go
// Checkpoint timing
start := time.Now()
for result := range compiled.Run(ctx, input,
    graph.WithRunID(runID),
    graph.WithCheckpointInterval(1),
) {
    if result.Checkpoint != nil {
        metrics.RecordCheckpointLatency(time.Since(start))
        start = time.Now()
    }
}

// Checkpoint size
cp, _ := checkpointer.Load(ctx, runID)
data, _ := json.Marshal(cp)
metrics.RecordCheckpointSize(len(data))
```

---

## Examples

See complete working examples:

- [checkpointing](https://github.com/hupe1980/agentmesh/tree/main/examples/checkpointing) - Basic checkpoint usage
- [checkpoint_encryption](https://github.com/hupe1980/agentmesh/tree/main/examples/checkpoint_encryption) - Encrypted checkpoints
- [checkpoint_signing](https://github.com/hupe1980/agentmesh/tree/main/examples/checkpoint_signing) - Signed checkpoints
- [time_travel](https://github.com/hupe1980/agentmesh/tree/main/examples/time_travel) - Time travel debugging
