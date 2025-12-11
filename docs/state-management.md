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
  - title: Namespaces
    url: "#namespaces"
  - title: Node-level namespaces
    url: "#node-level-namespaces"
  - title: Managed values
    url: "#managed-values"
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

AgentMesh uses a fluent command-based API for type-safe state updates. Nodes return commands that combine state updates with routing in a single expression.

### Basic pattern

All nodes use the `NodeFunc` signature with typed state keys for compile-time type safety:

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Define typed keys
var (
    CounterKey = graph.NewKey[int]("counter", 0)
    StatusKey  = graph.NewKey[string]("status", "")
)

// Create graph with keys
g := graph.New[string, string](CounterKey, StatusKey)

// Node function using commands
g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // Read current state
    counter := graph.Get(scope, CounterKey)
    
    // Return update + routing in one expression
    return graph.Set(CounterKey, counter+1).
        Set(StatusKey, "processing").
        To("next"), nil
}, "next")

g.Start("process")
compiled, _ := g.Build()
```

### Command patterns

The command API provides fluent, type-safe state updates:

```go
// Set single value and route
return graph.Set(CounterKey, 42).To("next"), nil

// Set multiple values
return graph.Set(CounterKey, 42).
    Set(StatusKey, "ready").
    To("next"), nil

// Append to list
return graph.Append(TagsKey, "new-tag").To("next"), nil

// Just route (no state changes)
return graph.To("next"), nil

// Route to END
return graph.To(graph.END), nil

// Signal failure
return graph.Fail(err)
```

### Node patterns

**Pattern 1: Single target with updates**
```go
g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(ResultKey, "processed").To("next"), nil
}, "next")
```

**Pattern 2: Multiple targets (parallel execution)**
```go
g.Node("split", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(StatusKey, "splitting").
        To("worker1", "worker2", "worker3"), nil
}, "worker1", "worker2", "worker3")
```

**Pattern 3: Conditional routing**
```go
g.Node("decide", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    score := graph.Get(scope, ScoreKey)
    
    cmd := graph.Set(ScoreKey, score+10)
    
    if score > 50 {
        return cmd.To("high_priority")
    }
    return cmd.To("normal_priority")
}, "high_priority", "normal_priority")
```

**Pattern 4: End node**
```go
g.Node("final", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(StatusKey, "complete").To(graph.END), nil
}, graph.END)
```

**Pattern 5: Read-only node**
```go
g.Node("log", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    data := graph.Get(scope, DataKey)
    fmt.Printf("Data: %v\n", data)
    return graph.To("next"), nil
}, "next")
```

### Type safety features

**Compile-time guarantees:**
- Type mismatches caught during compilation
- Typed key definitions with `graph.NewKey[T]()`
- Type-safe reads with `graph.Get(scope, TypedKey)`
- Zero runtime overhead for type checking

**Using typed keys:**
```go
// Define typed keys upfront
var (
    CounterKey  = graph.NewKey[int]("counter", 0)
    StatusKey   = graph.NewKey[string]("status", "")
    ValidKey    = graph.NewKey[bool]("valid", false)
    TagsKey     = graph.NewListKey[string]("tags")
    MessagesKey = message.MessagesKey  // Built-in message list key
)

