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

AgentMesh uses a simple tuple-based API for type-safe state updates. Nodes return `([]string, state.Updates, error)` directly. Two patterns are supported for creating updates:

1. **Map literal** - Explicit, traditional approach
2. **Command builder** - Fluent, ergonomic approach (recommended for complex updates)

### Basic pattern

All nodes use the same signature with Go generics for compile-time type safety:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/state"
)

// Define typed keys
var (
    CounterKey = state.NewKey[int]("counter", 0)
    StatusKey  = state.NewKey[string]("status", "")
)

// Node function returns tuple: (targets, updates, error)
func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Read current state
    counter := state.GetFromView(view, CounterKey)
    
    // Recommended: Command pattern for fluent, type-safe updates
    return command.New().
        Set(CounterKey, counter+1).
        Set(StatusKey, "processing").
        To("next_node")
}
```

### Command builder pattern

The Command builder provides a fluent API that eliminates `.Name()` calls:

```go
// Command pattern (recommended) - fluent, clean, type-safe
return command.New().
    Set(CounterKey, 42).
    Set(StatusKey, "ready").
    To("next")

// Manual updates (alternative) - more verbose
updates := state.Updates{}
updates[CounterKey.Name()] = 42
updates[StatusKey.Name()] = "ready"
return []string{"next"}, updates, nil
```

The builder automatically calls `.Name()` on keys and constructs the tuple in one expression. Both patterns work - use whichever fits your style.

### Node patterns

**Pattern 1: Single target with updates**
```go
func processNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    return command.New().
        Set(ResultKey, "processed").
        To("next")
}
```

**Pattern 2: Multiple targets (parallel execution)**
```go
func splitNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    return command.New().
        Set(StatusKey, "splitting").
        To("worker1", "worker2", "worker3")
}
```

**Pattern 3: Conditional routing**
```go
func decideNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    score := state.GetFromView(view, ScoreKey)
    
    cmd := command.New().With(command.SetValue(ScoreKey, score+10))
    
    if score > 50 {
        return cmd.To("high_priority")
    }
    return cmd.To("normal_priority")
}
```

**Pattern 4: End node (no further targets)**
```go
func finalNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    return command.New().
        Set(StatusKey, "complete").
        To(graph.END)
}
```

**Pattern 5: No updates (read-only or pass-through)**
```go
func readOnlyNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Just read and route - no state changes
    data := state.GetFromView(view, DataKey)
    fmt.Printf("Data: %v\n", data)
    return []string{"next"}, nil, nil
}
```

### Type safety features

**Compile-time guarantees:**
- Type mismatches caught during compilation
- Typed key definitions with `state.NewKey[T]()`
- Type-safe reads with `state.GetFromView(view, TypedKey)`
- Zero runtime overhead for type checking

**Using typed keys:**
```go
// Define typed keys upfront
var (
    CounterKey  = state.NewKey[int]("counter", 0)
    StatusKey   = state.NewKey[string]("status", "")
    ValidKey    = state.NewKey[bool]("valid", false)
    TagsKey     = state.NewListKey[string]("tags", 100)
    MessagesKey = message.MessagesKey  // Built-in message list key
)

// Use in node function
func myNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // ✅ Type-safe reads
    counter := state.GetFromView(view, CounterKey)   // int
    status := state.GetFromView(view, StatusKey)     // string
    valid := state.GetFromView(view, ValidKey)       // bool
    tags := state.GetFromView(view, TagsKey)         // []string
    
    // ✅ Type-safe updates
    updates := state.Updates{}
    updates[CounterKey.Name()] = counter + 1         // Must be int
    updates[StatusKey.Name()] = "active"             // Must be string
    updates[ValidKey.Name()] = true                  // Must be bool
    updates[TagsKey.Name()] = []string{"new"}        // Must be []string
    
    return []string{"next"}, updates, nil
}
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
import "github.com/hupe1980/agentmesh/pkg/state"

