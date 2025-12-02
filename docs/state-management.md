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
g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    // Read current state
    counter := graph.Get(view, CounterKey)
    
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
g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.Set(ResultKey, "processed").To("next"), nil
}, "next")
```

**Pattern 2: Multiple targets (parallel execution)**
```go
g.Node("split", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.Set(StatusKey, "splitting").
        To("worker1", "worker2", "worker3"), nil
}, "worker1", "worker2", "worker3")
```

**Pattern 3: Conditional routing**
```go
g.Node("decide", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    score := graph.Get(view, ScoreKey)
    
    cmd := graph.Set(ScoreKey, score+10)
    
    if score > 50 {
        return cmd.To("high_priority")
    }
    return cmd.To("normal_priority")
}, "high_priority", "normal_priority")
```

**Pattern 4: End node**
```go
g.Node("final", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.Set(StatusKey, "complete").To(graph.END), nil
}, graph.END)
```

**Pattern 5: Read-only node**
```go
g.Node("log", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    data := graph.Get(view, DataKey)
    fmt.Printf("Data: %v\n", data)
    return graph.To("next"), nil
}, "next")
```

### Type safety features

**Compile-time guarantees:**
- Type mismatches caught during compilation
- Typed key definitions with `graph.NewKey[T]()`
- Type-safe reads with `graph.Get(view, TypedKey)`
- Zero runtime overhead for type checking

**Using typed keys:**
```go
// Define typed keys upfront
var (
    CounterKey  = graph.NewKey[int]("counter", 0)
    StatusKey   = graph.NewKey[string]("status", "")
    ValidKey    = graph.NewKey[bool]("valid", false)
    TagsKey     = graph.NewListKey[string]("tags")
    MessagesKey = graph.MessagesKey  // Built-in message list key
)

// Use in node function
g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    // ✅ Type-safe reads
    counter := graph.Get(view, CounterKey)   // int
    status := graph.Get(view, StatusKey)     // string
    valid := graph.Get(view, ValidKey)       // bool
    tags := graph.GetList(view, TagsKey)     // []string
    
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
g.Node("agent1", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.Set(Agent1Status, "processing").To("next"), nil
}, "next")

