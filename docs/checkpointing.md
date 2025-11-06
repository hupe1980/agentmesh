---
layout: doc
title: Checkpointing
permalink: /checkpointing/
nav_order: 7
---

# Checkpointing & State Persistence
{: .no_toc }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

Checkpointing in AgentMesh enables **automatic state persistence** during graph execution. Every superstep (iteration) can be saved, allowing you to:

- 🔄 **Resume** interrupted workflows from the last checkpoint
- 🐛 **Debug** production issues by replaying exact execution states
- ⏪ **Time-travel** to any previous superstep for analysis
- 📊 **Audit** agent decisions with complete execution history
- 🔀 **Branch** from checkpoints to test alternative paths

---

## Checkpoint Lifecycle

### 1. Automatic Checkpointing

When enabled, AgentMesh automatically saves state after each superstep:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/checkpoint"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Create checkpoint store
store := checkpoint.NewMemory()

// Enable automatic checkpointing
compiled, err := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),  // Save every superstep
)

// Each execution is automatically checkpointed
threadID := "workflow-123"
messages := []message.Message{
    message.NewHumanMessage("Start workflow"),
}

result, err := compiled.Invoke(ctx, messages, 
    graph.WithThreadID(threadID),
)
```

### 2. Checkpoint Contents

Each checkpoint captures complete execution state:

```go
type Checkpoint struct {
    RunID          string                 // Unique execution identifier
    Superstep      int64                  // Iteration number (0, 1, 2, ...)
    Timestamp      time.Time              // When checkpoint was created
    State          map[string]any         // Graph state (channels, values)
    Messages       []message.Message      // Conversation history
    CompletedNodes []string               // Nodes that finished this superstep
    PausedNodes    []string               // Nodes waiting for input
    Metadata       map[string]any         // Custom execution metadata
}
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
builder := graph.NewBuilder()

builder.Node("start", startFunc)
builder.Node("process", processFunc)
builder.Node("finish", finishFunc)

builder.Edge("start", "process")
builder.Edge("process", "finish")

// Execution produces 4 checkpoints:
// - Superstep 0: Initial state
// - Superstep 1: After "start" completes
// - Superstep 2: After "process" completes  
// - Superstep 3: After "finish" completes (final)
```

### 4. Resume from Checkpoint

Continue execution from the last saved state:

```go
store := checkpoint.NewMemory()

// First execution (gets interrupted)
compiled.Invoke(ctx, messages,
    graph.WithThreadID("workflow-123"),
    graph.WithCheckpointStore(store),
)

// Later: Resume from last checkpoint
messages, err := compiled.InvokeFromCheckpoint(ctx, "workflow-123")
if err != nil {
    log.Fatal(err)
}
// Continues from where it left off
```

---

## Storage Backends

AgentMesh supports three checkpoint storage backends with different trade-offs:

### Memory (Development)

**Best for**: Testing, development, short-lived workflows

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

store := checkpoint.NewMemory()

compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),
)
```

**Characteristics**:
- ✅ Zero setup - works out of the box
- ✅ Fastest performance
- ✅ Perfect for unit tests
- ⚠️ Data lost when process exits
- ⚠️ No persistence across restarts
- ⚠️ Limited by process memory

**Use Cases**:
- Local development
- Unit/integration tests
- Proof-of-concept implementations
- Debugging during development

**Example - Testing**:

```go
func TestGraphCheckpointing(t *testing.T) {
    store := checkpoint.NewMemory()
    compiled, _ := builder.Compile(
        graph.WithCheckpointStore(store),
        graph.WithCheckpointInterval(1),
    )
    
    // Run workflow
    _, err := compiled.Invoke(ctx, messages,
        graph.WithThreadID("test-run"),
    )
    require.NoError(t, err)
    
    // Verify checkpoint saved
    cp, err := store.Load(ctx, "test-run")
    require.NoError(t, err)
    require.NotNil(t, cp)
}
```

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
store := checkpointdb.NewCheckpointer(client,
    checkpointdb.WithTableName("agentmesh-checkpoints"),
)

// Auto-create table if needed
err := store.CreateTable(ctx)

compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),
)
```

**Characteristics**:
- ✅ Fully managed (no servers to maintain)
- ✅ Automatic scaling
- ✅ Multi-region replication available
- ✅ Built-in backup/restore
- ✅ TTL for automatic cleanup
- ⚠️ Network latency overhead
- ⚠️ AWS costs (per-request pricing)

**Table Schema**:

```
Primary Key: 
  - Partition Key: run_id (String)
  - Sort Key: superstep (Number)

Attributes:
  - timestamp (String, ISO 8601)
  - state (Binary, JSON)
  - messages (Binary, JSON)
  - completed_nodes (Binary, JSON)
  - paused_nodes (Binary, JSON)
  - metadata (Binary, JSON)
```

**IAM Permissions Required**:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "dynamodb:PutItem",
        "dynamodb:GetItem",
        "dynamodb:Query",
        "dynamodb:DeleteItem",
        "dynamodb:DescribeTable",
        "dynamodb:CreateTable"
      ],
      "Resource": "arn:aws:dynamodb:*:*:table/agentmesh-checkpoints"
    }
  ]
}
```

**Production Configuration**:

```go
// With TTL for automatic cleanup
store := checkpointdb.NewCheckpointer(client,
    checkpointdb.WithTableName("agentmesh-checkpoints"),
)

// Enable TTL (7 days) via AWS Console or CLI:
// aws dynamodb update-time-to-live \
//   --table-name agentmesh-checkpoints \
//   --time-to-live-specification "Enabled=true, AttributeName=ttl"

// Add TTL to checkpoints
metadata := map[string]any{
    "ttl": time.Now().Add(7 * 24 * time.Hour).Unix(),
}
compiled.Invoke(ctx, messages,
    graph.WithThreadID(threadID),
    graph.WithMetadata(metadata),
)
```

### SQL (On-Premise Production)

**Best for**: Self-hosted deployments, existing SQL infrastructure

```go
import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"  // or postgres, mysql
    checkpointsql "github.com/hupe1980/agentmesh/pkg/checkpoint/sql"
)

// SQLite
db, _ := sql.Open("sqlite3", "./checkpoints.db")
store, _ := checkpointsql.NewSQLiteCheckpointer(ctx, db)

// PostgreSQL
db, _ := sql.Open("postgres", "postgres://user:pass@localhost/agentmesh")
store, _ := checkpointsql.NewPostgreSQLCheckpointer(ctx, db)

// MySQL
db, _ := sql.Open("mysql", "user:pass@tcp(localhost:3306)/agentmesh")
store, _ := checkpointsql.NewMySQLCheckpointer(ctx, db)

compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),
)
```

**Characteristics**:
- ✅ Full control over data
- ✅ ACID guarantees
- ✅ Rich querying capabilities
- ✅ Existing backup strategies
- ✅ No vendor lock-in
- ⚠️ Requires database management
- ⚠️ Need to handle scaling

**Table Schema** (Auto-created):

```sql
CREATE TABLE checkpoints (
    run_id TEXT NOT NULL,
    superstep BIGINT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    state TEXT NOT NULL,           -- JSON
    messages TEXT NOT NULL,        -- JSON
    completed_nodes TEXT NOT NULL, -- JSON
    paused_nodes TEXT NOT NULL,    -- JSON
    metadata TEXT NOT NULL,        -- JSON
    PRIMARY KEY (run_id, superstep)
);

CREATE INDEX idx_checkpoints_run_id ON checkpoints(run_id);
CREATE INDEX idx_checkpoints_timestamp ON checkpoints(timestamp);
```

**Advanced Queries**:

```go
// Custom SQL queries for analytics
rows, _ := db.Query(`
    SELECT run_id, COUNT(*) as steps, MAX(timestamp) as last_update
    FROM checkpoints
    WHERE timestamp > $1
    GROUP BY run_id
    ORDER BY last_update DESC
`, time.Now().Add(-24*time.Hour))
```

### Backend Comparison

| Feature | Memory | DynamoDB | SQL |
|---------|--------|----------|-----|
| **Setup** | Zero | AWS account | Database server |
| **Persistence** | ❌ Process only | ✅ Durable | ✅ Durable |
| **Scaling** | Process memory | Automatic | Manual |
| **Cost** | Free | $$ per request | $ infrastructure |
| **Latency** | < 1ms | 10-50ms | 5-20ms |
| **Querying** | Limited | Limited | Rich SQL |
| **Multi-region** | ❌ | ✅ | Manual setup |
| **Backups** | ❌ | Built-in | Your strategy |
| **Best for** | Dev/Test | Cloud/Serverless | On-premise |