// 1. Global keys (default) - simple, no prefix
var GlobalConfig = state.NewKey[string]("config", "")
var GlobalCounter = state.NewKey[int]("counter", 0)

// 2. Namespaced keys - isolated with dot notation
agent1NS := state.MustNamespace("agent1")
agent2NS := state.MustNamespace("agent2")

// Create namespaced keys: "agent1.status", "agent2.status"
agent1Status := state.TypedKey[string](agent1NS, "status", "idle")
agent2Status := state.TypedKey[string](agent2NS, "status", "idle")

// No collisions - each agent has its own "status" key
mgr := state.NewManager()
state.RegisterKey(mgr, agent1Status)
state.RegisterKey(mgr, agent2Status)

state.Set(ctx, mgr, agent1Status, "processing")  // agent1.status = "processing"
state.Set(ctx, mgr, agent2Status, "waiting")     // agent2.status = "waiting"
```

### Creating namespaces

```go
// Create namespace (returns error if invalid)
ns, err := state.NewNamespace("agent1")
if err != nil {
    return err
}

// Create namespace (panics if invalid) - use for package-level variables
var agentNS = state.MustNamespace("agent")

// Global namespace (no prefix)
globalNS := state.Global

// Check if namespace is global
if ns.IsGlobal() {
    fmt.Println("No prefix")
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
view, err := mgr.CreateReadView(ctx)
if err != nil {
    return err
}

// Get all keys in a namespace
agent1NS := state.MustNamespace("agent1")
agent1View := state.GetNamespaceView(view, agent1NS)
// Returns: map[string]any{"status": "processing", "progress": 50}
// Note: Keys are returned WITHOUT namespace prefix

// Get global keys
globalView := state.GetNamespaceView(view, state.Global)
// Returns: map[string]any{"config": "production", "counter": 100}
```

**List namespaces** - Discover active namespaces:

```go
view, _ := mgr.CreateReadView(ctx)
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
```

**Copy namespace** - Transfer state between agents (useful for handoffs):

```go
agent1NS := state.MustNamespace("agent1")
agent2NS := state.MustNamespace("agent2")

// IMPORTANT: Target keys must be registered first!
agent1Status := state.TypedKey[string](agent1NS, "status", "")
agent2Status := state.TypedKey[string](agent2NS, "status", "")

state.RegisterKey(mgr, agent1Status)
state.RegisterKey(mgr, agent2Status)

// Set source state
state.Set(ctx, mgr, agent1Status, "processing")

// Copy agent1 state to agent2
err := state.CopyNamespace(ctx, mgr, agent1NS, agent2NS)
// Now agent2.status = "processing"
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

// Each agent has its own "status" and "progress" keys
researcherStatus := state.TypedKey[string](researcherNS, "status", "")
writerStatus := state.TypedKey[string](writerNS, "status", "")
editorStatus := state.TypedKey[string](editorNS, "status", "")

// Register all keys
mgr := state.NewManager()
state.RegisterKey(mgr, researcherStatus)
state.RegisterKey(mgr, writerStatus)
state.RegisterKey(mgr, editorStatus)

// Each agent updates its own state independently
state.Set(ctx, mgr, researcherStatus, "researching")
state.Set(ctx, mgr, writerStatus, "writing")
state.Set(ctx, mgr, editorStatus, "editing")

// No collisions - each has separate namespace
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

For guaranteed state isolation, nodes can declare which namespace they operate in. This is ideal for multi-agent systems and pipeline stages where you want to enforce strict boundaries.

#### NamespacedNode interface

Nodes implement the optional `NamespacedNode` interface to declare their namespace:

```go
type NamespacedNode interface {
    Node
    Namespace() state.Namespace
}
```

When a node implements this interface:
- It **declares** which namespace it uses (for documentation and introspection)
- State isolation is **enforced** through namespaced key names
- Keys from different namespaces cannot collide (e.g., `"agent1.status"` vs `"agent2.status"`)

#### Creating namespaced nodes

Use `NewNamespacedCommandNode()` for convenient namespaced nodes:

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Define namespaces
validationNS := state.MustNamespace("validation")
enrichmentNS := state.MustNamespace("enrichment")

// Define keys per namespace
validKey := state.TypedKey[bool](validationNS, "is_valid", false)
enrichedKey := state.TypedKey[map[string]any](enrichmentNS, "data", nil)

// Create namespaced nodes
validationNode := &graph.BaseNode{
    NodeName: "validation",
    Namespace: validationNS,
    DeclaredTargets: []string{"enrichment"},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // This node only works with "validation.*" keys
        updates := state.Updates{}
        updates[validKey.Name()] = true
        
        return []string{"enrichment"}, updates, nil
    },
}

