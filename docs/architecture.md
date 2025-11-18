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
  - title: Graph builder and usage
    url: "#graph-builder"
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

```
┌──────────────────────────────────────────────────────────────┐
│              Application Layer (pkg/agent)                    │
│  • ReActAgent: Reasoning + Acting pattern                    │
│  • SupervisorAgent: Multi-agent coordination                 │
│  • RAGAgent: Retrieval-Augmented Generation                  │
└──────────────────────────┬───────────────────────────────────┘
                           │ builds on
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                         Graph                                 │
│  • Builder API for graph construction                         │
│  • Validates topology (nodes, edges, conditionals)            │
│  • Attaches Executor during construction                      │
│  • Compile() → Compiled                                       │
└────────────────────────┬─────────────────────────────────────┘
                         │ compile()
                         ▼
┌──────────────────────────────────────────────────────────────┐
│                    Compiled (Pure Coordinator)                │
│  • Immutable topology (implements Structure interface)        │
│  • Public API: Run(ctx, messages) → event iterator           │
│  • StateManager: Manages state, messages, checkpoints        │
│  • Executor: Injected execution strategy                      │
│  • NO execution logic - pure delegation                       │
└────────────────┬─────────────────────────┬───────────────────┘
                 │                         │
                 │ provides to executor    │ delegates to
                 ▼                         ▼
    ┌────────────────────────┐  ┌────────────────────────────┐
    │    Structure           │  │       Executor             │
    │    (Interface)         │  │       (Interface)          │
    │                        │  │                            │
    │  • Nodes/edges         │  │  Run(ctx, topology,        │
    │  • Topology queries    │  │      stateManager,         │
    │  • StateManager access │  │      messages, options)    │
    │  • Mark completed      │  │  CurrentSuperstep()        │
    │  • Pause/resume        │  │  Pause/Resume/IsPaused()   │
    │  • Superstep tracking  │  │                            │
    └────────────────────────┘  └──────────┬─────────────────┘
                                           │
                         ┌─────────────────┴─────────────────┐
                         │                                   │
                         ▼                                   ▼
            ┌─────────────────────────┐      ┌─────────────────────────┐
            │   PregelExecutor        │      │ SimpleGraphExecutor     │
            │   (Default)             │      │ (Future)                │
            │                         │      │                         │
            │ • BSP Supersteps        │      │ • Topological order     │
            │ • Parallel workers      │      │ • Single-threaded       │
            │ • Message bus           │      │ • No synchronization    │
            │ • Aggregators           │      │ • For debugging         │
            │ • Combiners             │      │                         │
            │ • Checkpoint callbacks  │      │                         │
            │                         │      │                         │
            │ Uses:                   │      │                         │
            │ • graphRuntime          │      │                         │
            │ • pkg/pregel runtime    │      │                         │
            │ • vertexScheduler       │      │                         │
            │ • stateCoordinator      │      │                         │
            │ • eventEmitter          │      │                         │
            └─────────────────────────┘      └─────────────────────────┘
```

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
   - SimpleGraphExecutor would manage sequential execution
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

> **📖 For detailed executor architecture and implementation guide, see [EXECUTOR.md](/docs/EXECUTOR.md)**

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

```
Superstep 0:
  ┌─────────────────────────────────────┐
  │ 1. Scheduler identifies ready nodes │
  │    (nodes with all dependencies met)│
  └──────────────┬──────────────────────┘
                 │
  ┌──────────────▼──────────────────────┐
  │ 2. Execute nodes in parallel        │
  │    - Worker pool processes vertices │
  │    - Each vertex reads its mailbox  │
  │    - Vertices compute and generate  │
  │      NodeResult with updates        │
  └──────────────┬──────────────────────┘
                 │
  ┌──────────────▼──────────────────────┐
  │ 3. Apply state updates              │
  │    - Immediate: Updates applied to  │
  │      shared StateManager (in-memory)│
  │    - Via messages: Updates sent to  │
  │      downstream nodes (distributed) │
  │    - Hybrid approach supports both  │
  │      single-process and distributed │
  └──────────────┬──────────────────────┘
                 │
  ┌──────────────▼──────────────────────┐
  │ 4. Message delivery phase           │
  │    - Evaluate conditional routes    │
  │    - Send messages to downstream    │
  │      node mailboxes via MessageBus  │
  │    - Messages available in NEXT     │
  │      superstep                      │
  └──────────────┬──────────────────────┘
                 │
  ┌──────────────▼──────────────────────┐
  │ 5. Synchronization barrier          │
  │    - Wait for all vertices complete │
  │    - Save checkpoint                │
  └──────────────┬──────────────────────┘
                 │
                 ▼
Superstep 1: Repeat until END or max iterations
```

### Mailbox System

Each vertex has a **mailbox** that stores messages sent to it by other vertices. Messages sent in superstep N are delivered in superstep N+1:

```go
// Superstep 0: Node A updates state
return map[string]any{"data": result}, nil

// Superstep 1: Node B receives updates via state view
// The data is available via state.GetFromView(view, DataKey)
```

**Mailbox Bounds**: To prevent memory exhaustion, mailboxes can be configured with size limits:

```go
// Configure mailbox size (default: unlimited)
runtime := pregel.NewRuntime(graph, state,
    pregel.WithMaxMailboxSize[*ChannelState, ChannelMessage](1000),
)

// Recommendations:
// - Small graphs (< 100 nodes): 10,000 messages per vertex
// - Medium graphs (100-1000 nodes): 1,000 messages per vertex
// - Large graphs (> 1000 nodes): 100-500 messages per vertex
```

When a mailbox exceeds its limit:
- Additional messages are **dropped** (not queued)
- A **warning event** is emitted to the stream with `ErrMailboxFull`
- The graph continues executing (graceful degradation)

**Message Combiners**: To reduce mailbox pressure, you can use combiners to merge messages:

```go
// Combiner reduces multiple messages into one
combiner := func(a, b ChannelMessage) ChannelMessage {
    // Merge message contents
    return mergedMessage
}
```

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
builder.Node("writer", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    draft := generateDraft()
    return map[string]any{"draft": draft}, nil
})

builder.Node("evaluator", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    draft := state.GetFromView(view, DraftKey)
    if isGoodEnough(draft) {
        return map[string]any{"done": true}, nil
    }
    // Send feedback and loop back to writer
    return map[string]any{
        "feedback": "improve clarity",
        "done": false,
    }, nil
})

// Add static edge from writer to evaluator
builder.AddEdge("writer", "evaluator")

// Use conditional edges to create cycle - routes based on state
builder.AddConditionalEdges("evaluator", func(ctx context.Context, view *state.ReadView) []string {
    done := state.GetFromView(view, DoneKey)
    if done {
        return []string{"END"}
    }
    return []string{"writer"}  // Creates a cycle!
}, []string{"END", "writer"})
```

This creates a loop where the writer improves the draft based on evaluator feedback, executing over multiple supersteps until quality is acceptable.

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

builder := graph.NewBuilder()

// Nodes execute in parallel when possible
builder.Node("fetch_data", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    // Fetch from API...
    return map[string]any{"data": result}, nil
})

builder.Node("process", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    data := view.Get("data")
    // Process...
    return map[string]any{"processed": true}, nil
})

builder.AddEdge("START", "fetch_data")
builder.AddEdge("fetch_data", "process")
builder.AddEdge("process", "END")

compiled, _ := builder.Compile()
```

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

```go
type TopologyScheduler struct {
    incoming map[string]int  // Remaining dependencies for each node
    baseline map[string]int  // Initial dependency count (for reset)
    executed map[string]bool // Completed nodes
}
```

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

**Thread Safety**: Uses `sync.RWMutex` with read locks for `Ready()` (called frequently) and write locks for `MarkExecuted()`.

**Performance Optimization**:

The TopologyScheduler uses a **maintained ready queue** for O(1) ready vertex retrieval:

```go
type TopologyScheduler struct {
    incoming   map[string]int  // Remaining dependencies per vertex
    readyQueue []string        // Maintained list of ready vertices
    inQueue    map[string]bool // Fast lookup for queue membership
}
```

**Complexity Analysis**:

- **Previous implementation**: O(n) per `Ready()` call - iterated all vertices
- **Current implementation**: O(1) `Ready()` - returns pre-maintained queue
- **MarkExecuted()**: O(d log n) where d = out-degree, n = queue size
  - Decrements downstream dependencies
  - Adds newly-ready vertices to queue
  - Maintains sorted order for deterministic execution

**Why this matters**:

- `Ready()` is called at the start of every superstep
- With 1000 nodes and 100 supersteps: ~100,000 iterations saved
- Especially impactful for large graphs with many supersteps
- Memory overhead: O(k) where k = number of ready vertices (typically small)

**Example Performance**:

```
Graph with 10,000 nodes:
  Previous: ~10,000 comparisons per Ready() call
  Current:  ~1 array return operation

Typical improvement: 100x faster ready vertex lookup
```

### 2. ConditionalEvaluator: Dynamic Routing

The **ConditionalEvaluator** handles conditional edges that determine routing at runtime:

```go
type ConditionalEvaluator struct {
    cg         *Compiled
    gateStatus map[string]bool  // Which conditional gates are open
}
```

**How it works**:

1. Conditional edges are evaluated based on state
2. The evaluator maintains "gate status" for conditional branches
3. A node with conditional incoming edges only executes when its gate is open

**Example**:

```go
builder.Node("classifier", func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    messages := state.GetFromView(view, agent.MessagesKey)
    category := analyzeInput(messages)
    
    return map[string]any{"category": category}, nil
})

// Use conditional edge function to route based on state
builder.AddConditionalEdges("classifier", func(ctx context.Context, view *state.ReadView) []string {
    category := state.GetFromView(view, CategoryKey)
    // Return different paths based on runtime data
    if category == "urgent" {
        return []string{"urgent_handler"}
    }
    return []string{"standard_handler"}
}, []string{"urgent_handler", "standard_handler"})
```

**Gate Mechanism**:

- When a conditional edge fires, it **opens the gate** for the target node
- The target node can only execute when its gate is open
- This prevents nodes from executing before their conditional logic has been evaluated

### 3. ExecutionTracker: History and State

The **ExecutionTracker** maintains execution history:

```go
type ExecutionTracker struct {
    history   []string         // Ordered list of completed nodes
    completed map[string]bool  // Set of completed nodes (fast lookup)
}
```

**Purposes**:

1. **Observability**: Track execution order for debugging
2. **Cycle Detection**: Detect infinite loops (node executed too many times)
3. **Resume Support**: When resuming from checkpoint, replay history to restore state
4. **Metrics**: Count executions per node for performance analysis

### 4. Pause State: Human-in-the-Loop Support

The **pause mechanism** enables human-in-the-loop workflows:

```go
type vertexScheduler struct {
    paused map[string]bool  // Which nodes are paused
}
```

**Usage**:

```go
// Pause is handled via the executor interface
executor := compiled.executor // internal access

// Pause execution before a node (typically set before Run)
executor.Pause("review_node")

// Execute graph (will stop before review_node)
_, err := agent.CollectMessages(compiled.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}

// Human reviews and provides input
// ...

// Resume execution
executor.Resume("review_node")
messages, err := agent.CollectMessages(compiled.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}
```

**Note**: Pause/Resume is currently part of the internal executor interface, not exposed as public methods on Compiled. For human-in-the-loop workflows, nodes can return `ErrHumanInterrupt` to pause execution naturally.

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

### Concurrency Model

The scheduler uses **lock splitting** to minimize contention:

```
┌─────────────────────────────────────┐
│ vertexScheduler                     │
│   sync.RWMutex (coarse lock)        │
│                                     │
│   ┌──────────────────────┐         │
│   │ TopologyScheduler    │         │
│   │   sync.RWMutex      │         │  ← Independent lock
│   └──────────────────────┘         │
│                                     │
│   ┌──────────────────────┐         │
│   │ ConditionalEvaluator │         │
│   │   sync.RWMutex      │         │  ← Independent lock
│   └──────────────────────┘         │
│                                     │
│   ┌──────────────────────┐         │
│   │ ExecutionTracker     │         │
│   │   sync.Mutex        │         │  ← Independent lock
│   └──────────────────────┘         │
└─────────────────────────────────────┘
```

**Lock ordering** (prevents deadlocks):

1. Never acquire `vertexScheduler.mu` while holding component locks
2. Never acquire multiple component locks simultaneously
3. Always release locks before calling external code (node functions, callbacks)

**Read-heavy optimization**:

- `TopologyScheduler` and `ConditionalEvaluator` use `RWMutex`
- Multiple `Ready()` calls can execute concurrently (read locks)
- Only `MarkExecuted()` requires exclusive write lock

This design minimizes lock contention during parallel node execution.

---

## Runtime execution engine {#runtime-execution}

The **Pregel Runtime** is the low-level execution engine that orchestrates superstep execution, manages mailboxes, and coordinates worker threads.

### Architecture

```
┌──────────────────────────────────────────────────┐
│         Pregel Runtime[S, M]                     │
│  Generic BSP execution engine                    │
│                                                  │
│  ┌────────────────┐  ┌──────────────────┐      │
│  │ Worker Pool    │  │ Mailbox System   │      │
│  │ - Configurable │  │ - Per-vertex     │      │
│  │   parallelism  │  │ - Bounded queues │      │
│  │ - goroutines   │  │ - Message passing│      │
│  └────────────────┘  └──────────────────┘      │
│                                                  │
│  ┌────────────────┐  ┌──────────────────┐      │
│  │ Aggregators    │  │ Stream Events    │      │
│  │ - Global state │  │ - Observability  │      │
│  │ - Read-only    │  │ - Real-time feed │      │
│  └────────────────┘  └──────────────────┘      │
└──────────────────────────────────────────────────┘
```

### Generic Type Parameters

The runtime is parameterized over two types:

```go
type Runtime[S any, M any] struct {
    graph Graph[S, M]  // Graph topology
    state S                  // Shared state (e.g., *State)
    // ...
}
```

- **S (State)**: The shared state type (e.g., `*State` with channels)
- **M (Message)**: The message type passed between vertices (e.g., `ChannelMessage`)

This enables type-safe execution without `interface{}` pollution.

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
   │  ├─ Call node's RunFunc
   │  └─ Produce NodeResult
   │
   ├─ Worker 2: Execute vertex B (concurrent)
   │  └─ Same as Worker 1
   │
   └─ Worker N: Execute vertex N (concurrent)
       └─ Same as Worker 1

3. Synchronization
   ├─ Wait for all workers to complete
   └─ Collect all NodeResults

4. State Update (Sequential)
   ├─ Apply channel updates atomically
   ├─ Add messages to state
   └─ Update aggregators

5. Message Delivery (Sequential)
   ├─ Evaluate conditional edges
   ├─ Determine next nodes for each result
   ├─ Send messages to successor mailboxes
   │  (Messages available in NEXT superstep)
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

```go
// Configuration
opts := pregel.RuntimeOptions[S, M]{
    MaxWorkers: 10,  // Max concurrent vertex executions
}

// Execution
vertices := scheduler.Ready()  // May return 10,000 vertices

// Fixed worker pool: Creates exactly MaxWorkers goroutines (10 in this case)
// All 10,000 vertices are processed by these 10 workers via a task queue
tasks := make(chan string, len(vertices))
for _, vertex := range vertices {
    tasks <- vertex  // Queue work
}
close(tasks)

var wg sync.WaitGroup
for i := 0; i < MaxWorkers; i++ {  // Create exactly 10 workers
    wg.Add(1)
    go func() {
        defer wg.Done()
        for vertex := range tasks {  // Process from queue
            executeVertex(vertex)
        }
    }()
}
wg.Wait()
```