---

## Time-Travel Debugging

One of the most powerful features of checkpointing is the ability to "time-travel" to any previous state:

### Basic Time-Travel

Load and inspect any checkpoint:

```go
store := checkpoint.NewMemory()

// Run workflow with checkpointing
compiled.Invoke(ctx, messages,
    graph.WithThreadID("debug-run"),
    graph.WithCheckpointStore(store),
)

// List all checkpoints for the run
checkpoints, _ := store.List(ctx, "debug-run")
fmt.Printf("Total supersteps: %d\n", len(checkpoints))

// Inspect checkpoint at superstep 2
cp, _ := store.LoadAtSuperstep(ctx, "debug-run", 2)
fmt.Printf("State at step 2: %+v\n", cp.State)
fmt.Printf("Messages: %d\n", len(cp.Messages))
fmt.Printf("Completed nodes: %v\n", cp.CompletedNodes)
```

### Comparing Checkpoints

Analyze state evolution across supersteps:

```go
func compareCheckpoints(store checkpoint.Checkpointer, runID string, step1, step2 int64) {
    cp1, _ := store.LoadAtSuperstep(ctx, runID, step1)
    cp2, _ := store.LoadAtSuperstep(ctx, runID, step2)
    
    // Compare state changes
    for key, val1 := range cp1.State {
        if val2, ok := cp2.State[key]; ok {
            if !reflect.DeepEqual(val1, val2) {
                fmt.Printf("State '%s' changed:\n", key)
                fmt.Printf("  Step %d: %v\n", step1, val1)
                fmt.Printf("  Step %d: %v\n", step2, val2)
            }
        }
    }
    
    // Compare messages
    newMessages := len(cp2.Messages) - len(cp1.Messages)
    if newMessages > 0 {
        fmt.Printf("\n%d new messages:\n", newMessages)
        for _, msg := range cp2.Messages[len(cp1.Messages):] {
            fmt.Printf("  - %s: %s\n", msg.Type(), msg.Content())
        }
    }
}

compareCheckpoints(store, "debug-run", 1, 3)
```

### Production Debugging Pattern

Debug production issues without re-running:

```go
// 1. User reports issue with run "prod-123"
// 2. Load final checkpoint
finalCP, _ := store.Load(ctx, "prod-123")

// 3. Analyze what went wrong
fmt.Printf("Final state: %+v\n", finalCP.State)
fmt.Printf("Completed nodes: %v\n", finalCP.CompletedNodes)
fmt.Printf("Paused nodes: %v\n", finalCP.PausedNodes)

// 4. Look at intermediate steps
checkpoints, _ := store.List(ctx, "prod-123")
for _, cp := range checkpoints {
    // Find when error occurred
    if errorState, ok := cp.State["error"]; ok {
        fmt.Printf("Error detected at superstep %d: %v\n", 
            cp.Superstep, errorState)
    }
}

// 5. Examine exact message history
fmt.Println("\nMessage history:")
for i, msg := range finalCP.Messages {
    fmt.Printf("%d. [%s] %s\n", i+1, msg.Type(), msg.Content())
}
```

### Replay with Modifications

Fork from a checkpoint and try different paths:

```go
// Load checkpoint from superstep 2
cp, _ := store.LoadAtSuperstep(ctx, "original-run", 2)

// Modify state for testing
cp.State["test_mode"] = true
cp.State["threshold"] = 0.5  // Change parameter

// Create new run from this state
newThreadID := "test-fork-" + uuid.New().String()
newCheckpoint := &checkpoint.Checkpoint{
    RunID:          newThreadID,
    Superstep:      0,  // Reset to start
    Timestamp:      time.Now(),
    State:          cp.State,
    Messages:       cp.Messages,
    CompletedNodes: []string{},
    PausedNodes:    []string{},
    Metadata:       map[string]any{"forked_from": "original-run", "at_step": 2},
}

store.Save(ctx, newCheckpoint)

// Resume execution with modified state
result, _ := compiled.InvokeFromCheckpoint(ctx, newThreadID)
```