// Use in node function
g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // ✅ Type-safe reads
    counter := graph.Get(scope, CounterKey)   // int
    status := graph.Get(scope, StatusKey)     // string
    valid := graph.Get(scope, ValidKey)       // bool
    tags := graph.GetList(scope, TagsKey)     // []string
    
    // ✅ Type-safe updates
    return graph.Set(CounterKey, counter+1).
        Set(StatusKey, "active").
        Set(ValidKey, true).
        Append(TagsKey, "new").
        To("next"), nil
}, "next")
```

See [examples/typed_updates](https://github.com/hupe1980/agentmesh/tree/main/examples/typed_updates) for a complete working example.

---

## Namespaces {#namespaces}

Namespaces provide state isolation for multi-agent systems, subgraphs, and tools. They allow different components to use the same key names without conflicts.

### Philosophy: Global First

AgentMesh follows a **global-first** approach:
- **Default:** Use simple global keys (no namespace prefix)
- **Opt-in:** Add namespaces only when you need isolation
- **Zero overhead:** Namespaces are just string prefixes (e.g., `"agent1.status"`)

### When to use namespaces

**Use namespaces when:**
- Running multiple instances of the same agent/component
- Building multi-agent systems with separate state
- Isolating subgraph state from parent graph
- Preventing key collisions between tools

**Don't use namespaces when:**
- You have a single agent
- Keys are naturally unique
- Simplicity is more important than organization

### Basic usage

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// 1. Global keys (default) - simple, no prefix
var GlobalConfig = graph.NewKey[string]("config", "")
var GlobalCounter = graph.NewKey[int]("counter", 0)

// 2. Namespaced keys - use dot notation for logical grouping
var Agent1Status = graph.NewKey[string]("agent1.status", "idle")
var Agent2Status = graph.NewKey[string]("agent2.status", "idle")

// Create graph with all keys
g := graph.New[string, string](
    GlobalConfig, GlobalCounter,
    Agent1Status, Agent2Status,
)

// Each agent updates its own namespaced key
g.Node("agent1", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(Agent1Status, "processing").To("next"), nil
}, "next")

g.Node("agent2", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(Agent2Status, "waiting").To("next"), nil
}, "next")
```

### Creating namespaces

```go
// Create namespace for logical grouping
ns := graph.NewNamespace("agent1")

// Use namespace to prefix keys
prefixedKey := ns.Prefix("status") // Returns "agent1.status"

// Create a namespaced key directly
var AgentStatus = graph.NewKey[string](ns.Prefix("status"), "idle")
}
```

**Validation rules:**
- Must start with letter or underscore
- Can contain letters, numbers, underscores
- Cannot contain dots (reserved for key separation)
- Empty string = global namespace

### Creating namespaced keys

Namespaces are implemented via key naming conventions using dot notation:

```go
// Global keys (no prefix)
var ConfigKey = graph.NewKey("config", "")
var CounterKey = graph.NewKey("counter", 0)

// Namespaced keys - use dot notation for logical grouping
var Agent1Status = graph.NewKey("agent1.status", "idle")
var Agent1Progress = graph.NewKey("agent1.progress", 0)

var Agent2Status = graph.NewKey("agent2.status", "idle")
var Agent2Progress = graph.NewKey("agent2.progress", 0)

// List keys with namespace prefix
var Agent1Results = graph.NewListKey[string]("agent1.results")
```

### Multi-agent example

```go
// Define namespaced keys for each agent
var (
    ResearcherStatus = graph.NewKey("researcher.status", "")
    WriterStatus     = graph.NewKey("writer.status", "")
    EditorStatus     = graph.NewKey("editor.status", "")
)

// Create graph with all keys
g := graph.New[string, string](
    ResearcherStatus,
    WriterStatus,
    EditorStatus,
)

// Each agent updates its own state independently
g.Node("researcher", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(ResearcherStatus, "researching").To("writer"), nil
}, "writer")

g.Node("writer", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(WriterStatus, "writing").To("editor"), nil
}, "editor")

g.Node("editor", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(EditorStatus, "editing").To(graph.END), nil
}, graph.END)

g.Start("researcher")
compiled, _ := g.Build()
```

### Best practices

**1. Package-level key constants:**
```go
// pkg/agent/researcher/keys.go
package researcher

var (
    StatusKey  = graph.NewKey("researcher.status", "idle")
    ResultsKey = graph.NewListKey[string]("researcher.results")
)
```

**2. Namespace naming conventions:**
- Use lowercase with underscores: `"agent_name"`, `"tool_1"`
- Keep names short and descriptive
- Avoid abbreviations unless well-known

**3. Documentation:**
```go
// Keys for the model execution subsystem
// Namespace prefix: "model."
// Keys:
//   - model.counter: int - Number of API calls
//   - model.status: string - Current execution status
var (
    CounterKey = graph.NewKey("model.counter", 0)
    StatusKey  = graph.NewKey("model.status", "idle")
)
```