enrichmentNode := &graph.BaseNode{
    NodeName: "enrichment",
    Namespace: enrichmentNS,
    DeclaredTargets: []string{graph.END},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // This node only works with "enrichment.*" keys
        // Cannot access "validation.*" keys (different namespace)
        updates := state.Updates{}
        enrichedData := map[string]any{"status": "enriched"}
        updates[enrichedKey.Name()] = enrichedData
        
        return []string{graph.END}, updates, nil
    },
}
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

node := graph.NewNamespacedCommandNodeWithRetry(
    "processor",
    processorNS,
    commandFunc,
    retryPolicy,
    targets,
)
```

#### When to use namespaced nodes

**Use `NamespacedCommandNode` when:**
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

1. When a `NamespacedCommandNode` executes, it receives a `NamespacedReadView` (not full `ReadView`)
2. `NamespacedReadView` is a filtered wrapper that only exposes keys from the node's namespace
3. Calling `view.Keys()` returns only keys from that namespace
4. Calling `view.Has("other_namespace.key")` returns `false` (filtered out)
5. The node physically cannot access keys from other namespaces
6. **Returned updates are validated** - attempting to update keys outside the namespace causes an error
7. Optionally, global (non-namespaced) keys can be included via `includeGlobal` parameter

```go
// Keys are created with namespace prefixes
agent1Status := state.TypedKey[string](agent1NS, "status", "")  // "agent1.status"
agent2Status := state.TypedKey[string](agent2NS, "status", "")  // "agent2.status"

// Both keys exist in state
state.Set(ctx, mgr, agent1Status, "processing")
state.Set(ctx, mgr, agent2Status, "idle")

// But when agent1 node executes:
// - view.Keys() returns ["status"] (only agent1's keys, without prefix)
// - view.Has("agent1.status") returns true
// - view.Has("agent2.status") returns false (filtered out!)
// - Cannot access agent2's state at all
```

#### NamespacedReadView (automatic)

`NamespacedCommandNode` automatically receives a `NamespacedReadView` during execution. You don't need to create it manually - it's provided automatically:

```go
// When you create a NamespacedCommandNode:
node := graph.NewNamespacedCommandNode(
    "validation",
    validationNS,
    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // 'view' is actually a NamespacedReadView filtered to validationNS
        // It only exposes keys from "validation" namespace
        
        keys := view.Keys()  // Returns: ["is_valid", "score"] (WITHOUT prefix)
        
        // Can only see validation.* keys
        valid := state.GetFromView(view, validKey)  // Works
        
        // Cannot see other namespace keys
        exists := view.Has("enrichment.data")  // Returns false (filtered out)
        
        return command.New().To(graph.EndNode)
    },
    targets,
)
```

The filtering is automatic and enforced at runtime by the framework.

#### Best practices

**1. One namespace per agent/stage:**
```go
// ✅ Clear separation
researcherNS := state.MustNamespace("researcher")
writerNS := state.MustNamespace("writer")

researcherNode := graph.NewNamespacedCommandNode("researcher", researcherNS, ...)
writerNode := graph.NewNamespacedCommandNode("writer", writerNS, ...)
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