### Checkpoint Statistics

Monitor execution patterns:

```go
func analyzeExecution(store checkpoint.Checkpointer, runID string) {
    checkpoints, _ := store.List(ctx, runID)
    
    if len(checkpoints) == 0 {
        fmt.Println("No checkpoints found")
        return
    }
    
    // Execution metrics
    start := checkpoints[len(checkpoints)-1].Timestamp
    end := checkpoints[0].Timestamp
    duration := end.Sub(start)
    
    fmt.Printf("Execution Analysis for %s:\n", runID)
    fmt.Printf("  Total supersteps: %d\n", len(checkpoints))
    fmt.Printf("  Duration: %v\n", duration)
    fmt.Printf("  Avg time per step: %v\n", duration/time.Duration(len(checkpoints)))
    
    // Node execution count
    nodeCount := make(map[string]int)
    for _, cp := range checkpoints {
        for _, node := range cp.CompletedNodes {
            nodeCount[node]++
        }
    }
    
    fmt.Println("\nNode execution counts:")
    for node, count := range nodeCount {
        fmt.Printf("  %s: %d times\n", node, count)
    }
    
    // State growth
    finalState := checkpoints[0].State
    fmt.Printf("\nFinal state keys: %d\n", len(finalState))
    fmt.Printf("Message count: %d\n", len(checkpoints[0].Messages))
}
```

---

## Production Considerations

### 1. Checkpoint Interval Strategy

Balance between recovery granularity and overhead:

```go
// Fine-grained (every step) - Best for critical workflows
compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),  // Save every superstep
)

// Coarse-grained (every 5 steps) - Better performance
compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(5),  // Save every 5 supersteps
)

// Manual control - Checkpoint only at critical points
// (Don't use WithCheckpointInterval, checkpoint explicitly in nodes)
```

**Recommendations**:

| Workflow Type | Interval | Rationale |
|--------------|----------|-----------|
| Financial transactions | 1 | Must recover from any failure |
| Data processing pipelines | 5-10 | Balance recovery vs overhead |
| Long-running batch jobs | 50-100 | Minimize storage costs |
| Interactive agents | 1 | User experience critical |

### 2. Storage Space Management

Checkpoints can grow large - estimate your needs:

**Size Calculation**:
```
Checkpoint Size = 
  State (varies) +
  Messages × Avg Message Size +
  Node Lists (small) +
  Metadata (small)

Example:
  State: 10 KB
  Messages: 50 × 2 KB = 100 KB
  Other: 1 KB
  Total: ~111 KB per checkpoint

For 1000 runs with 10 supersteps each:
  1000 × 10 × 111 KB = 1.11 GB
```

**Optimization Strategies**:

```go
// 1. Limit message history in checkpoints
compiled.Invoke(ctx, messages,
    graph.WithThreadID(threadID),
    graph.WithMaxMessages(100),  // Keep only last 100 messages
)

// 2. Exclude large state values
builder.Node("process", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Don't persist large binary data
    largeData := processData()  // Use locally
    
    return &graph.NodeResult{
        Updates: map[string]any{
            "result_id": resultID,  // Store reference only
            // Don't include largeData in state
        },
    }, nil
})

// 3. Compress state data
type CompressedCheckpointer struct {
    inner checkpoint.Checkpointer
}

func (c *CompressedCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
    // Compress state before saving
    compressed := compressCheckpoint(cp)
    return c.inner.Save(ctx, compressed)
}
```

### 3. Error Handling

Robust checkpoint error handling:

```go
func executeWithCheckpointing(
    ctx context.Context,
    compiled *graph.CompiledGraph,
    threadID string,
    messages []message.Message,
) ([]message.Message, error) {
    
    // Try to resume from checkpoint first
    result, err := compiled.InvokeFromCheckpoint(ctx, threadID)
    if err != nil {
        // Check if it's a "not found" error
        if errors.Is(err, checkpoint.ErrNotFound) {
            // No checkpoint exists, start fresh
            return compiled.Invoke(ctx, messages,
                graph.WithThreadID(threadID),
            )
        }
        
        // Checkpoint exists but corrupted - decide on strategy
        log.Warnf("Checkpoint load failed: %v", err)
        
        // Option 1: Start fresh (lose progress)
        return compiled.Invoke(ctx, messages,
            graph.WithThreadID(threadID),
        )
        
        // Option 2: Try older checkpoint
        // store.LoadAtSuperstep(ctx, threadID, superstep-1)
    }
    
    return result, nil
}
```

