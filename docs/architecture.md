---
layout: doc
title: Architecture
permalink: /architecture/
hero:
  title: Understand the Pregel BSP execution model
  description: Learn how AgentMesh uses bulk-synchronous parallel graph processing for deterministic, scalable agent orchestration.
  primary_cta:
    label: Explore the graph engine
    href: "#pregel-bsp-model"
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph"
    external: true
sidebar:
  - title: Component overview
    url: "#component-overview"
  - title: Pregel BSP model
    url: "#pregel-bsp-model"
  - title: Scheduler architecture
    url: "#scheduler-architecture"
  - title: Runtime execution
    url: "#runtime-execution"
  - title: Graph API
    url: "#graph-api"
  - title: State management
    url: "#state-management"
  - title: Execution lifecycle
    url: "#execution-lifecycle"
  - title: Performance characteristics
    url: "#performance-characteristics"
---

AgentMesh is built on a Pregel-inspired bulk-synchronous parallel (BSP) graph execution engine. This architecture enables deterministic, scalable multi-agent workflows with parallel execution and efficient state management.

---

## Component Architecture Overview {#component-overview}

AgentMesh follows a **clean, interface-based architecture** with strict separation of concerns:

<div class="mermaid">
graph TB
    subgraph Application["Application Layer (pkg/agent)"]
        ReAct["ReActAgent<br/>Reasoning + Acting"]
        Supervisor["SupervisorAgent<br/>Multi-agent coordination"]
        RAG["RAGAgent<br/>Retrieval-Augmented"]
    end
    
    subgraph GraphLayer["Graph Layer"]
        Builder["Graph Builder<br/>• Validates topology<br/>• Fluent API"]
        Compiled["Compiled<br/>• Immutable topology<br/>• Run() → events<br/>• Pure delegation"]
    end
    
    subgraph Interfaces["Core Interfaces"]
        Structure["Structure<br/>• Nodes/edges<br/>• Topology queries"]
        Executor["Executor<br/>• Run()<br/>• CurrentSuperstep()"]
        StateManager["StateManager<br/>• State persistence<br/>• Message handling"]
    end
    
    subgraph Executors["Executor Implementations"]
        Pregel["PregelExecutor<br/>• BSP Supersteps<br/>• Parallel workers<br/>• Message bus"]
        Simple["SequentialExecutor<br/>• Topological order<br/>• Single-threaded"]
    end
    
    Application --> GraphLayer
    Builder -->|"Build()"| Compiled
    Compiled --> Structure
    Compiled --> Executor
    Compiled --> StateManager
    Executor --> Pregel
    Executor --> Simple
    
    style Application fill:#1e40af,stroke:#3b82f6,color:#fff
    style GraphLayer fill:#0f766e,stroke:#14b8a6,color:#fff
    style Interfaces fill:#7c3aed,stroke:#a78bfa,color:#fff
    style Executors fill:#b45309,stroke:#f59e0b,color:#fff
</div>

**Key Design Principles:**

1. **Clean Architecture**: No special cases or type switching
   - `Compiled.Run()` simply delegates to `executor.Run()`
   - All executors treated uniformly through interface
   - No coupling between Compiled and specific executor implementations

2. **Interface-Based Design**: Both State and Execution are abstracted
   - `Structure`: Read-only topology access for executors
   - `StateManager`: Mutable state management
   - `Executor`: Pluggable execution strategies

3. **Self-Contained Executors**: Each executor owns its execution logic
   - PregelExecutor manages BSP coordination, workers, message bus
   - SequentialExecutor manages sequential execution
   - No shared execution state between executor types

4. **Separation of Concerns**:
   - **Graph**: Construction and validation
   - **Compiled**: Topology storage and coordination
   - **Executor**: Execution strategy and runtime management
   - **StateManager**: State persistence and message handling

5. **Extensibility**: Easy to add new execution strategies
   - Implement `Executor` interface
   - No changes to Compiled or Graph needed
   - Full access to topology via Structure interface

### Execution Abstraction Layer

AgentMesh uses an **executor pattern** to separate execution concerns from orchestration:

**Model Execution** (`pkg/model/executor.go`):
- `model.Executor` interface: Handles model generation lifecycle
- `DefaultExecutor`: Standard implementation with plugins, observability, streaming
- Custom executors: Retry, caching, rate limiting, circuit breakers
- **Unified Interface**: `iter.Seq2[*Response, error]` for streaming and non-streaming

**Tool Execution** (`pkg/tool/executor.go`):
- `tool.Executor` interface: Handles tool execution lifecycle
- `tool.NewExecutor`: Default sequential executor, one tool at a time
- `ParallelExecutor`: Concurrent execution with optional concurrency limits
- Custom executors: Caching, batching, circuit breakers
- **Arguments as JSON Strings**: `Call.Arguments` is `string` (not `map[string]any`)
  - Eliminates wasteful marshal/unmarshal cycles
  - Arguments flow as JSON from LLM → ToolCall → Executor → Tool

**Benefits**:
- ✅ **Reusability**: Use executors in graphs, chains, or direct calls
- ✅ **Testability**: Test execution independently from graph/state
- ✅ **Extensibility**: Custom implementations without modifying core
- ✅ **Performance**: Arguments stay as JSON strings (no extra conversions)
- ✅ **Clean Boundaries**: Nodes are thin orchestration layers (~130-180 lines)

The rest of this document explores the **Pregel BSP execution engine** (PregelExecutor) that powers the framework.

---

## Pregel BSP model {#pregel-bsp-model}

Inspired by Google's Pregel paper, AgentMesh executes graphs using a **Bulk Synchronous Parallel (BSP)** execution model. This provides a powerful foundation for complex agent workflows with loops, conditions, and parallel execution.

### What is BSP?

BSP divides computation into discrete **supersteps**, each consisting of three phases:

1. **Compute Phase** – All ready vertices (nodes) execute in parallel
2. **Message Passing** – Vertices send messages to other vertices via a mailbox system
3. **Synchronization Barrier** – Wait for all vertices to complete before the next superstep

This model provides:
- ⚡ **Parallel execution** of independent nodes (~6μs overhead per node)
- 🔒 **Deterministic ordering** within supersteps
- 📊 **Easy reasoning** about distributed state
- 🔄 **Automatic checkpointing** at superstep boundaries
- 🔁 **Natural support for iterative algorithms** (loops, refinement)

### Superstep Execution Flow

<div class="mermaid">
flowchart TB
    subgraph S0["Superstep N"]
        A["1. Scheduler identifies<br/>ready nodes"]
        B["2. Execute nodes<br/>in parallel"]
        C["3. Apply state<br/>updates"]
        D["4. Message delivery<br/>phase"]
        E["5. Synchronization<br/>barrier"]
    end
    
    A --> B
    B --> C
    C --> D
    D --> E
    E -->|"Next superstep"| A
    E -->|"END reached"| F["Complete"]
    
    style A fill:#1e40af,stroke:#3b82f6,color:#fff
    style B fill:#059669,stroke:#10b981,color:#fff
    style C fill:#7c3aed,stroke:#a78bfa,color:#fff
    style D fill:#b45309,stroke:#f59e0b,color:#fff
    style E fill:#dc2626,stroke:#f87171,color:#fff
    style F fill:#16a34a,stroke:#22c55e,color:#fff
</div>

**Phase Details:**

1. **Scheduler** identifies ready nodes (nodes with all dependencies met)
2. **Execute** nodes in parallel using worker pool; each vertex reads its mailbox
3. **Apply state updates** immediately to shared StateManager (in-memory)
4. **Message delivery** evaluates conditional routes and sends to downstream mailboxes
5. **Synchronization barrier** waits for all vertices, saves checkpoint

### Why BSP for Agent Workflows?

Traditional agent frameworks use **sequential DAG execution**, which limits expressiveness:

❌ **Sequential DAG limitations**:
- No loops (can't retry or refine)
- No cycles (can't implement feedback)
- No iterative refinement

✅ **BSP advantages**:
- ✅ Natural loops via mailbox messages and multiple supersteps
- ✅ Conditional routing can create cycles
- ✅ Iterative refinement (agent tries → evaluator judges → agent retries)
- ✅ Parallel execution when dependencies allow
- ✅ Deterministic execution despite parallelism

**Example: Iterative Refinement**

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

var DraftKey = graph.NewKey[string]("draft")
var FeedbackKey = graph.NewKey[string]("feedback")
var DoneKey = graph.NewKey[bool]("done")

g := graph.New[string, string](DraftKey, FeedbackKey, DoneKey)

// Writer node generates drafts
g.Node("writer", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    feedback := graph.Get(scope, FeedbackKey)
    draft := generateDraft(feedback)
    return graph.Set(DraftKey, draft).To("evaluator"), nil
}, "evaluator")

// Evaluator checks quality and creates a cycle!
g.Node("evaluator", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    draft := graph.Get(scope, DraftKey)
    if isGoodEnough(draft) {
        return graph.Set(DoneKey, true).To(graph.END), nil
    }
    // Loop back to writer - creates a cycle!
    return graph.Set(FeedbackKey, "improve clarity").To("writer"), nil
}, "writer", graph.END)

g.Start("writer")
compiled, _ := g.Build()
```

This creates a loop where the writer improves the draft based on evaluator feedback, executing over multiple supersteps until quality is acceptable.

---

## Graph Introspection {#graph-introspection}

AgentMesh provides a comprehensive introspection API for debugging, monitoring, and visualizing compiled graphs:

### Introspection Methods

```go
// Basic inspection
nodes := compiled.GetNodes()                    // List all node names
info, _ := compiled.GetNodeInfo("my_node")     // Node metadata

// Topology analysis
topo := compiled.GetTopology()
fmt.Printf("Entry points: %v\n", topo.EntryPoints)
fmt.Printf("Exit points: %v\n", topo.ExitPoints)
fmt.Printf("Max depth: %d\n", topo.MaxDepth)

// Graph metrics
metrics := compiled.GetMetrics()
fmt.Printf("Cyclomatic complexity: %d\n", metrics.CyclomaticComplexity)
fmt.Printf("Completed nodes: %v\n", metrics.CompletedNodes)

// Execution paths
paths := compiled.GetExecutionPath(100)
for i, path := range paths {
    fmt.Printf("Path %d: %v\n", i+1, path)
}
```

### Mermaid Flowchart Generation

Generate visual diagrams of your graph structure:

```go
// Generate flowchart with top-down layout
flowchart := compiled.GenerateMermaidFlowchart("TD")
os.WriteFile("graph.mmd", []byte(flowchart), 0644)

// Supported directions: TD (top-down), LR (left-right), BT, RL
```

The generated Mermaid syntax includes:
- **Stadium shapes** for START/END nodes
- **Diamond shapes** for conditional nodes
- **Rectangle shapes** for standard nodes
- **Solid arrows** for direct edges
- **Dashed arrows** for conditional branches

See the [graph_introspection example](https://github.com/hupe1980/agentmesh/tree/main/examples/graph_introspection) for complete usage.

---

## Scheduler architecture {#scheduler-architecture}

The scheduler is the brain of the execution engine, determining which nodes can execute in each superstep. It's composed of **four specialized components** that work together:

### Component Overview

```
┌───────────────────────────────────────────────────┐
│          vertexScheduler (Orchestrator)           │
│  Coordinates all scheduling decisions             │
└───────┬───────────────────────────────────────────┘
        │
        ├─── Delegates to ───┐
        │                    │
   ┌────▼────────────┐  ┌───▼──────────────┐
   │ Topology        │  │ Conditional      │
   │ Scheduler       │  │ Evaluator        │
   │                 │  │                  │
   │ - DAG tracking  │  │ - Route logic    │
   │ - In-degrees    │  │ - Gate checks    │
   │ - Dependencies  │  │ - Dynamic edges  │
   └────┬────────────┘  └───┬──────────────┘
        │                   │
   ┌────▼────────────┐  ┌───▼──────────────┐
   │ Execution       │  │ Pause State      │
   │ Tracker         │  │                  │
   │                 │  │ - HITL support   │
   │ - History       │  │ - Manual gates   │
   │ - Completed     │  │ - Debugging      │
   └─────────────────┘  └──────────────────┘