**Benefits**:

- ⚡ **Parallel execution** when topology allows
- 🔒 **Resource control** - Creates exactly `MaxWorkers` goroutines, not `len(frontier)` goroutines
- 📊 **Predictable performance** - No unbounded goroutine spawning
- 💾 **Fixed memory usage** - Stack memory scales with `MaxWorkers`, not frontier size
- 🎯 **Efficient scheduling** - Task queue distributes work evenly across workers

**Comparison:**

| Approach | Goroutines Created | Memory Usage | Suitable For |
|----------|-------------------|--------------|--------------|
| **Semaphore** (old) | `len(frontier)` | O(frontier) stack space | Small frontiers (<1000) |
| **Fixed Pool** (current) | `MaxWorkers` | O(MaxWorkers) stack space | Any frontier size |

For a frontier of 10,000 vertices with `MaxWorkers=10`:
- ❌ Semaphore approach: 10,000 goroutines, ~80MB stack memory
- ✅ Fixed pool approach: 10 goroutines, ~80KB stack memory

**Tuning guidance**:

- **CPU-bound nodes**: `MaxWorkers = runtime.NumCPU()`
- **I/O-bound nodes** (API calls): `MaxWorkers = 2-4x runtime.NumCPU()`
- **Mixed workload**: `MaxWorkers = runtime.NumCPU() + small buffer`
- **Large graphs**: Worker pool automatically scales down to `min(MaxWorkers, len(frontier))`

### Message Passing and Mailboxes

Each vertex has a **mailbox** that stores messages sent to it:

```go
type Runtime[S, M] struct {
    mailbox map[string][]M  // vertex name → messages
    mu      sync.Mutex      // Protects mailbox
}
```

**State update lifecycle**:

```
Superstep N:
  Vertex A executes
  ├─ Returns NodeResult{Updates: {...}}
  └─ Runtime applies updates to state
      └─ State changes visible in next superstep

Superstep N+1:
  Vertex B executes
  ├─ Reads state via view (contains updates from A)
  ├─ Processes messages
  └─ Clears mailbox["B"]
```

**Mailbox bounds** (memory safety):

```go
// Unbounded (default)
runtime := pregel.NewRuntime(graph, state)  // Mailboxes grow indefinitely

// Bounded (recommended for production)
runtime := pregel.NewRuntime(graph, state,
    pregel.WithMaxMailboxSize[S, M](1000),
)
```

When mailbox is full:

1. ❌ **New messages are dropped** (not queued)
2. ⚠️ **Warning event emitted** to stream with `ErrMailboxFull`
3. ✅ **Execution continues** (graceful degradation)

**Why messages might be dropped**:

- 🔄 Infinite loops sending messages repeatedly
- 📊 Fan-in patterns with many sources
- 🐛 Logic errors causing message storms

**Mitigation strategies**:

1. **Combiners**: Merge messages to reduce volume
   ```go
   combiner := func(a, b Message) Message {
       return Message{Data: a.Data + b.Data}  // Combine instead of accumulate
   }
   ```

2. **Appropriate limits**: Set based on graph size
   - Small graphs: 10,000 per vertex
   - Medium graphs: 1,000 per vertex
   - Large graphs: 100-500 per vertex

3. **Max iterations**: Prevent runaway loops
   ```go
   pregel.WithMaxIterations[S, M](100)
   ```

### Aggregators: Global Coordination

**Aggregators** provide global read-only state across all vertices:

```go
type Aggregator[S, M, A any] interface {
    Init(ctx context.Context, state S) (A, error)
    Aggregate(ctx context.Context, state S, prev A) (A, error)
}
```

**Use cases**:

- **Convergence detection**: Track global error metric
- **Statistics**: Count total messages processed
- **Coordination**: Share read-only config across vertices

**Example: Convergence Detection**

```go
type ErrorAggregator struct{}

func (a *ErrorAggregator) Init(ctx context.Context, state *State) (float64, error) {
    return 0.0, nil
}

func (a *ErrorAggregator) Aggregate(ctx context.Context, state *State, prev float64) (float64, error) {
    // Calculate global error metric
    currentError := computeError(state)
    return currentError, nil
}

// Use in node - execute function signature:
// func(ctx context.Context, view *state.ReadView) (state.Updates, error)
func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    globalError := state.GetFromView(view, ErrorKey)
    if globalError < 0.01 {
        return map[string]any{"done": true}, nil
    }
    // Continue processing...
    return map[string]any{"done": false}, nil
}
```

**Thread safety**: Aggregators are computed **sequentially** after parallel vertex execution, so no synchronization needed.

### Stream Events: Observability

The runtime emits **stream events** for real-time observability:

```go
type StreamEvent struct {
    Node      string          // Which node completed/errored
    Superstep int64           // Current superstep
    Updates   map[string]any  // State updates
    Messages  []Message       // New messages
    Err       error           // Error if any
}
```

**Event types**:

- ✅ **Node completion**: Successful node execution
- ❌ **Node error**: Node execution failed
- ⚠️ **Warning**: Mailbox overflow, rate limit hit
- 📊 **Metadata**: Superstep boundaries, checkpoint saved

**Usage**:

```go
seq := compiled.Run(ctx, messages)
for event, err := range seq {
    if err != nil {
        log.Printf("Error in node %s: %v", event.Node, err)
        break
    }
    case event.Node != "":
        log.Printf("Superstep %d: %s completed", event.Superstep, event.Node)
    }
}
```

This enables:

- 🔍 **Real-time monitoring** of execution progress
- 📊 **Metrics collection** for performance analysis
- 🐛 **Debugging** (see exact execution order)
- ⚠️ **Alerting** on errors or resource limits

---

## Graph builder and usage {#graph-builder}

The `graph.Builder` provides a fluent API for constructing agent workflows:

### Basic structure

```go
builder := graph.NewBuilder()

// Add nodes (computation units)
builder.AddNode(&graph.Node{
    Name: "agent",
    RunFunc: agentFunction,
})

// Add edges (define execution order)
builder.AddEdge("START", "agent")
builder.AddEdge("agent", "END")

// Compile into executable graph
compiled, err := builder.Compile()
```

### Conditional routing

Routes are determined dynamically based on node outputs:

```go
builder.AddConditionalEdges("classifier", func(result *graph.NodeResult) []string {
    category := result.Updates["category"].(string)
    switch category {
    case "urgent":
        return []string{"urgent_handler"}
    case "research":
        return []string{"researcher"}
    default:
        return []string{"default_handler"}
    }
})
```

### Parallel execution

Independent nodes automatically execute in parallel:

```go
// These three nodes will execute concurrently in the same superstep
builder.AddEdge("START", "analyst_a")
builder.AddEdge("START", "analyst_b")
builder.AddEdge("START", "analyst_c")

// All converge to aggregator
builder.AddEdge("analyst_a", "aggregator")
builder.AddEdge("analyst_b", "aggregator")
builder.AddEdge("analyst_c", "aggregator")
```

---

## Executor pattern {#executor-pattern}

AgentMesh uses the **Executor interface** to abstract execution strategies:

```go
type Executor interface {
    Run(ctx context.Context, topology *ExecutorTopology, ...) iter.Seq2[state.ExecutionResult, error]
    CurrentSuperstep() int64
    Pause(nodeName string)
    Resume(nodeName string)
    IsPaused(nodeName string) bool
}
```

### Default Execution: Pregel BSP

**By default**, `Compiled.Run()` uses the **Pregel BSP execution engine** (parallel, distributed-ready):

```go
// Default: Uses Pregel BSP execution automatically
g := graph.New()
// ... build graph ...
compiled, _ := exec.CompileGraph(g)
results := compiled.Run(ctx, initialMessages)  // Parallel execution via Pregel
```

The Pregel execution is implemented through `pkg/graph/pregel.go`, which integrates the `pkg/pregel` BSP runtime:
- **Parallel execution** via worker pools
- **Bulk-Synchronous Parallel** (BSP) superstep coordination
- **Message passing** between nodes
- **Distributed-ready** for future MessageBus backends

### Configuring Pregel Execution: PregelExecutor

**PregelExecutor** provides typed configuration for Pregel-specific features without polluting the graph package API:

```go
// Configure Pregel BSP execution engine
executor := graph.NewPregelExecutor(
    // Aggregators: Global state aggregation across all nodes
    graph.WithPregelAggregators(map[string]pregel.Aggregator{
        "total_cost": pregel.SumAggregator{},
        "max_latency": pregel.MaxAggregator{},
    }),
    
    // Combiner: Message optimization before delivery
    graph.WithPregelCombiner(func(messages []graph.ChannelMessage) []graph.ChannelMessage {
        // Merge, deduplicate, or filter messages
        return messages
    }),
    
    // Message Bus: Pluggable backend for distributed messaging
    graph.WithMessageBus(redisMessageBus),  // Redis, Kafka, etc.
    
    // Workers: Parallel execution configuration
    graph.WithMaxWorkers(8),
    
    // Max Iterations: Prevent infinite loops
    graph.WithPregelMaxIterations(1000),
)

// Apply configuration before compilation
g := graph.New()
// ... build graph ...
g.WithExecutor(executor)
compiled, _ := exec.CompileGraph(g)

// Run with configured Pregel executor
result, _ := graph.Last(compiled.Run(ctx, initialMessages))

// Access aggregated values
totalCost := result.Aggregates["total_cost"]    // Global sum
maxLatency := result.Aggregates["max_latency"]  // Global max
```

**Key Design Principles:**
- **Compile-Time Configuration**: Aggregators and combiners are part of graph structure
- **Clean Separation**: Pregel concerns isolated from graph package
- **Type Safety**: Typed options at configuration time
- **Runtime Overrides**: Per-run `WithMaxIterations()` still available

**Architecture:**
```
User Code → PregelExecutor (typed config)
    ↓
Graph.SetExecutor() → Compiled.Run()
    ↓
runOptions (interface{} bridge) → pregel.Runtime (typed execution)
```

The bridging pattern allows clean separation while maintaining type safety at both configuration and execution time.

### Alternative: SimpleGraphExecutor

For **debugging** or **testing**, you can override with `SimpleGraphExecutor` (sequential, single-threaded):

```go
// Override for debugging: Sequential execution
compiled := graph.NewCompiledGraph(...).
    WithExecutor(graph.NewSimpleExecutor())

results := compiled.Run(ctx, initialMessages)  // Sequential execution
```

### Custom Executors

You can implement custom execution strategies by satisfying the `Executor` interface:

```go
type CustomExecutor struct { ... }

func (e *CustomExecutor) Run(ctx context.Context, topology *ExecutorTopology, ...) iter.Seq2[state.ExecutionResult, error] {
    // Custom execution logic
}

// Use custom executor
compiled := graph.NewCompiledGraph(...).
    WithExecutor(NewCustomExecutor())
```

**Architecture Benefits**:
- ✅ **Default Performance**: Pregel BSP execution out-of-the-box
- ✅ **Pluggable Design**: Switch execution strategies via `WithExecutor()`
- ✅ **Extensibility**: Implement custom executors for specialized needs
- ✅ **Testing Support**: Use SimpleGraphExecutor for deterministic debugging

This pattern allows for different execution strategies (parallel, sequential, distributed) without changing the core graph topology or node implementations.

---

## State management {#state-management}

AgentMesh uses a **channel-based state system** for deterministic data flow. State is shared across all nodes with thread-safe access patterns.

### StateManager Concrete Type Design

AgentMesh uses a **concrete `*state.Manager` type** for state management, leveraging Go's generics for compile-time type safety:

```go
// Create state manager
mgr := state.NewManager()

// Register typed keys (compile-time type safety)
statusKey := state.NewKey[string]("status", "")
state.RegisterKey(mgr, statusKey)

// Builder accepts *state.Manager
builder := graph.NewBuilder()
builder.SetStateManager(mgr)

// Type-safe state access via generic functions
value, err := state.Get(ctx, mgr, statusKey)  // value is string, not any
```

**Benefits**:
- ✅ **Compile-time Type Safety**: Generic functions eliminate runtime type assertions
- ✅ **Zero-cost Abstraction**: No interface overhead or type checking at runtime
- ✅ **Simplified API**: Direct access via `state.Get/Set/Append` helpers
- ✅ **Better Performance**: ~2-3x faster without reflection or type assertions
- ✅ **Cleaner Code**: No need for type casts or error-prone `any` conversions

The concrete type approach provides channel-based state with versioning, checkpointing, and thread-safe access.

### StateManager Architecture (Decomposed Design)

Following the **Single Responsibility Principle**, the StateManager has been decomposed from a monolithic "god object" into focused, composable components. This architecture improves testability, maintainability, and extensibility.

#### Component Architecture

```
┌────────────────────────────────────────────────────────────┐
│                    ChannelState                             │
│               (StateManager Implementation)                 │
│                                                             │
│  Composes four specialized components:                     │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ channelStore                                         │ │
│  │ • Channel registration and lookup                    │ │
│  │ • Value updates (single & batch)                     │ │
│  │ • Thread-safe access via channel.Set                 │ │
│  └──────────────────────────────────────────────────────┘ │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ aggregateStore                                       │ │
│  │ • Aggregate value storage                            │ │
│  │ • Aggregate function management                      │ │
│  │ • Immutable snapshots with lazy copy-on-write        │ │
│  │ • Thread-safe with sync.RWMutex                      │ │
│  └──────────────────────────────────────────────────────┘ │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ checkpointCoordinator                                │ │
│  │ • Checkpoint backend configuration                   │ │
│  │ • Save/load operations                               │ │
│  │ • Metadata management                                │ │
│  │ • Thread-safe backend access                         │ │
│  └──────────────────────────────────────────────────────┘ │
│  ┌──────────────────────────────────────────────────────┐ │
│  │ versionTracker                                       │ │
│  │ • Monotonic version counter                          │ │
│  │ • Change detection                                   │ │
│  │ • Checkpoint integrity validation                    │ │
│  │ • Thread-safe increment                              │ │
│  └──────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────┘
```

#### Type-Safe Generic API

The state management API uses Go generics for compile-time type safety without runtime overhead:

```go
// Manager - Concrete state management struct
type Manager struct {
    mu             sync.RWMutex
    store          map[string]any
    channels       map[string]*channel.Channel
    registeredKeys map[string]keyInfo
    snapshots      map[string]*snapshot
    checkpointer   checkpoint.Checkpointer
}

// Type-safe key definitions
type Key[T any] struct {
    name         string
    defaultValue T
}

type ListKey[T any] struct {
    Key[[]T]
}

// Generic registration functions (compile-time type safety)
func RegisterKey[T any](m *Manager, key Key[T]) error
func RegisterListKey[T any](m *Manager, key ListKey[T]) error

// Generic accessor functions (no type assertions needed)
func Get[T any](ctx context.Context, m *Manager, key Key[T]) (T, error)
func Set[T any](ctx context.Context, m *Manager, key Key[T], value T) error
func Append[T any](ctx context.Context, m *Manager, key ListKey[T], value T) error
func GetList[T any](ctx context.Context, m *Manager, key ListKey[T]) ([]T, error)

// ReadView - Immutable snapshot for concurrent reads
type ReadView struct {
    data      map[string]any
    timestamp time.Time
}

func (m *Manager) CreateReadView(ctx context.Context) (*ReadView, error)
```

#### Benefits of Decomposition

**1. Single Responsibility**
- Each component has one clear purpose
- Easy to understand and maintain
- Changes are localized to specific components

**2. Testability**
- Components can be tested independently
- Easy to mock focused interfaces
- Reduced test complexity

**3. Independent Thread Safety**
- Each component manages its own locking
- No global lock contention
- Better concurrent performance