**4. Keep namespaces simple:**
```go
// ✅ Simple prefixes work well
var ResearcherStatus = graph.NewKey("researcher.status", "")
var WriterStatus = graph.NewKey("writer.status", "")
```

See [examples/namespaces](https://github.com/hupe1980/agentmesh/tree/main/examples/namespaces) for a complete working example.

### Node-level namespace scoping {#node-level-namespaces}

For guaranteed state isolation, nodes can be scoped to operate within a specific namespace. This is ideal for multi-agent systems and pipeline stages where you want to enforce strict boundaries.

#### Creating namespaced nodes

Use `graph.WithNamespace()` to wrap node functions:

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Define keys with namespace prefixes (convention: "namespace.keyname")
var (
    validKey    = graph.NewKey("validation.is_valid", false)
    enrichedKey = graph.NewKey("enrichment.data", map[string]any(nil))
)

// Create namespaces
validationNS := graph.NewNamespace("validation")
enrichmentNS := graph.NewNamespace("enrichment")

// Create graph with all keys
g := graph.New[string, string](validKey, enrichedKey)

// Wrap node functions with WithNamespace for isolation
g.Node("validation", graph.WithNamespace(
    func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
        // This node only sees "validation.*" keys
        return graph.Set(validKey, true).To("enrichment"), nil
    }, 
    validationNS, 
    false, // includeGlobal=false
), "enrichment")

g.Node("enrichment", graph.WithNamespace(
    func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
        // This node only sees "enrichment.*" keys
        enrichedData := map[string]any{"status": "enriched"}
        return graph.Set(enrichedKey, enrichedData).To(graph.END), nil
    },
    enrichmentNS,
    false,
), graph.END)

g.Start("validation")
compiled, _ := g.Build()
```

#### With retry policies

Combine namespacing with retry policies using `graph.Compose`:

```go
retryPolicy := graph.RetryPolicy{
    MaxAttempts:    3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     time.Second,
    BackoffFactor:  2.0,
}

g.Node("processor", graph.Compose(
    func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
        // Processing logic
        return graph.Set(resultKey, "processed").To(graph.END), nil
    },
    func(fn graph.NodeFunc[string]) graph.NodeFunc[string] {
        return graph.WithRetry(fn, retryPolicy)
    },
    func(fn graph.NodeFunc[string]) graph.NodeFunc[string] {
        return graph.WithNamespace(fn, processorNS, false)
    },
), graph.END)
```

#### When to use WithNamespace

**Use `WithNamespace` when:**
- Building multi-agent systems with strict state isolation
- Creating reusable pipeline stages with clear boundaries
- You want runtime validation that nodes can't access each other's state
- Documentation should clearly show which namespace each node uses

**Use regular nodes when:**
- Single agent with naturally unique keys
- Nodes need to share state freely
- Simplicity is more important than isolation

#### How enforcement works

State isolation is enforced through **runtime scope filtering and update validation**:

1. When a `WithNamespace`-wrapped node executes, it receives a filtered scope
2. The filtered scope only exposes keys from the node's namespace prefix
3. Reading keys outside the namespace returns zero values
4. **Returned updates are validated** - attempting to update keys outside the namespace causes an error

```go
// Keys are created with namespace prefixes
var (
    agent1Status = graph.NewKey("agent1.status", "")  // "agent1.*" namespace
    agent2Status = graph.NewKey("agent2.status", "")  // "agent2.*" namespace
)

// When agent1 node executes:
// - Can read/write agent1.* keys
// - Cannot read agent2.* keys (returns zero value)
// - Cannot write agent2.* keys (returns ErrNamespaceViolation)
```

#### Update validation

`WithNamespace` validates all returned updates:

```go
ns1 := graph.NewNamespace("agent1")

g.Node("validator", graph.WithNamespace(
    func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
        // ❌ This will cause a validation error:
        return graph.Set(agent1StatusKey, "ok").      // ✅ Allowed (own namespace)
            With(graph.SetValue(agent2StatusKey, "failed")).  // ❌ ERROR: wrong namespace
            To(graph.END), nil
    },
    ns1,
    false,
), graph.END)