func NewNode() graph.Node {
    return graph.NewNamespacedCommandNode("validation", NS, commandFunc, targets)
}
```

**3. Document namespace usage:**
```go
// ValidationNode checks input data quality
// Namespace: "validation"
// Keys: is_valid (bool), score (int)
func NewValidationNode() graph.Node {
    return graph.NewNamespacedCommandNode("validation", validationNS, ...)
}
```

**4. Introspection:**
```go
// Check if node declares namespace
if nsNode, ok := node.(graph.NamespacedNode); ok {
    fmt.Printf("Node uses namespace: %s\n", nsNode.Namespace().Name())
}
```

#### Global state access

By default, `NamespacedCommandNode` only sees keys from its own namespace. Set `includeGlobal=true` to also expose global (non-namespaced) keys:

```go
// Node that can access both its namespace AND global keys
configNode := graph.NewNamespacedCommandNode(
    "config_reader",
    agentNS,
    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // Can read agent1.* keys
        agentData := state.GetFromView(view, agentDataKey)
        
        // Can also read global keys
        globalConfig := state.GetFromView(view, globalConfigKey)
        
        // Can update both using Command pattern
        return command.New().
            Set(agent1ResultKey, computeResult(agentData, globalConfig)).
            Set(globalCounterKey, incrementCounter()). // Allowed!
            To(graph.EndNode)
    },
    targets,
    true, // includeGlobal: expose global keys
)
```

**Use cases for includeGlobal:**
- Reading shared configuration
- Updating global counters or metrics
- Accessing system-wide state
- Coordinating between namespaces through global keys

**Important:** Even with `includeGlobal=true`, nodes still cannot access keys from *other* namespaces.

#### Update validation

`NamespacedCommandNode` validates all returned updates:

```go
validationNode := graph.NewNamespacedCommandNode(
    "validator",
    agent1NS,
    func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // ❌ This will cause a validation error:
        return command.New().
            Set(agent1StatusKey, "ok").      // ✅ Allowed (own namespace)
            Set(agent2StatusKey, "failed").  // ❌ ERROR: wrong namespace
            To(graph.EndNode) // Will return error
    },
    targets,
    false,
)