```

### 1. TopologyScheduler: DAG Dependency Tracking

The **TopologyScheduler** maintains the directed acyclic graph (DAG) structure and tracks dependencies using **in-degree counting**:

**How it works**:

1. Each node has an **in-degree** (number of incoming edges)
2. A node is **ready** when its in-degree reaches 0 (all dependencies satisfied)
3. When a node executes, it **decrements** the in-degree of its successors
4. This naturally handles parallel execution (multiple nodes can reach in-degree 0 simultaneously)

**Example**:

```
Initial state:
  START → A (in-degree: 1)
  START → B (in-degree: 1)
  A → C (in-degree: 1)
  B → C (in-degree: 2)  ← Note: C depends on both A and B

Superstep 0:
  Ready: [START]  (in-degree = 0)
  Execute: START
  After execution: A (in-degree: 0), B (in-degree: 0)

Superstep 1:
  Ready: [A, B]  ← Both ready simultaneously
  Execute: A, B in parallel
  After execution: C (in-degree: 0)  ← Now both dependencies satisfied

Superstep 2:
  Ready: [C]
  Execute: C
```

### 2. ConditionalEvaluator: Dynamic Routing

The **ConditionalEvaluator** handles conditional edges that determine routing at runtime:

**How it works**:

1. Conditional edges are evaluated based on state
2. The evaluator maintains "gate status" for conditional branches
3. A node with conditional incoming edges only executes when its gate is open

**Example**:

```go
g.Node("classifier", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    messages := message.GetMessages(scope)
    category := analyzeInput(messages)
    
    // Return different paths based on runtime data
    if category == "urgent" {
        return graph.Set(CategoryKey, category).To("urgent_handler"), nil
    }
    return graph.Set(CategoryKey, category).To("standard_handler"), nil
}, "urgent_handler", "standard_handler")
```

### 3. ExecutionTracker: History and State

The **ExecutionTracker** maintains execution history for:

1. **Observability**: Track execution order for debugging
2. **Cycle Detection**: Detect infinite loops (node executed too many times)
3. **Resume Support**: When resuming from checkpoint, replay history to restore state
4. **Metrics**: Count executions per node for performance analysis

### 4. Pause State: Human-in-the-Loop Support

The **pause mechanism** enables human-in-the-loop workflows:

**Use cases**:

- Manual approval gates
- Human review of agent decisions
- Interactive debugging
- A/B testing (pause and compare branches)

### Scheduler Coordination

The **vertexScheduler** orchestrates all components:

```go
func (s *vertexScheduler) Ready() []string {
    // 1. Get topologically ready nodes
    candidates := s.topology.Ready()
    
    ready := []string{}
    for _, name := range candidates {
        // 2. Filter out paused nodes
        if s.paused[name] {
            continue
        }
        
        // 3. Filter out nodes with closed conditional gates
        if !s.evaluator.IsGateOpen(name) {
            continue
        }
        
        ready = append(ready, name)
    }
    
    return ready
}
```

**Order of evaluation**:

1. ✅ **Topology**: Is the node's DAG dependencies satisfied?
2. ✅ **Pause**: Is the node manually paused?
3. ✅ **Conditional**: Is the node's conditional gate open?

Only nodes passing all three checks are **ready** to execute.

---

## Runtime execution engine {#runtime-execution}

The **Pregel Runtime** is the low-level execution engine that orchestrates superstep execution, manages mailboxes, and coordinates worker threads.

### Superstep Execution

Each superstep follows this precise sequence:

```
1. Initialize
   ├─ Get ready vertices from scheduler
   ├─ Create worker pool (size = MaxWorkers)
   └─ Prepare mailboxes for reading

2. Compute Phase (Parallel)
   ├─ Worker 1: Execute vertex A
   │  ├─ Read mailbox messages
   │  ├─ Read shared state
   │  ├─ Call node's function
   │  └─ Produce Command
   │
   ├─ Worker 2: Execute vertex B (concurrent)
   └─ Worker N: Execute vertex N (concurrent)

3. Synchronization
   ├─ Wait for all workers to complete
   └─ Collect all Commands

4. State Update (Sequential)
   ├─ Apply state updates atomically
   └─ Update aggregators

5. Message Delivery (Sequential)
   ├─ Evaluate conditional routes
   ├─ Determine next nodes for each result
   └─ Update scheduler state

6. Checkpoint (if configured)
   ├─ Save state snapshot
   └─ Store superstep number