g.Node("agent2", func(ctx context.Context, view graph.View) (*graph.Command, error) {
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

```go
// Regular keys
modelNS := state.MustNamespace("model")
counterKey := state.TypedKey[int](modelNS, "counter", 0)        // "model.counter"
statusKey := state.TypedKey[string](modelNS, "status", "idle")  // "model.status"

// List keys
toolNS := state.MustNamespace("tool")
resultsKey := state.TypedListKey[string](toolNS, "results", 100, nil)  // "tool.results"

// Global keys (no prefix)
configKey := state.TypedKey[string](state.Global, "config", "")  // "config"
```

### Namespace operations

**Get namespace view** - Filter state by namespace:

```go
g.Node("reader", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    // Get all keys in a namespace
    agent1NS := state.MustNamespace("agent1")
    agent1View := state.GetNamespaceView(view, agent1NS)
    // Returns: map[string]any{"status": "processing", "progress": 50}
    // Note: Keys are returned WITHOUT namespace prefix

    // Get global keys
    globalView := state.GetNamespaceView(view, state.Global)
    // Returns: map[string]any{"config": "production", "counter": 100}
    
    return graph.To("next"), nil
}, "next")
```

**List namespaces** - Discover active namespaces:

```go
g.Node("discover", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    namespaces := state.ListNamespaces(view)

    for _, ns := range namespaces {
        if ns.IsGlobal() {
            fmt.Println("(global)")
        } else {
            fmt.Printf("%s\n", ns.Name())
        }
    }
    // Output:
    // agent1
    // agent2
    // tool
    
    return graph.To("next"), nil
}, "next")
```

### Key introspection

```go
// Check if key is namespaced
isNS := state.IsNamespaced("agent1.status")  // true
isNS = state.IsNamespaced("config")          // false

// Parse namespaced key
ns, local := state.ParseNamespacedKey("agent1.status")
// ns = "agent1", local = "status"

ns, local = state.ParseNamespacedKey("config")
// ns = "", local = "config" (global)

// Extract namespace object
ns := state.ExtractNamespace("agent1.status")
// Returns: Namespace{name: "agent1"}
```

### Multi-agent example

```go
// Define namespaces for each agent
researcherNS := state.MustNamespace("researcher")
writerNS := state.MustNamespace("writer")
editorNS := state.MustNamespace("editor")

// Each agent has its own "status" key
researcherStatus := state.TypedKey[string](researcherNS, "status", "")
writerStatus := state.TypedKey[string](writerNS, "status", "")
editorStatus := state.TypedKey[string](editorNS, "status", "")

// Create graph with all keys
g := graph.New[string, string](
    researcherStatus,
    writerStatus,
    editorStatus,
)

// Each agent updates its own state independently
g.Node("researcher", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.Set(researcherStatus, "researching").To("writer"), nil
}, "writer")

g.Node("writer", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.Set(writerStatus, "writing").To("editor"), nil
}, "editor")

g.Node("editor", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    return graph.Set(editorStatus, "editing").To(graph.END), nil
}, graph.END)

g.Start("researcher")
compiled, _ := g.Build()
```

### Best practices

**1. Package-level namespace constants:**
```go
// pkg/agent/researcher/keys.go
package researcher

var (
    NS = state.MustNamespace("researcher")
    StatusKey = state.TypedKey[string](NS, "status", "idle")
    ResultsKey = state.TypedListKey[string](NS, "results", 100, nil)
)
```

**2. Namespace naming conventions:**
- Use lowercase with underscores: `"agent_name"`, `"tool_1"`
- Keep names short and descriptive
- Avoid abbreviations unless well-known

**3. Documentation:**
```go
// Keys for the model execution subsystem
// Namespace: "model"
// Keys:
//   - counter: int - Number of API calls
//   - status: string - Current execution status
var (
    ModelNS = state.MustNamespace("model")
    CounterKey = state.TypedKey[int](ModelNS, "counter", 0)
    StatusKey = state.TypedKey[string](ModelNS, "status", "idle")
)
```

**4. Avoid deeply nested namespaces:**
```go
// ❌ Too complex
ns := state.MustNamespace("agent.researcher.team1")

// ✅ Keep it simple
researcherNS := state.MustNamespace("researcher_team1")
```

### Limitations

- **No key deletion:** `DeleteNamespace()` is not implemented (channels cannot be deleted)
- **Copy requires registration:** Target keys must be registered before `CopyNamespace()`
- **No nested namespaces:** Only one level of hierarchy (single dot)

See [examples/namespaces](https://github.com/hupe1980/agentmesh/tree/main/examples/namespaces) for a complete working example.

### Node-level namespace scoping {#node-level-namespaces}

For guaranteed state isolation, nodes can be scoped to operate within a specific namespace. This is ideal for multi-agent systems and pipeline stages where you want to enforce strict boundaries.

#### Creating namespaced nodes

Use `g.NamespacedNode()` for namespace-scoped nodes:

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Define namespaces
validationNS := state.MustNamespace("validation")
enrichmentNS := state.MustNamespace("enrichment")

// Define keys per namespace
validKey := state.TypedKey[bool](validationNS, "is_valid", false)
enrichedKey := state.TypedKey[map[string]any](enrichmentNS, "data", nil)

// Create graph with all keys
g := graph.New[string, string](validKey, enrichedKey)

// Create namespaced nodes using fluent API
g.NamespacedNode("validation", validationNS, 
    func(ctx context.Context, view graph.View) (*graph.Command, error) {
        // This node only works with "validation.*" keys
        return graph.Set(validKey, true).To("enrichment"), nil
    }, 
    "enrichment",
)

g.NamespacedNode("enrichment", enrichmentNS,
    func(ctx context.Context, view graph.View) (*graph.Command, error) {
        // This node only works with "enrichment.*" keys
        enrichedData := map[string]any{"status": "enriched"}
        return graph.Set(enrichedKey, enrichedData).To(graph.END), nil
    },
    graph.END,
)

g.Start("validation")
compiled, _ := g.Build()
```

#### With retry policies

Namespace-scoped nodes also support retry policies:

```go
retryPolicy := graph.RetryPolicy{
    MaxAttempts:    3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     time.Second,
    BackoffFactor:  2.0,
}

g.NamespacedNodeWithRetry("processor", processorNS,
    func(ctx context.Context, view graph.View) (*graph.Command, error) {
        // Processing logic
        return graph.Set(resultKey, "processed").To(graph.END), nil
    },
    retryPolicy,
    graph.END,
)
```

#### When to use namespaced nodes

**Use `NamespacedNode` when:**
- Building multi-agent systems with strict state isolation
- Creating reusable pipeline stages with clear boundaries
- You want compile-time safety that nodes can't access each other's state
- Documentation should clearly show which namespace each node uses

**Use regular nodes when:**
- Single agent with naturally unique keys
- Nodes need to share state freely
- Simplicity is more important than isolation

#### How enforcement works

State isolation is enforced through **runtime view filtering and update validation**:

1. When a `NamespacedNode` executes, it receives a filtered view
2. The filtered view only exposes keys from the node's namespace
3. Calling `view.Keys()` returns only keys from that namespace
4. **Returned updates are validated** - attempting to update keys outside the namespace causes an error

```go
// Keys are created with namespace prefixes
agent1Status := state.TypedKey[string](agent1NS, "status", "")  // "agent1.status"
agent2Status := state.TypedKey[string](agent2NS, "status", "")  // "agent2.status"

// When agent1 node executes:
// - view.Keys() returns ["status"] (only agent1's keys, without prefix)
// - Cannot access agent2's state at all
```

#### Update validation

`NamespacedNode` validates all returned updates:

```go
g.NamespacedNode("validator", agent1NS,
    func(ctx context.Context, view graph.View) (*graph.Command, error) {
        // ❌ This will cause a validation error:
        return graph.Set(agent1StatusKey, "ok").      // ✅ Allowed (own namespace)
            Set(agent2StatusKey, "failed").           // ❌ ERROR: wrong namespace
            To(graph.END), nil
    },
    graph.END,
)

// Execution will fail with:
// "node 'validator' in namespace 'agent1' attempted to update key 
//  'agent2.status' which belongs to a different namespace"
```

#### Best practices

**1. One namespace per agent/stage:**
```go
// ✅ Clear separation
researcherNS := state.MustNamespace("researcher")
writerNS := state.MustNamespace("writer")

g.NamespacedNode("researcher", researcherNS, researcherFunc, "writer")
g.NamespacedNode("writer", writerNS, writerFunc, graph.END)
```

**2. Use package-level namespace and keys:**
```go
// pkg/pipeline/validation/node.go
package validation

var (
    NS = state.MustNamespace("validation")
    IsValidKey = state.TypedKey[bool](NS, "is_valid", false)
    ScoreKey = state.TypedKey[int](NS, "score", 0)
)
```

**3. Document namespace usage:**
```go
// ValidationNode checks input data quality
// Namespace: "validation"
// Keys: is_valid (bool), score (int)
g.NamespacedNode("validation", validation.NS, validateFunc, targets...)
```

See [examples/subgraph](https://github.com/hupe1980/agentmesh/tree/main/examples/subgraph) for a complete working example with namespaced pipeline stages.

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

g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
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
results, err := graph.Collect(compiled.Run(ctx, newInput,
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
results, err := graph.Collect(compiled.Run(ctx, input,
    graph.WithRunID(runID),
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
var LimitedMessagesKey = graph.NewListKey[message.Message]("messages")

// When using MessageGraph, limit is configured at build time
g := graph.NewMessageGraph()

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
g.Node("request_approval", func(ctx context.Context, view graph.View) (*graph.Command, error) {
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
approvalGuard := func(ctx context.Context, view graph.View) (bool, string, error) {
    content := graph.Get(view, ContentKey)
    if containsSensitiveData(content) {
        return true, "Contains sensitive information", nil
    }
    return false, "", nil  // No approval needed
}

// Add node with approval guard
g.Node("send_email", func(ctx context.Context, view graph.View) (*graph.Command, error) {
    content := graph.Get(view, ContentKey)
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
sensitiveGuard := func(ctx context.Context, view graph.View) (bool, string, error) {
    content := graph.Get(view, ContentKey)
    keywords := []string{"confidential", "secret", "classified"}
    
    for _, kw := range keywords {
        if strings.Contains(strings.ToLower(content), kw) {
            return true, fmt.Sprintf("Contains sensitive keyword: %s", kw), nil
        }
    }
    return false, "", nil  // Auto-continue
}

// Example: Amount threshold
amountGuard := func(ctx context.Context, view graph.View) (bool, string, error) {
    amount := graph.Get(view, AmountKey)
    if amount > 10000 {
        return true, fmt.Sprintf("Amount exceeds $10k: $%.2f", amount), nil
    }
    return false, "", nil
}

// Example: Always require approval
alwaysGuard := func(ctx context.Context, view graph.View) (bool, string, error) {
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
guard := func(ctx context.Context, view graph.View) (bool, string, error) {
    if !needsReview(view) {
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

// Access in node
func myNode(ctx context.Context, view graph.View) (*graph.Command, error) {
    config := graph.GetManaged(ctx, view, configMV)
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

### Comparison with regular state

| Feature | Regular State | Managed Values |
|---------|--------------|----------------|
| Access | `graph.Get(view, key)` | `graph.GetManaged(ctx, view, mv)` |
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