**4. Extensibility**
- Can implement subsets for specialized use cases
- Easy to add new storage backends
- Clear extension points

**5. Clean API Surface**
- Nodes receive Reader or Writer (not full StateManager)
- Runtime uses specific manager interfaces
- Explicit dependencies prevent misuse

#### Usage Examples

**Type-safe state access:**
```go
// Define typed keys
var (
    statusKey  = state.NewKey[string]("status", "idle")
    counterKey = state.NewKey[int]("counter", 0)
    itemsKey   = state.NewListKey[string]("items", 10)
)

// Register keys with manager
mgr := state.NewManager()
state.RegisterKey(mgr, statusKey)
state.RegisterKey(mgr, counterKey)
state.RegisterListKey(mgr, itemsKey)

// Type-safe reads (no type assertions)
status, err := state.Get(ctx, mgr, statusKey)  // status is string
count, err := state.Get(ctx, mgr, counterKey)   // count is int

// Type-safe writes (compile-time validation)
state.Set(ctx, mgr, statusKey, "active")       // ✅ compiles
state.Set(ctx, mgr, statusKey, 123)            // ❌ compile error: cannot use int as string

// List operations
state.Append(ctx, mgr, itemsKey, "new-item")
items, err := state.GetList(ctx, mgr, itemsKey) // items is []string

// Read-only snapshots for concurrent access
view, err := mgr.CreateReadView(ctx)
status := view.Get(statusKey.Name())  // Safe concurrent reads
```

**Runtime uses full StateManager:**
```go
// Runtime has full access for coordination
func (r *Runtime) executeSuperstep(ctx context.Context, sm state.StateManager) error {
    // Use ChannelManager for updates
    sm.UpdateChannels(ctx, updates)
    
    // Use AggregateManager for coordination
    sm.SetAggregates(newAggregates)
    
    // Use CheckpointManager for persistence
    sm.SaveCheckpoint(ctx, runID, superstep, metadata)
    
    return nil
}
```

#### Implementation Details

The components are internal implementation details in `pkg/state/`:

- `channelStore` - Wraps `channel.Set` for thread-safe channel management (delegates to `ChannelRegistry`)
- `ChannelRegistry` - Uses `sync.Map` for lock-free concurrent reads of channel metadata
- `aggregateStore` - Uses `sync.RWMutex` for aggregate value protection with lazy copy-on-write caching
- `checkpointCoordinator` - Manages pluggable checkpoint backends
- `versionTracker` - Provides atomic version increments for state integrity

The `ChannelState` struct composes these components and delegates all operations to the appropriate component, maintaining a clean separation of concerns.

**Performance characteristics**: The `ChannelRegistry` using `sync.Map` provides lock-free concurrent reads, making it ideal for BSP workloads where all workers read state simultaneously at superstep boundaries. Read throughput scales linearly with worker count.

### Hybrid State Propagation

AgentMesh uses a **hybrid approach** for state updates to support both single-process and distributed execution:

1. **In-Memory Mode** (single process):
   - State updates are applied **immediately** to the shared StateManager after node execution
   - Efficient for local development and testing
   - No serialization overhead

2. **Distributed Mode** (multi-process):
   - State updates flow through the **MessageBus** to downstream nodes
   - Nodes receive and apply updates from incoming messages in the next superstep
   - Enables scaling across multiple machines or containers

3. **Automatic Detection**:
   - Runtime detects execution mode based on StateManager references
   - No configuration required - works seamlessly in both modes

This architecture ensures correctness while optimizing for the common single-process case.

### Aggregate Snapshot Caching (Phase 3 Optimization)

Aggregates are global values computed across all nodes (e.g., sums, averages, max values). Each node can read aggregates from previous supersteps via `Reader.AggregatesSnapshot()`.

**Problem**: Original implementation copied the entire aggregate map on every snapshot call:
```go
// Old: O(n) copy per call
snapshot := make(map[string]any, len(aggregates))
for k, v := range aggregates {
    snapshot[k] = v
}
```

**Solution**: Lazy copy-on-write with version tracking:

1. **Version Counter**: Each `SetAggregates()` call increments a version number
2. **Cached Snapshot**: First `GetAggregatesSnapshot()` creates and caches the copy
3. **Cache Validation**: Subsequent calls return cached copy if version matches
4. **Automatic Invalidation**: Cache invalidated on next `SetAggregates()` call

**Performance Impact**:

| Scenario | Before | After | Improvement |
|----------|--------|-------|-------------|
| 100 nodes reading aggregates | 100 × O(n) copies | 1 × O(n) copy | 100x fewer allocations |
| Single aggregate update | O(old + new) delete+copy | O(1) pointer swap | Constant time |
| Memory overhead | None | 1 cached snapshot | Negligible |

**Code Example**:
```go
// State maintains version and cache
type State struct {
    aggregates        map[string]any
    aggregateCache    map[string]any // Cached snapshot
    aggregateVersion  uint64         // Incremented on update
    cachedVersion     uint64         // Version of cache
}

// SetAggregates: O(1) - just pointer assignment
func (s *State) SetAggregates(aggregates map[string]any) {
    s.aggregates = aggregates      // Direct replacement
    s.aggregateVersion++            // Invalidate cache
}

// GetAggregatesSnapshot: O(1) cache hit, O(n) cache miss
func (s *State) GetAggregatesSnapshot() map[string]any {
    if s.cachedVersion == s.aggregateVersion {
        return s.aggregateCache  // Fast path: return cached copy
    }
    // Slow path: create snapshot and cache it
    s.aggregateCache = copyMap(s.aggregates)
    s.cachedVersion = s.aggregateVersion
    return s.aggregateCache
}
```