// Execution will fail with:
// "graph: namespace violation: attempted to update key "agent2.status" 
//  (only agent1.* keys are allowed)"
```

#### Including global keys

Set `includeGlobal=true` to allow access to non-namespaced keys:

```go
var (
    agentData  = graph.NewKey("agent.data", "")   // Namespaced
    sharedKey  = graph.NewKey("shared", "")       // Global (no dot prefix)
)

agentNS := graph.NewNamespace("agent")

// This node can access both agent.* keys AND global keys
g.Node("agent", graph.WithNamespace(agentFunc, agentNS, true), graph.END)
```

#### Best practices

**1. One namespace per agent/stage:**
```go
// ✅ Clear separation
researcherNS := graph.NewNamespace("researcher")
writerNS := graph.NewNamespace("writer")

g.Node("researcher", graph.WithNamespace(researcherFunc, researcherNS, false), "writer")
g.Node("writer", graph.WithNamespace(writerFunc, writerNS, false), graph.END)
```

**2. Use package-level namespace and keys:**
```go
// pkg/pipeline/validation/keys.go
package validation

import "github.com/hupe1980/agentmesh/pkg/graph"

var (
    NS         = graph.NewNamespace("validation")
    IsValidKey = graph.NewKey("validation.is_valid", false)
    ScoreKey   = graph.NewKey("validation.score", 0)
)
```

**3. Document namespace usage:**
```go
// ValidationNode checks input data quality
// Namespace: "validation"
// Keys: validation.is_valid (bool), validation.score (int)
g.Node("validation", graph.WithNamespace(validateFunc, validation.NS, false), targets...)
```

See [examples/namespaces](https://github.com/hupe1980/agentmesh/tree/main/examples/namespaces) for a complete working example with namespace-scoped nodes.

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
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Define keys
var StatusKey = graph.NewKey[string]("status", "")

// Create graph
g := graph.New[string, string](StatusKey)

g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(StatusKey, "done").To(graph.END), nil
}, graph.END)

g.Start("process")

// Build with checkpointer
checkpointer := checkpoint.NewInMemory()
compiled, _ := g.Build(graph.WithCheckpointer(checkpointer))

// Execute with run ID for persistence
seq := compiled.Run(ctx, "input",
    graph.WithRunID("workflow-123"),
    graph.WithCheckpointInterval(1),
    graph.WithAutoRestore(true),
)

for result := range seq {
    // Process results
}

// Resume from checkpoint after failure
seq = compiled.Run(ctx, "input",
    graph.WithRunID("workflow-123"),
    graph.WithAutoRestore(true),
)

> **Performance note:** Restores now reuse the checkpoint map directly and wrap it in a copy-on-write layer. Large checkpoints (10k+ keys) no longer trigger duplicate map allocations during resume—only mutated keys incur copies. See `BenchmarkRestoreCheckpoint10KKeys` in `pkg/graph` for reference numbers.
```

### Checkpoint contents

Each checkpoint captures:

```go
type Checkpoint struct {
    RunID          string                 // Unique execution ID
    Superstep      int64                  // Iteration number
    State          map[string]any         // Graph state snapshot
    CompletedNodes []string               // Nodes that completed execution
    PausedNodes    []string               // Nodes paused for human-in-the-loop
    ApprovalMetadata *ApprovalMetadata    // Pending approvals and history
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
checkpointer := checkpoint.NewInMemory()
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
checkpointer, err := checkpoint.NewSQL(db, checkpoint.SQLOptions{
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
checkpointer, err := checkpoint.NewDynamoDB(sess, checkpoint.DynamoDBOptions{
    TableName: "agentmesh-checkpoints",
})
```

**Use when:**
- AWS-based infrastructure
- Serverless deployments
- Global distribution needed

### Custom storage

Implement the `Checkpointer` interface for custom backends:

```go
type Checkpointer interface {
    Save(ctx context.Context, cp *Checkpoint) error
    Load(ctx context.Context, runID string) (*Checkpoint, error)
    List(ctx context.Context, runID string) ([]*Checkpoint, error)
    Delete(ctx context.Context, runID string) error
}
```

---

## Time travel debugging {#time-travel-debugging}

Debug workflows by replaying from any superstep.

### List checkpoints

```go
checkpoints, err := checkpointer.List(ctx, "workflow-123")

for _, cp := range checkpoints {
    fmt.Printf("Superstep %d at %v\n", cp.Superstep, cp.Timestamp)
    fmt.Printf("  Completed nodes: %v\n", cp.CompletedNodes)
}
```

### Resume from specific superstep

```go
// Resume from superstep 5
for result, err := range compiled.Run(ctx, newInput,
    graph.WithRunID("workflow-123"),
    graph.WithResumeFromSuperstep(5),
) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(graph.Get(result, StatusKey))
}
```

### Time Travel Debugging

```go
// Resume with modified state
for result, err := range compiled.Run(ctx, input,
    graph.WithRunID("workflow-123"),
    graph.WithResumeFromSuperstep(3),
) {
    if err != nil {
        log.Fatal(err)
    }
    // Compare with original execution
}
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
for result, err := range compiled.Run(ctx, input,
    graph.WithRunID(runID),
    graph.WithResumeFromSuperstep(8),
) {
    if err != nil {
        log.Fatal(err)
    }
    // Debug output
    fmt.Printf("Superstep completed: %v\n", result)
}
```

See `examples/time_travel` for a complete demonstration.

---

## Message retention {#message-retention}

Control conversation history to prevent context overflow and manage costs.

### Set message limits

```go
// Create message key with limit (max 50 messages)
var LimitedMessagesKey = graph.NewListKey[message.Message]("messages")

// When using MessageGraph, limit is configured at build time
g := message.NewGraphBuilder()

// Add message retention configuration
compiled, _ := g.Build(graph.WithMessageRetention(50))
```

### Pruning strategies

When limit is reached, oldest messages are removed:

```go
// Current messages: [msg1, msg2, msg3, ..., msg100]
// After adding msg101: [msg2, msg3, ..., msg100, msg101]
```

### Unlimited messages

For workflows that need full history, use 0 as the limit:

```go
// Unlimited message history (default)
compiled, _ := g.Build(graph.WithMessageRetention(0))
```

**When to use unlimited:**
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
g.Node("request_approval", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(StatusKey, "awaiting_approval").
        Set(DataKey, sensitiveData).
        Interrupt(), nil  // Pause here
}, "next")
```

### Resume with input

```go
// Initial execution pauses at approval node
seq := compiled.Run(ctx, input,
    graph.WithRunID("approval-flow"),
)

// Process events until interrupt
for result := range seq {
    if result.Interrupted {
        break
    }
}

// Human reviews and provides input
// ...

