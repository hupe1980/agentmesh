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

AgentMesh follows a **component-based architecture** with clean separation of concerns:

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
│                    Compiled (Coordinator)                │
│  • Immutable graph topology (nodes, edges, conditionals)      │
│  • Public API (Invoke, Stream, Pause, Resume)                │
│  • Coordinates StateManager ↔ Executor                        │
│  • Rate limiting & retry policies                             │
└────────────────┬─────────────────────────┬───────────────────┘
                 │                         │
                 │ delegates to            │ delegates to
                 ▼                         ▼
    ┌────────────────────────┐  ┌────────────────────────────┐
    │    StateManager        │  │       Executor             │
    │    (Interface)         │  │       (Interface)          │
    │                        │  │                            │
    │  • Channels            │  │  • Execution Strategy      │
    │  • Checkpoints         │  │  • Superstep Coordination  │
    │  • Aggregates          │  │  • Event Streaming         │
    │  • Thread-safe access  │  │  • Pause/Resume Control    │
    │  • State versioning    │  │  • Execution Statistics    │
    └────────────────────────┘  └──────────┬─────────────────┘
                                           │
                                           │ implements
                                           ▼
                                  ┌──────────────────┐
                                  │ PregelExecutor   │
                                  │                  │
                                  │ • BSP Model      │
                                  │ • Worker Pool    │
                                  │ • Mailbox System │
                                  │ • pkg/pregel     │
                                  └──────────────────┘
```

**Key Design Principles:**
- **Separation of Concerns**: State, execution, and topology are independent
- **Interface-Based**: StateManager and Executor are interfaces for testability
- **Composition**: PregelExecutor wraps Compiled without modification
- **Extensibility**: Public `pkg/pregel` API for custom backends
- **Layered Abstraction**: High-level agents build on low-level graph primitives

The rest of this document explores the **Pregel BSP execution engine** that powers the framework.

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
// Superstep 0: Node A sends message
return &graph.NodeResult{
    NextNodes: []string{"node_b"},
    Updates: map[string]any{"data": result},
}, nil

// Superstep 1: Node B receives message in its mailbox
// The message is available via the StateReader
```

**Mailbox Bounds**: To prevent memory exhaustion, mailboxes can be configured with size limits:

```go
// Configure mailbox size (default: unlimited)
runtime := pregel.NewRuntime(graph, state,
    pregel.WithMaxMailboxSize[*State, ChannelMessage](1000),
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
builder.AddNode(&graph.Node{
    Name: "writer",
    RunFunc: func(ctx context.Context, state graph.StateReader) (*graph.NodeResult, error) {
        draft := generateDraft()
        return &graph.NodeResult{
            Updates: map[string]any{"draft": draft},
            NextNodes: []string{"evaluator"},
        }, nil
    },
})

builder.AddNode(&graph.Node{
    Name: "evaluator",
    RunFunc: func(ctx context.Context, state graph.StateReader) (*graph.NodeResult, error) {
        draft := state.Get("draft")
        if isGoodEnough(draft) {
            return &graph.NodeResult{NextNodes: []string{"END"}}, nil
        }
        // Send feedback and loop back to writer
        return &graph.NodeResult{
            Updates: map[string]any{"feedback": "improve clarity"},
            NextNodes: []string{"writer"},  // Creates a cycle!
        }, nil
    },
})
```

This creates a loop where the writer improves the draft based on evaluator feedback, executing over multiple supersteps until quality is acceptable.

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

builder := graph.NewBuilder()

// Nodes execute in parallel when possible
builder.AddNode(&graph.Node{
    Name: "fetch_data",
    RunFunc: func(ctx context.Context, state graph.StateReader) (*graph.NodeResult, error) {
        // Fetch from API...
        return &graph.NodeResult{
            Updates: map[string]any{"data": result},
        }, nil
    },
})