7. Check Termination
   ├─ END node reached?
   ├─ Max iterations exceeded?
   ├─ Context cancelled?
   └─ No ready vertices remaining?

8. Emit Stream Event
   └─ Send superstep completion event
```

**Key invariants**:

- ✅ All vertices in a superstep execute with the **same state snapshot**
- ✅ State updates are applied **atomically** after all vertices complete
- ✅ Messages sent in superstep N are **not visible** until superstep N+1
- ✅ No vertex sees partial state from another vertex's execution

### Worker Pool Pattern

The runtime uses a **fixed worker pool** to control parallelism and prevent unbounded goroutine creation:

**Benefits**:

- ⚡ **Parallel execution** when topology allows
- 🔒 **Resource control** - Creates exactly `MaxWorkers` goroutines
- 📊 **Predictable performance** - No unbounded goroutine spawning
- 💾 **Fixed memory usage** - Stack memory scales with `MaxWorkers`, not frontier size

**Tuning guidance**:

- **CPU-bound nodes**: `MaxWorkers = runtime.NumCPU()`
- **I/O-bound nodes** (API calls): `MaxWorkers = 2-4x runtime.NumCPU()`
- **Mixed workload**: `MaxWorkers = runtime.NumCPU() + small buffer`

---

## Graph API {#graph-api}

The graph package provides a fluent API for constructing agent workflows:

### Basic Structure

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Define typed state keys
var StatusKey = graph.NewKey[string]("status")
var CountKey = graph.NewKey[int]("count")

// Create graph with keys
g := graph.New[string, string](StatusKey, CountKey)

// Add nodes with fluent API
g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    count := graph.Get(scope, CountKey)
    return graph.Set(StatusKey, "done").
        Set(CountKey, count+1).
        To(graph.END), nil
}, graph.END)

// Set entry point
g.Start("process")

// Compile into executable graph
compiled, err := g.Build()
```

### MessageGraph for Agents

For agent workflows with message handling:

```go
g := message.NewGraphBuilder()

g.Node("agent", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    messages := message.GetMessages(scope)
    response := processMessages(messages)
    return graph.Set(message.MessagesKey, []message.Message{response}).To(graph.END), nil
}, graph.END)

g.Start("agent")
compiled, _ := g.Build()
```

### Conditional Routing

Routes are determined dynamically using commands:

```go
g.Node("classifier", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    category := graph.Get(scope, CategoryKey)
    
    switch category {
    case "urgent":
        return graph.To("urgent_handler"), nil
    case "research":
        return graph.To("researcher"), nil
    default:
        return graph.To("default_handler"), nil
    }
}, "urgent_handler", "researcher", "default_handler")
```

### Parallel Execution

Independent nodes automatically execute in parallel based on topology:

```go
// START fans out to three parallel workers
g.Start("start")

g.Node("start", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.To("analyst_a", "analyst_b", "analyst_c"), nil
}, "analyst_a", "analyst_b", "analyst_c")

// All converge to aggregator
g.Node("analyst_a", analyzeA, "aggregator")
g.Node("analyst_b", analyzeB, "aggregator")
g.Node("analyst_c", analyzeC, "aggregator")
```

---

## State management {#state-management}

AgentMesh uses a **type-safe state system** with compile-time guarantees.

### Type-Safe Keys

Define typed state keys for compile-time type safety:

```go
// Single value keys (ReplaceReducer - last write wins)
var StatusKey = graph.NewKey[string]("status")
var CounterKey = graph.NewKey[int]("counter")
var ConfigKey = graph.NewKey[Config]("config")

// List keys
var TagsKey = graph.NewListKey[string]("tags")
var MessagesKey = message.MessagesKey  // Built-in message list key
```

### Reading State

Nodes receive immutable state views:

```go
g.Node("reader", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // Type-safe reads (no type assertions)
    status := graph.Get(scope, StatusKey)      // string
    counter := graph.Get(scope, CounterKey)    // int
    tags := graph.GetList(scope, TagsKey)      // []string
    
    return graph.To("next"), nil
}, "next")
```

### Updating State

Nodes return commands with state updates:

```go
g.Node("updater", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(StatusKey, "complete").
        Set(CounterKey, 42).
        Append(TagsKey, "new-tag").
        To(graph.END), nil
}, graph.END)
```