// Resume execution with updated state
seq = compiled.Run(ctx, input,
    graph.WithRunID("approval-flow"),
    graph.WithAutoRestore(true),
    graph.WithStateUpdates(map[string]any{
        "approved": true,
        "reviewer": "alice@example.com",
    }),
)
```

### Use cases

- **Approval workflows** - Manager approval before taking action
- **Data validation** - Human verification of extracted data
- **Content review** - Review AI-generated content before publishing
- **Interactive debugging** - Pause and inspect state during development

See `examples/human_pause` for a complete workflow.

---

## Approval Workflows {#approval-workflows}

Advanced human-in-the-loop pattern with conditional guards, structured responses, state edits, and audit trails. Ideal for production workflows requiring human oversight.

### Key Features

- **🛡️ Conditional Guards** - Approval only when needed (e.g., sensitive keywords detected)
- **✍️ State Edits** - Modify state during approval (e.g., redact sensitive data)
- **❌ Rejection Handling** - Gracefully handle rejected operations
- **📊 Audit Trail** - Complete approval history with timestamps and users
- **⏱️ Timeouts** - Configurable approval timeouts
- **📝 Feedback Annotations** - Optionally add approval decision to message history

### Basic Approval Workflow

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// Define keys
var ContentKey = graph.NewKey[string]("content", "")
var SentKey = graph.NewKey[bool]("sent", false)

// Create graph
g := graph.New[string, bool](ContentKey, SentKey)

// Define approval guard function
approvalGuard := func(ctx context.Context, scope graph.Scope[string]) (bool, string, error) {
    content := graph.Get(scope, ContentKey)
    if containsSensitiveData(content) {
        return true, "Contains sensitive information", nil
    }
    return false, "", nil  // No approval needed
}

// Add node with approval guard
g.Node("send_email", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    content := graph.Get(scope, ContentKey)
    sendEmail(content)
    return graph.Set(SentKey, true).To(graph.END), nil
}, graph.END)

// Configure interrupt before node with guard
g.InterruptBefore("send_email",
    graph.WithApprovalGuard(approvalGuard),
    graph.WithFeedbackAnnotation(true),
    graph.WithApprovalTimeout(10 * time.Minute),
)

g.Start("send_email")

// Build with checkpointer
checkpointer := checkpoint.NewInMemory()
compiled, _ := g.Build(graph.WithCheckpointer(checkpointer))

// Step 1: Run until approval guard triggers
runID := "email-workflow-001"
for result := range compiled.Run(ctx, "Hello world",
    graph.WithRunID(runID),
    graph.WithCheckpointInterval(1),
) {
    // Execution pauses when guard returns true
}

// Step 2: Load checkpoint and review pending approval
cp, _ := checkpointer.Load(ctx, runID)
if cp.ApprovalMetadata != nil {
    for nodeName, pending := range cp.ApprovalMetadata.PendingApprovals {
        fmt.Printf("Approval needed for: %s\n", nodeName)
        fmt.Printf("Reason: %s\n", pending.Reason)
    }
}

// Step 3: Provide approval response
approval := &graph.ApprovalResponse{
    Decision:  graph.ApprovalApproved,
    Reason:    "Reviewed and approved",
    User:      "alice@example.com",
    Timestamp: time.Now(),
    Edits: map[string]any{
        ContentKey.Name(): "Redacted sensitive content",
    },
}

// Step 4: Resume with approval
for result := range compiled.Run(ctx, "",
    graph.WithCheckpoint(cp),
    graph.WithApproval("send_email", approval),
) {
    // Execution continues with approval applied
}

// Step 5: Query approval history
history, _ := checkpointer.GetApprovalHistory(ctx, runID)
for _, record := range history {
    fmt.Printf("%s: %s by %s\n", 
        record.NodeName, record.Decision, record.User)
}
```

### Approval Decisions

Four types of approval decisions:

```go
// Approve and continue
approval := &graph.ApprovalResponse{
    Decision: graph.ApprovalApproved,
    Reason:   "Looks good",
    User:     "alice@example.com",
}

// Reject and stop
rejection := &graph.ApprovalResponse{
    Decision: graph.ApprovalRejected,
    Reason:   "Policy violation",
    User:     "security@example.com",
}

// Approve with state edits
editApproval := &graph.ApprovalResponse{
    Decision: graph.ApprovalEdit,
    Reason:   "Approved with modifications",
    User:     "editor@example.com",
    Edits: map[string]any{
        ContentKey.Name(): "Modified content",
    },
}

// Skip approval (auto-approve)
skip := &graph.ApprovalResponse{
    Decision: graph.ApprovalSkip,
    Reason:   "Automated approval",
}
```

### Conditional Guards

Guards control when approval is needed:

```go
// Example: Sensitive keyword detection
sensitiveGuard := func(ctx context.Context, scope graph.Scope[string]) (bool, string, error) {
    content := graph.Get(scope, ContentKey)
    keywords := []string{"confidential", "secret", "classified"}
    
    for _, kw := range keywords {
        if strings.Contains(strings.ToLower(content), kw) {
            return true, fmt.Sprintf("Contains sensitive keyword: %s", kw), nil
        }
    }
    return false, "", nil  // Auto-continue
}

// Example: Amount threshold
amountGuard := func(ctx context.Context, scope graph.Scope[string]) (bool, string, error) {
    amount := graph.Get(scope, AmountKey)
    if amount > 10000 {
        return true, fmt.Sprintf("Amount exceeds $10k: $%.2f", amount), nil
    }
    return false, "", nil
}

// Example: Always require approval
alwaysGuard := func(ctx context.Context, scope graph.Scope[string]) (bool, string, error) {
    return true, "Manual approval required", nil
}
```