### 4. Concurrent Execution

Handle multiple concurrent runs safely:

```go
// Use unique thread IDs per execution
threadID := fmt.Sprintf("user-%s-%s", userID, uuid.New().String())

// Safe for concurrent executions
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(n int) {
        defer wg.Done()
        
        threadID := fmt.Sprintf("concurrent-run-%d", n)
        _, err := compiled.Invoke(ctx, messages,
            graph.WithThreadID(threadID),
        )
        if err != nil {
            log.Errorf("Run %d failed: %v", n, err)
        }
    }(i)
}
wg.Wait()
```

### 5. Monitoring and Alerting

Track checkpoint health:

```go
type CheckpointMetrics struct {
    TotalCheckpoints   int64
    FailedCheckpoints  int64
    AvgCheckpointSize  int64
    AvgSaveTime        time.Duration
}

func monitorCheckpoints(store checkpoint.Checkpointer) *CheckpointMetrics {
    // Implement with your metrics system (Prometheus, CloudWatch, etc.)
    metrics := &CheckpointMetrics{}
    
    // Track save operations
    start := time.Now()
    err := store.Save(ctx, cp)
    metrics.AvgSaveTime = time.Since(start)
    
    if err != nil {
        metrics.FailedCheckpoints++
        // Alert if failure rate > 5%
        if metrics.FailedCheckpoints*100/metrics.TotalCheckpoints > 5 {
            sendAlert("High checkpoint failure rate")
        }
    }
    
    return metrics
}
```

### 6. Data Retention Policies

Define retention based on business needs:

```go
// DynamoDB with TTL
metadata := map[string]any{
    "ttl": time.Now().Add(30 * 24 * time.Hour).Unix(),  // 30 days
    "category": "production",
}

// SQL with cleanup job
func cleanupOldCheckpoints(db *sql.DB, retentionDays int) error {
    _, err := db.Exec(`
        DELETE FROM checkpoints
        WHERE timestamp < ?
    `, time.Now().Add(-time.Duration(retentionDays)*24*time.Hour))
    return err
}

// Memory checkpointer with manual cleanup
func (m *Memory) CleanupOld(maxAge time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    cutoff := time.Now().Add(-maxAge)
    for runID, checkpoints := range m.checkpoints {
        filtered := make([]*checkpoint.Checkpoint, 0)
        for _, cp := range checkpoints {
            if cp.Timestamp.After(cutoff) {
                filtered = append(filtered, cp)
            }
        }
        m.checkpoints[runID] = filtered
    }
}
```

---

## Checkpoint Cleanup Strategies

### Strategy 1: Time-Based Cleanup (Recommended)

Automatic cleanup based on age:

```go
// SQL: Scheduled cleanup job
func scheduleCleanup(db *sql.DB) {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    
    for range ticker.C {
        // Keep only last 30 days
        result, err := db.Exec(`
            DELETE FROM checkpoints
            WHERE timestamp < ?
        `, time.Now().Add(-30*24*time.Hour))
        
        if err != nil {
            log.Errorf("Cleanup failed: %v", err)
            continue
        }
        
        rows, _ := result.RowsAffected()
        log.Infof("Cleaned up %d old checkpoints", rows)
    }
}
```

### Strategy 2: Keep-N-Latest

Retain only recent checkpoints per run:

```go
func cleanupKeepLatest(store checkpoint.Checkpointer, runID string, keepCount int) error {
    checkpoints, err := store.List(ctx, runID)
    if err != nil {
        return err
    }
    
    // Sort by superstep (newest first)
    sort.Slice(checkpoints, func(i, j int) bool {
        return checkpoints[i].Superstep > checkpoints[j].Superstep
    })
    
    // Delete older checkpoints
    if len(checkpoints) > keepCount {
        for _, cp := range checkpoints[keepCount:] {
            // Would need custom Delete method for individual checkpoints
            log.Printf("Would delete checkpoint at superstep %d", cp.Superstep)
        }
    }
    
    return nil
}
```