builder.AddNode(&graph.Node{
    Name: "process",
    RunFunc: func(ctx context.Context, state graph.StateReader) (*graph.NodeResult, error) {
        data := state.Get("data")
        // Process...
        return &graph.NodeResult{
            Updates: map[string]any{"processed": true},
        }, nil
    },
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

1. Nodes can return **dynamic next nodes** in their `NodeResult`
2. Conditional edges are evaluated based on node output
3. The evaluator maintains "gate status" for conditional branches
4. A node with conditional incoming edges only executes when its gate is open

**Example**:

```go
builder.AddNode(&graph.Node{
    Name: "classifier",
    RunFunc: func(ctx context.Context, state graph.StateReader) (*graph.NodeResult, error) {
        category := analyzeInput(state.MessagesSnapshot())
        
        // Dynamic routing based on classification
        var nextNodes []string
        if category == "urgent" {
            nextNodes = []string{"urgent_handler"}
        } else {
            nextNodes = []string{"standard_handler"}
        }
        
        return &graph.NodeResult{
            NextNodes: nextNodes,  // Conditional routing
        }, nil
    },
})

// Alternative: Use conditional edge function
builder.AddConditionalEdges("classifier", func(result *graph.NodeResult) []string {
    category := result.Updates["category"].(string)
    // Return different paths based on runtime data
    return []string{category + "_handler"}
})
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
// Pause execution before a node
compiled.Pause("review_node")

// Execute graph (will stop before review_node)
results, err := compiled.Invoke(ctx, messages)

// Human reviews and provides input
// ...

// Resume execution
compiled.Resume("review_node")
results, err = compiled.Invoke(ctx, messages)
```

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

The runtime uses a **bounded worker pool** to control parallelism:

```go
// Configuration
opts := pregel.RuntimeOptions[S, M]{
    MaxWorkers: 10,  // Max concurrent vertex executions
}

// Execution
vertices := scheduler.Ready()  // May return 100 vertices

// Only 10 execute concurrently (others queue)
semaphore := make(chan struct{}, MaxWorkers)
for _, vertex := range vertices {
    semaphore <- struct{}{}  // Acquire slot
    go func(v string) {
        defer func() { <-semaphore }()  // Release slot
        executeVertex(v)
    }(vertex)
}
```

**Benefits**:

- ⚡ **Parallel execution** when topology allows
- 🔒 **Resource control** (limit memory/CPU usage)
- 📊 **Predictable performance** (no unbounded goroutine spawning)

**Tuning guidance**:

- **CPU-bound nodes**: `MaxWorkers = runtime.NumCPU()`
- **I/O-bound nodes** (API calls): `MaxWorkers = 2-4x runtime.NumCPU()`
- **Mixed workload**: `MaxWorkers = runtime.NumCPU() + small buffer`

### Message Passing and Mailboxes

Each vertex has a **mailbox** that stores messages sent to it:

```go
type Runtime[S, M] struct {
    mailbox map[string][]M  // vertex name → messages
    mu      sync.Mutex      // Protects mailbox
}
```

**Message lifecycle**:

```
Superstep N:
  Vertex A executes
  ├─ Returns NodeResult{NextNodes: ["B"], Updates: {...}}
  └─ Runtime calls recordDeliveries()
      └─ Appends message to mailbox["B"]

Superstep N+1:
  Vertex B executes
  ├─ Reads mailbox["B"] (contains message from A)
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

// Use in node
RunFunc: func(ctx context.Context, state graph.StateReader) (*graph.NodeResult, error) {
    globalError := state.GetAggregate("error").(float64)
    if globalError < 0.01 {
        return &graph.NodeResult{NextNodes: []string{"END"}}, nil
    }
    // Continue processing...
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
stream := compiled.Stream(ctx, messages)
for event := range stream {
    switch {
    case event.Err != nil:
        log.Printf("Error in node %s: %v", event.Node, event.Err)
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
    Execute(ctx context.Context, initialMessages []Message, opts ExecuteOptions) (*InvokeResult, error)
    Stream(ctx context.Context, initialMessages []Message, opts StreamOptions) <-chan StreamEvent
    Pause(nodeName string) error
    Resume(nodeName string) error
    IsPaused(nodeName string) bool
    CurrentSuperstep() int64
    Stats() ExecutionStats
}
```

### PregelExecutor Implementation

The default implementation uses **composition over inheritance**:

```go
type PregelExecutor struct {
    cg *Compiled  // Wraps Compiled
}

// Delegates to proven Compiled methods
func (e *PregelExecutor) Execute(ctx context.Context, messages []Message, opts ExecuteOptions) (*InvokeResult, error) {
    return e.cg.invokeWithOptions(ctx, messages, convertOptions(opts))
}
```

**Architecture Benefits**:
- ✅ **Clean Separation**: Executor doesn't modify Compiled internals
- ✅ **No Circular Dependencies**: Composition pattern prevents cycles
- ✅ **Extensibility**: Can implement custom execution strategies
- ✅ **Testability**: Mock executors for unit tests

This pattern allows for future execution strategies (e.g., distributed executor, streaming executor) without changing the core graph engine.

---

## State management {#state-management}

AgentMesh uses a **channel-based state system** for deterministic data flow. State is shared across all nodes with thread-safe access patterns.

### StateManager Interface Pattern

AgentMesh uses the **StateManager interface** to provide clean abstraction for state management:

```go
// Create state using the interface
state := graph.NewStateManager(maxMessages)

// Builder accepts StateManager interface
builder := graph.NewBuilder()
builder.SetStateManager(state)

// Compiled.State() returns StateManager interface
compiled, _ := builder.Compile()
stateReader := compiled.State()
```

**Benefits**:
- ✅ **Testability**: Easy to mock state for unit tests
- ✅ **Extensibility**: Can implement custom state backends
- ✅ **Clean API**: Interface over concrete implementation
- ✅ **Type Safety**: Go interfaces with compile-time checking

The default implementation (`State`) provides channel-based state with versioning and checkpoint support.

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

Aggregates are global values computed across all nodes (e.g., sums, averages, max values). Each node can read aggregates from previous supersteps via `StateReader.AggregatesSnapshot()`.

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
RunFunc: func(ctx context.Context, state graph.StateReader) (*graph.NodeResult, error) {
    // Read values
    status := state.Get("status")
    messages := state.MessagesSnapshot()
    
    // Process...
    return &graph.NodeResult{...}, nil
}
```

### Updating state

Nodes update state via `NodeResult`:

```go
return &graph.NodeResult{
    Messages: []message.Message{response},
    Updates: map[string]any{
        "status": "complete",
        "counter": 1,  // Will be summed if BinaryOpChannel
    },
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
// Synchronous execution
results, err := compiled.Invoke(ctx, initialMessages)

// Streaming execution
stream := compiled.Stream(ctx, initialMessages)
for event := range stream {
    if event.Err != nil {
        log.Printf("Error: %v", event.Err)
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
- **O(1) ready vertex lookup** – Maintained ready queue eliminates O(n) iteration (Phase 2)
- **O(1) aggregate updates** – Lazy copy-on-write caching (Phase 3)
- **Parallel node execution** – Independent nodes run concurrently
- **Lock splitting** – Reduced contention via channel-specific locks
- **Efficient checkpointing** – Copy-on-write state snapshots
- **Configurable workers** – Tune parallelism based on workload

### Scheduler Optimization (Phase 2)

The TopologyScheduler uses a **ready queue** for constant-time vertex lookup:

| Operation | Previous | Current | Improvement |
|-----------|----------|---------|-------------|
| `Ready()` | O(n) | O(1) | 100x faster for large graphs |
| `MarkExecuted()` | O(d) | O(d log n) | Maintains sorted queue |
| Memory | O(n) | O(n + k) | k = ready vertices (small) |

**Impact on large graphs**:
- 10,000 nodes × 100 supersteps = 1M saved iterations
- Especially beneficial for iterative algorithms with many supersteps
- Memory overhead negligible (only ready vertices in queue)

### Aggregate Caching (Phase 3)

Lazy copy-on-write for aggregate snapshots:

| Operation | Previous | Current | Improvement |
|-----------|----------|---------|-------------|
| `SetAggregates()` | O(old + new) | O(1) | Pointer assignment |
| `GetAggregatesSnapshot()` (cached) | O(n) | O(1) | Return cached copy |
| `GetAggregatesSnapshot()` (miss) | O(n) | O(n) | Same (cache creation) |
| Memory | O(n) | O(2n) | One cached snapshot |

**Impact**:
- 100 nodes reading same aggregates: 100x fewer allocations
- Reduced GC pressure from redundant map copies
- Especially beneficial when many nodes read aggregates in same superstep

Benchmark results (100,000 iterations):

```
BenchmarkOptimized     100000    6147 ns/op    ~6μs per node
BenchmarkChannelOnly   100000    7432 ns/op
BenchmarkBaseline      100000   12891 ns/op
```