### State Edits During Approval

Modify state as part of the approval process:

```go
approval := &graph.ApprovalResponse{
    Decision: graph.ApprovalApproved,
    User:     "reviewer@example.com",
    Edits: map[string]any{
        // Redact sensitive data
        ContentKey.Name(): redactSensitiveInfo(originalContent),
        
        // Add approval metadata
        "approved_by": "reviewer@example.com",
        "approved_at": time.Now(),
        
        // Modify execution parameters
        "priority": "high",
    },
}
```

State edits are applied BEFORE the node executes, allowing the node to see the modified state.

### Approval Configuration Options

```go
g.InterruptBefore("critical_action",
    // Required: Guard function
    graph.WithApprovalGuard(guard),
    
    // Optional: Add approval decision to message history
    graph.WithFeedbackAnnotation(true),
    
    // Optional: Timeout after which approval auto-rejects
    graph.WithApprovalTimeout(30 * time.Minute),
    
    // Optional: Snapshot specific state keys for approval review
    graph.WithStateSnapshot("content", "metadata", "config"),
)
```

### Multiple Approvals

Handle multiple approval points in a single workflow:

```go
// Add approvals at different stages
g.InterruptBefore("draft", graph.WithApprovalGuard(draftGuard))
g.InterruptBefore("publish", graph.WithApprovalGuard(publishGuard))

// Provide approvals for each stage
for result := range compiled.Run(ctx, input,
    graph.WithCheckpoint(cp),
    graph.WithApproval("draft", draftApproval),
    graph.WithApproval("publish", publishApproval),
) {
    // Process
}
```

### Error Handling

```go
// Check if approval is required but not provided
if err := graph.CheckApproval(ctx, "send_email", true); err != nil {
    log.Printf("Approval required: %v", err)
}

// Create approval required error
if needsApproval {
    info := &graph.ApprovalInfo{
        NodeName:    "send_email",
        Reason:      "Sensitive content detected",
        RequestedAt: time.Now(),
    }
    return graph.NewApprovalRequiredError(info)
}

// Check error type
if graph.IsApprovalRequired(err) {
    info := graph.ApprovalInfoFromError(err)
    fmt.Printf("Approval needed: %s\n", info.Reason)
}
```

### Production Best Practices

**1. Use conditional guards to avoid unnecessary approvals:**
```go
guard := func(ctx context.Context, scope graph.ReadOnlyScope) (bool, string, error) {
    if !needsReview(scope) {
        return false, "", nil  // Auto-continue
    }
    return true, "Manual review required", nil
}
```

**2. Set appropriate timeouts:**
```go
// Short timeout for routine approvals
graph.WithApprovalTimeout(5 * time.Minute)

// Long timeout for complex reviews
graph.WithApprovalTimeout(24 * time.Hour)

// No timeout (wait indefinitely)
graph.WithApprovalTimeout(0)
```

**3. Use annotations for rich audit data:**
```go
approval := &graph.ApprovalResponse{
    Decision: graph.ApprovalApproved,
    User:     "alice@example.com",
    Annotations: map[string]any{
        "department":     "security",
        "risk_level":     "medium",
        "reviewed_by":    "Alice Smith",
        "policy_version": "2.1",
    },
}
```

See `examples/human_approval` for complete working examples with all approval scenarios.

---

## Managed values {#managed-values}

Managed values are **ephemeral runtime state** that is NOT included in checkpoints. They're ideal for:

- API keys and authentication tokens
- Session state (user context, preferences)
- Runtime metrics collectors
- Cached computed values
- Resource handles (connections, caches)

### Why use managed values?