// Execution will fail with:
// "node 'validator' in namespace 'agent1' attempted to update key 
//  'agent2.status' which belongs to a different namespace"
```

This prevents accidental cross-namespace pollution and enforces state boundaries at runtime.

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
    ApprovalMetadata *ApprovalMetadata     // Pending approvals and approval history
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
builder.AddNodeFunc("request_approval", func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
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

// Define approval guard function
approvalGuard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
    // Check if approval is needed
    content := state.GetFromView(view, contentKey)
    if containsSensitiveData(content) {
        return true, "Contains sensitive information", nil
    }
    return false, "", nil  // No approval needed
}

// Add interrupt with approval guard
g.AddInterruptBefore("send_email",
    graph.WithApprovalGuard(approvalGuard),
    graph.WithFeedbackAnnotation(true),  // Record approval in message history
    graph.WithApprovalTimeout(10 * time.Minute),
)

// Step 1: Run until approval guard triggers
runID := "email-workflow-001"
for _, err := range compiled.Run(ctx, messages,
    graph.WithRunID(runID),
    graph.WithCheckpointOptions(
        checkpoint.WithCheckpointer(checkpointer),
        checkpoint.WithSaveInterval(1),
    ),
) {
    // Execution pauses when guard returns true
}

// Step 2: Load checkpoint and review pending approval
cp, _ := checkpointer.Load(ctx, runID)
if cp.ApprovalMetadata != nil {
    for nodeName, pending := range cp.ApprovalMetadata.PendingApprovals {
        fmt.Printf("Approval needed for: %s\n", nodeName)
        fmt.Printf("Reason: %s\n", pending.Reason)
        fmt.Printf("Requested at: %v\n", pending.RequestedAt)
    }
}

// Step 3: Provide approval response
approval := &graph.ApprovalResponse{
    Decision:  graph.ApprovalApproved,  // or ApprovalRejected, ApprovalEdit, ApprovalSkip
    Reason:    "Reviewed and approved",
    User:      "alice@example.com",
    Timestamp: time.Now(),
    Edits: state.Updates{
        contentKey.Name(): "Redacted sensitive content",  // Optional state edits
    },
    Annotations: map[string]any{
        "department": "security",
        "risk_level": "medium",
    },
}

// Step 4: Resume with approval
for _, err := range compiled.Run(ctx, messages,
    graph.WithCheckpoint(cp),
    graph.WithApproval("send_email", approval),
    graph.WithCheckpointOptions(
        checkpoint.WithCheckpointer(checkpointer),  // Required for history
    ),
) {
    // Execution continues with approval applied
}

// Step 5: Query approval history
history, _ := checkpointer.GetApprovalHistory(ctx, runID)
for _, record := range history {
    fmt.Printf("%s: %s by %s at %v\n", 
        record.NodeName, record.Decision, record.User, record.Timestamp)
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
    Edits: state.Updates{
        contentKey.Name(): "Modified content",
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
sensitiveGuard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
    content := state.GetFromView(view, contentKey)
    keywords := []string{"confidential", "secret", "classified"}
    
    for _, kw := range keywords {
        if strings.Contains(strings.ToLower(content), kw) {
            return true, fmt.Sprintf("Contains sensitive keyword: %s", kw), nil
        }
    }
    return false, "", nil  // Auto-continue
}

// Example: Amount threshold
amountGuard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
    amount := state.GetFromView(view, amountKey)
    if amount > 10000 {
        return true, fmt.Sprintf("Amount exceeds $10k: $%.2f", amount), nil
    }
    return false, "", nil
}

// Example: Always require approval
alwaysGuard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
    return true, "Manual approval required", nil
}
```

### State Edits During Approval

Modify state as part of the approval process:

```go
approval := &graph.ApprovalResponse{
    Decision: graph.ApprovalApproved,
    User:     "reviewer@example.com",
    Edits: state.Updates{
        // Redact sensitive data
        "content": redactSensitiveInfo(originalContent),
        
        // Add approval metadata
        "approved_by": "reviewer@example.com",
        "approved_at": time.Now(),
        
        // Modify execution parameters
        "priority": "high",
    },
}
```

State edits are applied BEFORE the node executes, allowing the node to see the modified state.

### Accessing Approvals in Nodes

Nodes can check for approval responses:

```go
sendNode := &graph.BaseNode{
    NodeName: "send_email",
    DeclaredTargets: []string{graph.EndNode},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        // Check if approval was provided
        approval := graph.ApprovalFromContext(ctx, "send_email")
        if approval == nil {
            // No approval in context - first execution
            return []string{graph.EndNode}, state.Updates{
                sentKey.Name(): false,
            }, nil
        }
        
        // Handle approval decision
        switch approval.Decision {
        case graph.ApprovalRejected:
            log.Printf("Email rejected: %s", approval.Reason)
            return []string{graph.EndNode}, state.Updates{
                sentKey.Name(): false,
                errorKey.Name(): approval.Reason,
            }, nil
            
        case graph.ApprovalApproved:
            // State edits already applied - just send
            content := state.GetFromView(view, contentKey)
            sendEmail(content)
            return []string{graph.EndNode}, state.Updates{
                sentKey.Name(): true,
            }, nil
        }
        
        return []string{graph.EndNode}, nil, nil
    },
}
```

### Approval History & Audit Trail

Query complete approval history for compliance and debugging:

```go
// Get all approvals for a run
history, err := checkpointer.GetApprovalHistory(ctx, runID)

for _, record := range history {
    fmt.Printf("Node: %s\n", record.NodeName)
    fmt.Printf("Decision: %s\n", record.Decision)  // APPROVED, REJECTED, EDIT, SKIP
    fmt.Printf("User: %s\n", record.User)
    fmt.Printf("Reason: %s\n", record.Reason)
    fmt.Printf("Timestamp: %v\n", record.Timestamp)
    
    if len(record.StateEdits) > 0 {
        fmt.Printf("State edits: %v\n", record.StateEdits)
    }
    
    if len(record.Annotations) > 0 {
        fmt.Printf("Annotations: %v\n", record.Annotations)
    }
}
```

### Approval Configuration Options

```go
g.AddInterruptBefore("critical_action",
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
g.AddInterruptBefore("draft", graph.WithApprovalGuard(draftGuard))
g.AddInterruptBefore("publish", graph.WithApprovalGuard(publishGuard))

// Provide approvals for each stage
for _, err := range compiled.Run(ctx, messages,
    graph.WithCheckpoint(cp),
    graph.WithApproval("draft", draftApproval),
    graph.WithApproval("publish", publishApproval),
    graph.WithCheckpointOptions(checkpoint.WithCheckpointer(checkpointer)),
) {
    // Process
}
```

### Error Handling

```go
// Check if approval is required but not provided
if err := graph.CheckApproval(ctx, "send_email", true); err != nil {
    // Handle missing approval
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

**1. Always pass checkpointer when resuming:**
```go
// ❌ Wrong - approval history won't persist
compiled.Run(ctx, messages,
    graph.WithCheckpoint(cp),
    graph.WithApproval("node", approval),
)

// ✅ Correct - history persisted
compiled.Run(ctx, messages,
    graph.WithCheckpoint(cp),
    graph.WithApproval("node", approval),
    graph.WithCheckpointOptions(checkpoint.WithCheckpointer(checkpointer)),
)
```

**2. Use conditional guards to avoid unnecessary approvals:**
```go
// Only trigger approval when actually needed
guard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
    if !needsReview(view) {
        return false, "", nil  // Auto-continue
    }
    return true, "Manual review required", nil
}
```

**3. Validate approvals before critical operations:**
```go
func criticalOperation(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    approval := graph.ApprovalFromContext(ctx, "critical_op")
    if approval == nil {
        return nil, nil, fmt.Errorf("missing required approval")
    }
    
    if approval.Decision != graph.ApprovalApproved {
        return nil, nil, fmt.Errorf("operation not approved")
    }
    
    // Proceed with operation
    // ...
}
```

**4. Set appropriate timeouts:**
```go
// Short timeout for routine approvals
graph.WithApprovalTimeout(5 * time.Minute)

// Long timeout for complex reviews
graph.WithApprovalTimeout(24 * time.Hour)

// No timeout (wait indefinitely)
graph.WithApprovalTimeout(0)
```

**5. Use annotations for rich audit data:**
```go
approval := &graph.ApprovalResponse{
    Decision: graph.ApprovalApproved,
    User:     "alice@example.com",
    Annotations: map[string]any{
        "department":     "security",
        "risk_level":     "medium",
        "reviewed_by":    "Alice Smith",
        "policy_version": "2.1",
        "ip_address":     "192.168.1.100",
        "session_id":     "abc123",
    },
}
```

### Approval Metadata Structure

Stored in checkpoint for persistence:

```go
type ApprovalMetadata struct {
    // Pending approvals awaiting human decision
    PendingApprovals map[string]*PendingApproval
    
    // Complete history of all approvals
    ApprovalHistory []ApprovalRecord
}

type PendingApproval struct {
    NodeName      string
    Reason        string
    RequestedAt   time.Time
    TimeoutAt     *time.Time
    RequiredState map[string]any  // State snapshot for review
}

type ApprovalRecord struct {
    NodeName    string
    Decision    string  // "APPROVED", "REJECTED", "EDIT", "SKIP"
    Reason      string
    User        string
    Timestamp   time.Time
    StateEdits  state.Updates
    Annotations map[string]any
}
```

See `examples/human_approval` for complete working examples with all three approval scenarios.

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