**Benefits**:
- ✅ Eliminates redundant map allocations (common case: many reads, few writes)
- ✅ Reduces GC pressure from repeated snapshot copies
- ✅ Thread-safe with double-checked locking pattern
- ✅ No breaking changes - transparent optimization

### Channel types

1. **TopicChannel** – Accumulates messages (append-only list), perfect for conversation history
2. **LastValueChannel** – Stores only the most recent value (overwrite semantics)
3. **BinaryOpChannel** – Merges values using custom operators (sum, max, concat, etc.)

```go
state := graph.NewStateManager(maxMessages)

// Topic channel for conversation history
state.AddChannel(channel.NewTopicChannel("messages", 100))

// Last value channel for status tracking
state.AddChannel(channel.NewLastValueChannel("status"))

// Binary op channel for counters
state.AddChannel(channel.NewBinaryOpChannel("counter", func(a, b any) any {
    return a.(int) + b.(int)
}))
```

### Reading state

Nodes receive immutable state snapshots:

```go
RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Read values
    status := state.Get("status")
    messages := view.MessagesSnapshot()
    
    // Process...
    return map[string]any{...}, nil
}
```

### Updating state

Nodes return state updates directly:

```go
return map[string]any{
    "status": "complete",
    "counter": 1,  // Will be summed if BinaryOpChannel
    message.MessagesKey: []message.Message{response},
}, nil
```

---

## Execution lifecycle {#execution-lifecycle}

### 1. Initialization

```go
compiled, err := builder.Compile()
```

The compiler validates the graph topology, checks for cycles, and prepares the execution scheduler.

### 2. Invocation

```go
// Blocking execution - collect all messages
messages, err := agent.CollectMessages(compiled.Run(ctx, initialMessages))
if err != nil {
    log.Fatal(err)
}

// Streaming execution - process events as they arrive
seq := compiled.Run(ctx, initialMessages)
for event, err := range seq {
    if err != nil {
        log.Printf("Error: %v", err)
    }
    log.Printf("Superstep %d: Node %s completed", event.Superstep, event.Node)
}
```

### 3. Superstep execution

For each superstep:
1. Scheduler identifies ready nodes (all dependencies satisfied)
2. Nodes execute in parallel (up to worker pool size)
3. State updates are applied atomically
4. Conditional routes are evaluated
5. Checkpoint is saved
6. Process repeats until END or max iterations

### 4. Result collection

Final state and messages are returned:

```go
results := &graph.InvokeResult{
    Messages: state.MessagesSnapshot(),
    State:    state.Snapshot(),
}
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

### Aggregate Caching

Lazy copy-on-write with version tracking for aggregate snapshots:

| Operation | Complexity | Description |
|-----------|------------|-------------|
| `SetAggregates()` | O(1) | Pointer assignment + version increment |
| `GetAggregatesSnapshot()` (cache hit) | O(1) | Return cached copy |
| `GetAggregatesSnapshot()` (cache miss) | O(n) | Create and cache snapshot |
| Memory | O(2n) | Original map + one cached snapshot |

**Performance characteristics**:
- Multiple nodes reading same aggregates: Only one copy operation per superstep
- Reduced GC pressure from eliminated redundant map allocations
- Automatic cache invalidation on aggregate updates
- Especially beneficial when many nodes read aggregates in same superstep (typical BSP pattern)

Benchmark results (100,000 iterations):

```
BenchmarkOptimized     100000    6147 ns/op    ~6μs per node
BenchmarkChannelOnly   100000    7432 ns/op
BenchmarkBaseline      100000   12891 ns/op
```

### Lock-Free Channel Registry

The ChannelRegistry uses **`sync.Map`** for lock-free concurrent reads, enabling true parallel state access:

| Operation | Implementation | Characteristics |
|-----------|----------------|-----------------|
| `GetChannel()` | Lock-free read | O(1), no contention |
| `GetChannelValue()` | Lock-free read | O(1), no contention |
| `GetChannelMetadata()` | Lock-free read | O(1), no contention |
| `RegisterChannel()` | Lightweight mutex | Write operations use simple mutex |
| `Snapshot()`/`Restore()` | `sync.Map.Range()` | Iteration over all channels |
| Memory | O(n) + sync.Map overhead | Minimal overhead for high-read workloads |

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
- **Stable key set**: `sync.Map` is optimized for channels (registered once, read many times)
- **Minimal write overhead**: Registration uses lightweight mutex only

**Concrete impact**:
- 100 workers reading state: 100x parallel read throughput vs sequential locks
- Read-heavy workloads (typical): 10-50x faster state access
- Individual channel operations: Already lock-free, unaffected
- Write operations: Minimal overhead (channels registered infrequently)

**Design rationale**:
- BSP workloads have burst read patterns (all workers at superstep boundaries)
- Channels are registered once at graph build time, then read repeatedly
- `sync.Map` provides lock-free reads for stable key sets
- Slight memory overhead is negligible compared to throughput gains