Regular state (via `graph.Get`/`graph.Set`) is persisted to checkpoints. This is problematic for:

1. **Sensitive data** - API keys shouldn't be stored in checkpoints
2. **Runtime-only state** - Metrics, counters, and handles that don't survive restarts
3. **Computed values** - State that should be recomputed on access

### Types of managed values

#### Static managed value

Thread-safe storage for runtime configuration:

```go
// Create with initial value
var configMV = graph.NewManagedValue("config", &Config{
    APIKey:  os.Getenv("API_KEY"),
    Timeout: 30 * time.Second,
})

// Access in node - use scope which embeds ReadOnlyScope
func myNode(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    config := graph.GetManaged(ctx, scope, configMV)
    // Use config.APIKey, config.Timeout, etc.
    return graph.Set(resultKey, result).End()
}
```

#### Provider (always fresh)

Recomputed on every access:

```go
var counterMV = graph.NewManagedValueProvider("counter", func(ctx context.Context) (int64, error) {
    return atomic.AddInt64(&count, 1), nil
})
```

#### Provider with caching

Add `WithCacheTTL` to cache the computed value:

```go
// Cached: reuses value for 5 seconds, then recomputes
var cachedTimeMV = graph.NewManagedValueProvider("cached_time", func(ctx context.Context) (time.Time, error) {
    return time.Now(), nil
}, graph.WithCacheTTL(5*time.Second))

// Invalidate cache when needed
cachedTimeMV.Invalidate()
```

### Using managed values

Pass managed values when running the graph:

```go
// Define managed values
var apiKeyMV = graph.NewManagedValue("api_key", os.Getenv("API_KEY"))
var metricsMV = graph.NewManagedValueProvider("metrics", computeMetrics)

// Pass to Run
for output, err := range compiled.Run(ctx, input,
    graph.WithManagedValues(apiKeyMV, metricsMV)) {
    // ...
}
```

### Checkpoint safety

Managed values never ride along in checkpoints, but the **metadata does**. Each checkpoint now stores a list of managed value descriptors (name and required flag) so the executor can validate restores before user code runs.

```go
var runtimeConfigMV = graph.NewManagedValue(
    "runtime_config",
    &RuntimeConfig{APIKey: os.Getenv("API_KEY"), Timeout: 15 * time.Second},
    graph.WithManagedValueRequired(),          // resume fails if missing
    graph.WithManagedValueRehydrator(func(ctx context.Context) error {
        cfg, err := runtimeConfigMV.Get(ctx)
        if err != nil {
            return err
        }
        cfg.APIKey = os.Getenv("API_KEY")     // refresh secrets after restore
        return nil
    }),
)

compiled.Run(ctx, input,
    graph.WithManagedValues(runtimeConfigMV),  // must be provided on resume
)
```

- **`WithManagedValueRequired`**: Checkpoint restore aborts early if the managed value is missing, which protects nodes from nil pointers or stale config.
- **`WithManagedValueRehydrator`**: Runs after checkpoint restore and after cached providers refresh, which is ideal for rotating API keys, reopening DB connections, or syncing handles with the environment.

If you rely on `graph.WithCheckpoints`, make sure the same managed value registry is supplied when calling `Resume`. Missing required values will surface as descriptive errors before any graph nodes execute.

### Comparison with regular state

| Feature | Regular State | Managed Values |
|---------|--------------|----------------|
| Access | `graph.Get(scope, key)` | `graph.GetManaged(ctx, scope, mv)` |
| Checkpointed | ✅ Yes | ❌ No |
| Survives restart | ✅ Yes | ❌ No |
| Type-safe | ✅ Yes | ✅ Yes |
| Thread-safe | ✅ Yes | ✅ Yes |
| Sensitive data | ❌ No | ✅ Yes |
| Computed values | ❌ No | ✅ Yes |

See `examples/managed_values` for a complete working example.

---

## Best practices

### Checkpoint management

**Do:**
- Set appropriate checkpoint intervals (balance performance vs recoverability)
- Use meaningful run IDs (workflow-{id}, user-{id}-session-{id})
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