### Strategy 3: Size-Based Cleanup

Clean up when storage exceeds threshold:

```go
func cleanupBySize(db *sql.DB, maxSizeGB float64) error {
    // Check current size
    var sizeGB float64
    err := db.QueryRow(`
        SELECT SUM(LENGTH(state) + LENGTH(messages)) / 1024.0 / 1024.0 / 1024.0
        FROM checkpoints
    `).Scan(&sizeGB)
    
    if err != nil || sizeGB < maxSizeGB {
        return err
    }
    
    // Delete oldest 10% if over limit
    _, err = db.Exec(`
        DELETE FROM checkpoints
        WHERE (run_id, superstep) IN (
            SELECT run_id, superstep
            FROM checkpoints
            ORDER BY timestamp ASC
            LIMIT (SELECT COUNT(*) / 10 FROM checkpoints)
        )
    `)
    
    return err
}
```

### Strategy 4: Selective Retention

Keep different retention for different priorities:

```go
func selectiveCleanup(db *sql.DB) error {
    // Keep critical runs longer
    _, err := db.Exec(`
        DELETE FROM checkpoints
        WHERE timestamp < ?
        AND metadata NOT LIKE '%"priority":"high"%'
    `, time.Now().Add(-7*24*time.Hour))  // 7 days for normal
    
    // High priority: 90 days
    _, err = db.Exec(`
        DELETE FROM checkpoints
        WHERE timestamp < ?
        AND metadata LIKE '%"priority":"high"%'
    `, time.Now().Add(-90*24*time.Hour))
    
    return err
}
```

### Strategy 5: Archival Before Deletion

Move old checkpoints to cold storage:

```go
func archiveAndCleanup(
    store checkpoint.Checkpointer,
    archiveStore checkpoint.Checkpointer,
    cutoff time.Time,
) error {
    
    // Get all checkpoints
    allRuns := getAllRunIDs(store)
    
    for _, runID := range allRuns {
        checkpoints, _ := store.List(ctx, runID)
        
        for _, cp := range checkpoints {
            if cp.Timestamp.Before(cutoff) {
                // Archive to cold storage (S3, Glacier, etc.)
                if err := archiveStore.Save(ctx, cp); err != nil {
                    log.Errorf("Archive failed for %s: %v", runID, err)
                    continue
                }
                
                // Delete from primary storage
                // (Would need individual checkpoint deletion)
            }
        }
    }
    
    return nil
}
```

---

## Best Practices

### 1. Use Meaningful Thread IDs

```go
// ✅ Good: Includes context
threadID := fmt.Sprintf("user-%s-workflow-%s-run-%s",
    userID, workflowType, timestamp)

// ❌ Bad: Hard to track
threadID := uuid.New().String()
```

### 2. Add Metadata for Tracking

```go
metadata := map[string]any{
    "user_id":      userID,
    "workflow":     "data-processing",
    "environment":  "production",
    "version":      "v2.1.0",
    "started_at":   time.Now(),
    "triggered_by": "api",
}

compiled.Invoke(ctx, messages,
    graph.WithThreadID(threadID),
    graph.WithMetadata(metadata),
)
```

### 3. Test Recovery Scenarios

```go
func TestCheckpointRecovery(t *testing.T) {
    store := checkpoint.NewMemory()
    
    // Start execution
    threadID := "test-recovery"
    _, err := compiled.Invoke(ctx, messages,
        graph.WithThreadID(threadID),
        graph.WithCheckpointStore(store),
        graph.WithMaxIterations(5),  // Partial execution
    )
    require.NoError(t, err)
    
    // Verify checkpoint exists
    cp, err := store.Load(ctx, threadID)
    require.NoError(t, err)
    require.NotNil(t, cp)
    
    // Resume execution
    result, err := compiled.InvokeFromCheckpoint(ctx, threadID)
    require.NoError(t, err)
    require.NotNil(t, result)
}
```

### 4. Monitor Checkpoint Performance

```go
func (c *InstrumentedCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
    start := time.Now()
    defer func() {
        duration := time.Since(start)
        metrics.RecordCheckpointSave(duration, err == nil)
        
        if duration > time.Second {
            log.Warnf("Slow checkpoint save: %v for run %s", duration, cp.RunID)
        }
    }()
    
    return c.inner.Save(ctx, cp)
}
```