### Channel Types

Internally, state uses different channel types:

1. **TopicChannel** – Accumulates messages (append-only list)
2. **LastValueChannel** – Stores only the most recent value (overwrite semantics)
3. **BinaryOpChannel** – Merges values using custom operators (sum, max, concat, etc.)

---

## Execution lifecycle {#execution-lifecycle}

### 1. Initialization

```go
compiled, err := g.Build()
```

The compiler validates the graph topology, checks for cycles, and prepares the execution scheduler.

### 2. Invocation

```go
// Streaming execution - process results as they arrive
for result, err := range compiled.Run(ctx, input) {
    if err != nil {
        log.Fatal(err)
    }
    // Access state from result view
    status := graph.Get(result, StatusKey)
    fmt.Printf("Status: %s\n", status)
}
```

### 3. Superstep Execution

For each superstep:
1. Scheduler identifies ready nodes (all dependencies satisfied)
2. Nodes execute in parallel (up to worker pool size)
3. State updates are applied atomically
4. Conditional routes are evaluated
5. Checkpoint is saved (if configured)
6. Process repeats until END or max iterations

### 4. Result Collection

Final output is returned from the graph:

```go
// Get last result using iterator
var lastResult graph.ReadOnlyScope
for result, err := range compiled.Run(ctx, input) {
    if err != nil {
        log.Fatal(err)
    }
    lastResult = result
}
// lastResult contains the final state (read-only scope)
```

---

## Performance characteristics {#performance-characteristics}

The graph engine is optimized for low-latency, high-throughput execution:

- **~6μs overhead per node** – Minimal execution overhead from the scheduler
- **O(1) ready vertex lookup** – Maintained ready queue for constant-time vertex retrieval
- **O(1) aggregate updates** – Lazy copy-on-write caching for aggregate snapshots
- **Lock-free state reads** – `sync.Map`-based ChannelRegistry for concurrent reads without contention
- **Parallel node execution** – Independent nodes run concurrently
- **Lock splitting** – Reduced contention via channel-specific locks
- **Efficient checkpointing** – Copy-on-write state snapshots
- **Configurable workers** – Tune parallelism based on workload

### Scheduler Optimization

The TopologyScheduler uses a **maintained ready queue** for constant-time vertex lookup:

| Operation | Complexity | Notes |
|-----------|------------|-------|
| `Ready()` | O(1) | Returns pre-maintained queue |
| `MarkExecuted()` | O(d log n) | d = out-degree, maintains sorted queue |
| Memory | O(n + k) | k = ready vertices (typically small) |

**Benefits for large graphs**:
- Constant-time ready vertex retrieval eliminates per-superstep iteration
- Especially beneficial for iterative algorithms with many supersteps
- Memory overhead negligible (only ready vertices in queue)
- 10,000 nodes × 100 supersteps: ~1M iterations avoided

### Lock-Free Channel Registry

The ChannelRegistry uses **`sync.Map`** for lock-free concurrent reads, enabling true parallel state access:

| Operation | Implementation | Characteristics |
|-----------|----------------|-----------------|
| `GetChannel()` | Lock-free read | O(1), no contention |
| `GetChannelValue()` | Lock-free read | O(1), no contention |
| `RegisterChannel()` | Lightweight mutex | Write operations use simple mutex |

**BSP Execution Pattern**:

In bulk-synchronous parallel execution, **all workers read state simultaneously** at superstep boundaries:

```
Parallel State Reads (sync.Map):
Worker 1: [read]─────────────────────────────────
Worker 2: [read]─────────────────────────────────
Worker 3: [read]─────────────────────────────────
Worker N: [read]─────────────────────────────────
         ↑ All workers read concurrently without locks
```

**Performance characteristics**:
- **Lock-free reads**: No mutex acquisition for read operations
- **Linear scalability**: Read throughput scales with worker count
- **BSP-optimized**: Designed for read-heavy workloads with burst access patterns

### Benchmark Results

```
BenchmarkOptimized     100000    6147 ns/op    ~6μs per node
BenchmarkChannelOnly   100000    7432 ns/op
BenchmarkBaseline      100000   12891 ns/op
```