### 5. Document Checkpoint Schema

```go
// Define expected state structure
type WorkflowState struct {
    CurrentStep  string                 `json:"current_step"`
    ProcessedIDs []string               `json:"processed_ids"`
    Results      map[string]interface{} `json:"results"`
    StartTime    time.Time              `json:"start_time"`
}

// Validate checkpoint state
func validateCheckpoint(cp *checkpoint.Checkpoint) error {
    var state WorkflowState
    data, _ := json.Marshal(cp.State)
    if err := json.Unmarshal(data, &state); err != nil {
        return fmt.Errorf("invalid state structure: %w", err)
    }
    return nil
}
```

---

## Examples

### Complete Production Setup

```go
package main

import (
    "context"
    "database/sql"
    "log"
    
    _ "github.com/lib/pq"
    "github.com/hupe1980/agentmesh/pkg/checkpoint/sql"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

func main() {
    ctx := context.Background()
    
    // Setup PostgreSQL checkpointer
    db, err := sql.Open("postgres", 
        "postgres://user:pass@localhost/agentmesh?sslmode=disable")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    
    store, err := sql.NewPostgreSQLCheckpointer(ctx, db,
        sql.WithTableName("production_checkpoints"),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // Build graph with checkpointing
    builder := graph.NewBuilder()
    // ... add nodes ...
    
    compiled, err := builder.Compile(
        graph.WithCheckpointStore(store),
        graph.WithCheckpointInterval(1),
        graph.WithMaxIterations(100),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // Execute with error handling
    threadID := "production-workflow-123"
    result, err := executeWithRetry(ctx, compiled, threadID, messages)
    if err != nil {
        log.Fatalf("Execution failed: %v", err)
    }
    
    log.Printf("Workflow completed: %d messages", len(result))
    
    // Cleanup old checkpoints (keep 30 days)
    go scheduleCleanup(db, 30)
}

func executeWithRetry(
    ctx context.Context,
    compiled *graph.CompiledGraph,
    threadID string,
    messages []message.Message,
) ([]message.Message, error) {
    
    maxRetries := 3
    for attempt := 0; attempt < maxRetries; attempt++ {
        // Try to resume from checkpoint
        result, err := compiled.InvokeFromCheckpoint(ctx, threadID)
        if err == nil {
            return result, nil
        }
        
        // If no checkpoint, start fresh
        if attempt == 0 {
            result, err := compiled.Invoke(ctx, messages,
                graph.WithThreadID(threadID),
            )
            if err == nil {
                return result, nil
            }
        }
        
        log.Printf("Attempt %d failed: %v", attempt+1, err)
    }
    
    return nil, fmt.Errorf("execution failed after %d attempts", maxRetries)
}

func scheduleCleanup(db *sql.DB, retentionDays int) {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    
    for range ticker.C {
        _, err := db.Exec(`
            DELETE FROM production_checkpoints
            WHERE timestamp < $1
        `, time.Now().Add(-time.Duration(retentionDays)*24*time.Hour))
        
        if err != nil {
            log.Printf("Cleanup error: %v", err)
        } else {
            log.Println("Checkpoint cleanup completed")
        }
    }
}
```

---

## Related Resources

- [Time-Travel Example](https://github.com/hupe1980/agentmesh/tree/main/examples/time_travel)
- [Checkpointing Example](https://github.com/hupe1980/agentmesh/tree/main/examples/checkpointing)
- [Graph Architecture](/architecture/)
- [Advanced Features](/advanced/)

---

## Next Steps

1. **Start with Memory**: Use `checkpoint.NewMemory()` for development
2. **Choose Backend**: Pick DynamoDB (cloud) or SQL (on-premise) for production
3. **Test Recovery**: Verify checkpoint/resume works with your workflow
4. **Setup Cleanup**: Implement retention policy appropriate for your use case
5. **Monitor Performance**: Track checkpoint size and save times

For implementation examples, see [examples/checkpointing](https://github.com/hupe1980/agentmesh/tree/main/examples/checkpointing) and [examples/time_travel](https://github.com/hupe1980/agentmesh/tree/main/examples/time_travel).
