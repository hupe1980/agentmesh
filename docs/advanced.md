---
layout: doc
title: Advanced Patterns
description: Advanced AgentMesh patterns including circuit breakers, aggregators, and subgraphs.
permalink: /advanced/
example: subgraph
hero:
  title: Advanced Patterns
  description: Leverage resilience middleware, state-based aggregators, and subgraph composition.
  primary_cta:
    label: Explore examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
  secondary_cta:
    label: Graph API →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph"
    external: true
sidebar:
  - title: Resilience Middleware
    url: "#resilience-middleware"
  - title: Circuit Breaker
    url: "#circuit-breaker"
  - title: Aggregators
    url: "#aggregators"
  - title: Custom Schedulers
    url: "#custom-schedulers"
  - title: Subgraphs
    url: "#subgraphs"
---

# Advanced Patterns

This guide covers advanced patterns for building robust, scalable AgentMesh applications.

{: .note }
> For state management patterns (checkpointing, time travel, message retention, human-in-loop), see **[State Management](/state-management/)**. For extensibility and observability, see **[Middleware System](/middleware/)**.

---

## Resilience Middleware {#resilience-middleware}

Use built-in middleware for automatic retries, circuit breakers, and rate limiting:

### Retry Middleware

Automatically retry failed model calls with exponential backoff:

```go
import (
    modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"
    "time"
)

// Create retry middleware with custom configuration
retry := modelmw.NewRetryMiddleware(
    modelmw.WithMaxRetries(3),
    modelmw.WithInitialBackoff(100*time.Millisecond),
    modelmw.WithMaxBackoff(10*time.Second),
    modelmw.WithBackoffMultiplier(2.0),
)

// Apply to agent
agent.NewReAct(model,
    agent.WithModelMiddleware(retry),
)
```

**Key Features**:
- **Exponential backoff**: Configurable multiplier (default 2.0)
- **Backoff limits**: Set initial and maximum backoff durations
- **Context-aware**: Respects context cancellation
- **Automatic**: Retries all iterator errors transparently

**Default Configuration**:
```go
RetryMiddleware{
    MaxRetries:     3,
    InitialBackoff: 100ms,
    MaxBackoff:     10s,
    Multiplier:     2.0,
}
```

**For Node-Level Retries**: Use `g.NodeWithRetry()` when adding nodes to retry specific operations.

---

## Circuit Breaker {#circuit-breaker}

The circuit breaker pattern prevents cascading failures when calling external services. Use the built-in `CircuitBreakerMiddleware` for tools:

```go
import (
    toolmw "github.com/hupe1980/agentmesh/pkg/tool/middleware"
    "time"
)

// Create circuit breaker
cb := toolmw.NewCircuitBreakerMiddleware(
    3,             // maxFailures before opening
    30*time.Second, // resetTimeout
)

// Apply to agent
agent.NewReAct(model,
    agent.WithTools(tools...),
    agent.WithToolMiddleware(cb),
)

// Monitor circuit state
state := cb.State()  // StateClosed, StateOpen, StateHalfOpen
cb.Reset()           // Manual reset
```

### Circuit States

- **StateClosed** - Normal operation, all requests pass through
- **StateOpen** - Fast fail after threshold exceeded, returns error immediately
- **StateHalfOpen** - Testing recovery, limited requests allowed

### How It Works

1. **Closed**: All tool calls execute normally
2. **Failure Tracking**: Each error increments failure count
3. **Opening**: After `maxFailures`, circuit opens
4. **Reset Timer**: After `resetTimeout`, transitions to half-open
5. **Testing**: In half-open, first success closes circuit
6. **Recovery**: Successful calls reset failure count

### Example

See [examples/middleware](https://github.com/hupe1980/agentmesh/tree/main/examples/middleware) for complete implementation.


## Aggregators & Global State {#aggregators}

### What are Aggregators?

Aggregators provide a mechanism for **global coordination** across all nodes in a graph by accumulating values during execution. They're implemented as special channels in AgentMesh's unified state system.

**Key characteristics**:
- **Global visibility**: All nodes can read the aggregated value
- **Accumulation semantics**: Values are combined using aggregator logic (sum, max, avg, etc.)
- **Type-safe**: Registered via aggregate keys
- **Channel-based**: Integrated with state management system

### Built-in Aggregators

Aggregators are available in the `pkg/pregel` package for use with the Pregel runtime:

#### Custom Aggregator Implementation

Define custom aggregators by implementing the `Aggregator` interface:

```go
import "github.com/hupe1980/agentmesh/pkg/pregel"

// SumAggregator accumulates numeric values
type SumAggregator struct{}

func (SumAggregator) Zero() any { return 0.0 }

func (SumAggregator) Aggregate(current, value any) any {
    return current.(float64) + value.(float64)
}

// Use with Pregel runtime
aggregators := map[string]pregel.Aggregator{
    "sum": SumAggregator{},
}
```

The `Aggregator` interface:

```go
type Aggregator interface {
    Zero() any                        // Identity element
    Aggregate(current, value any) any // Combine values
}
```

#### MinAggregator / MaxAggregator

Tracks the minimum or maximum value across all vertices:

```go
type MinAggregator struct{}

func (MinAggregator) Zero() any { return math.MaxFloat64 }

func (MinAggregator) Aggregate(current, value any) any {
    if value.(float64) < current.(float64) {
        return value
    }
    return current
}

// Contribute via graph.Set
g.Node("optimizer", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(MinCostKey, estimatedCost).
        Set(MaxPriorityKey, taskPriority).
        To(graph.END), nil
}, graph.END)
```

**Returns**: `float64` - Minimum or maximum value observed

#### AvgAggregator

Computes the running average of numeric values using Welford's algorithm for numerical stability:

```go
var AvgLatencyKey = graph.NewAggregateKey[aggregators.AvgState](
    "avg_latency",
    &aggregators.AvgAggregator{},
)

g := graph.New[string, string](AvgLatencyKey)

g.Node("monitor", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(AvgLatencyKey, responseTime).To(graph.END), nil
}, graph.END)

// Read result (returns AvgState)
// avgState := graph.Get(scope, AvgLatencyKey)
// average := avgState.Mean
// count := avgState.Count
```

**Returns**: `aggregators.AvgState{Mean: float64, Count: int64}` - Running mean and sample count

#### VarianceAggregator

Computes the variance of numeric values using Welford's algorithm:

```go
var VarianceKey = graph.NewAggregateKey[aggregators.VarianceState](
    "latency_variance",
    &aggregators.VarianceAggregator{},
)

g := graph.New[string, string](VarianceKey)

g.Node("stats", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(VarianceKey, responseTime).To(graph.END), nil
}, graph.END)

// Read result
// varState := graph.Get(scope, VarianceKey)
// variance := varState.M2 / float64(varState.Count)
// stdDev := math.Sqrt(variance)
```

**Returns**: `aggregators.VarianceState{Mean: float64, M2: float64, Count: int64}` - Mean, sum of squared differences (M2), and count

#### CountAggregator

Counts non-nil contributions:

```go
var ActiveNodesKey = graph.NewAggregateKey[int](
    "active_nodes",
    &aggregators.CountAggregator{},
)

g := graph.New[string, string](ActiveNodesKey)

g.Node("worker", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // Any non-nil value increments
    return graph.Set(ActiveNodesKey, 1).To(graph.END), nil
}, graph.END)
```

**Returns**: `int` - Total count

#### AllTrueAggregator / AnyTrueAggregator

Boolean aggregators for convergence detection and monitoring:

```go
var AllConvergedKey = graph.NewAggregateKey[bool](
    "all_converged",
    &aggregators.AllTrueAggregator{},
)

var HasErrorsKey = graph.NewAggregateKey[bool](
    "has_errors",
    &aggregators.AnyTrueAggregator{},
)

g := graph.New[string, string](AllConvergedKey, HasErrorsKey)

g.Node("validator", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(AllConvergedKey, isConverged).
        Set(HasErrorsKey, hasError).
        To(graph.END), nil
}, graph.END)

// Check convergence
// if graph.Get(scope, AllConvergedKey) {
//     // All nodes converged, can terminate early
// }
```

**Returns**: `bool` - Logical AND (AllTrue) or OR (AnyTrue)

### Using Aggregators in Nodes

Nodes contribute to aggregators via `graph.Set()` and read accumulated values via `graph.Get()`:

```go
g.Node("processor", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // Read current aggregated values
    totalProcessed := graph.Get(scope, TotalProcessedKey)
    avgLatency := graph.Get(scope, AvgLatencyKey)
    
    fmt.Printf("Progress: %v items, avg latency: %v\n", totalProcessed, avgLatency.Mean)
    
    // Process some items
    itemsProcessed := 42
    latency := 150.0
    
    // Contribute to aggregators via graph.Set
    return graph.Set(TotalProcessedKey, float64(itemsProcessed)).
        Set(AvgLatencyKey, latency).
        To(graph.END), nil
}, graph.END)
```

**Use cases**:
- Count total messages processed
- Track cumulative errors  
- Calculate global statistics (mean, variance, min/max)
- Monitor convergence criteria
- Distributed coordination and decision-making

### Custom Aggregators

Implement the `Aggregator` interface for custom reduction logic:

```go
type Aggregator interface {
    Zero() any
    Aggregate(current, value any) any
}
```

#### Example: Median Aggregator

Track values to compute median:

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

type MedianAggregator struct{}

type medianState struct {
    Values []float64
}

func (a *MedianAggregator) Zero() any {
    return medianState{Values: []float64{}}
}

func (a *MedianAggregator) Aggregate(current, value any) any {
    state := current.(medianState)
    if val, ok := value.(float64); ok {
        state.Values = append(state.Values, val)
    } else if val, ok := value.(int); ok {
        state.Values = append(state.Values, float64(val))
    }
    return state
}

// Usage
var MedianKey = graph.NewAggregateKey[medianState](
    "latency_median",
    &MedianAggregator{},
)

g := graph.New[string, string](MedianKey)

g.Node("collector", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(MedianKey, latency).To(graph.END), nil
}, graph.END)

// After execution, compute median from collected values
// ms := graph.Get(scope, MedianKey)
// sort.Float64s(ms.Values)
// median := ms.Values[len(ms.Values)/2]
```

#### Example: Histogram Aggregator

Build distribution of values:

```go
type HistogramAggregator struct {
    Bins []float64 // Bin boundaries
}

type histogramState struct {
    Counts []int
}

func (a *HistogramAggregator) Zero() any {
    return histogramState{Counts: make([]int, len(a.Bins)+1)}
}

func (a *HistogramAggregator) Aggregate(current, value any) any {
    state := current.(histogramState)
    val, ok := value.(float64)
    if !ok {
        return state
    }
    
    // Find appropriate bin
    bin := 0
    for i, boundary := range a.Bins {
        if val >= boundary {
            bin = i + 1
        } else {
            break
        }
    }
    state.Counts[bin]++
    
    return state
}

// Usage
var HistogramKey = graph.NewAggregateKey[histogramState](
    "response_time_histogram",
    &HistogramAggregator{Bins: []float64{100, 200, 500, 1000}},
)
```

### Advanced Patterns

#### Convergence Detection

Use aggregators to detect when a graph has converged:

```go
var GlobalErrorKey = graph.NewAggregateKey[float64](
    "global_error",
    &aggregators.SumAggregator{},
)

g := graph.New[string, string](GlobalErrorKey)

g.Node("optimizer", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    // Calculate local error
    localError := computeLocalError()
    
    // Check previous superstep's global error
    globalError := graph.Get(scope, GlobalErrorKey)
    if globalError < 0.001 {
        // Converged! Route to END
        return graph.Set(GlobalErrorKey, localError).To(graph.END), nil
    }
    
    // Continue processing
    return graph.Set(GlobalErrorKey, localError).To("optimizer"), nil
}, "optimizer", graph.END)
```

#### Distributed Counting

Track statistics across parallel branches:

```go
var SuccessCountKey = graph.NewAggregateKey[float64](
    "success_count",
    &aggregators.SumAggregator{},
)

var FailureCountKey = graph.NewAggregateKey[float64](
    "failure_count",
    &aggregators.SumAggregator{},
)

var TotalLatencyKey = graph.NewAggregateKey[float64](
    "total_latency",
    &aggregators.SumAggregator{},
)

g := graph.New[string, string](SuccessCountKey, FailureCountKey, TotalLatencyKey)

// Parallel worker nodes
g.Node("worker1", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    start := time.Now()
    result, err := doWork()
    latency := float64(time.Since(start).Milliseconds())
    
    if err != nil {
        return graph.Set(FailureCountKey, 1.0).
            Set(TotalLatencyKey, latency).
            To("reporter"), nil
    }
    return graph.Set(SuccessCountKey, 1.0).
        Set(TotalLatencyKey, latency).
        To("reporter"), nil
}, "reporter")

// Final reporting node
g.Node("reporter", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    successCount := graph.Get(scope, SuccessCountKey)
    failureCount := graph.Get(scope, FailureCountKey)
    totalLatency := graph.Get(scope, TotalLatencyKey)
    avgLatency := totalLatency / (successCount + failureCount)
    
    log.Printf("Success: %.0f, Failures: %.0f, Avg Latency: %.2fms",
        successCount, failureCount, avgLatency)
    
    return graph.To(graph.END), nil
}, graph.END)
```

### BSP Semantics & Aggregators

Understanding the BSP (Bulk Synchronous Parallel) model is key to using aggregators effectively:

#### Superstep Execution Model

```
Superstep N:
1. All nodes execute in parallel
2. Nodes contribute to aggregators via graph.Set()
3. Barrier: wait for all nodes to complete
4. Aggregate values computed by combining contributions
5. New aggregate values become visible

Superstep N+1:
1. Nodes read aggregates from superstep N via graph.Get()
2. Nodes contribute to aggregators for superstep N+1
3. ... repeat
```

#### Important Rules

1. **Contributions are isolated**: Values aggregated in superstep N are NOT visible until superstep N+1
2. **Thread-safe by design**: Aggregation happens after the barrier, no need for locks
3. **Multiple contributions**: Same node can set aggregate multiple times in one superstep
4. **Reset between retries**: If a node fails and retries, its aggregate contributions are cleared

### Performance Considerations

**When to use aggregators**:
- ✅ Global coordination needed (convergence, voting)
- ✅ Statistics across parallel branches
- ✅ Read-mostly workloads (read aggregate, contribute occasionally)

**When NOT to use aggregators**:
- ❌ High-frequency updates (use channels instead)
- ❌ Need immediate visibility (aggregates lag by one superstep)
- ❌ Complex data structures (keep aggregates simple)

**Best practices**:
- Keep aggregate values small (primitives or small structs)
- Minimize contributions per node (one or two per superstep)
- Use for coordination, not data passing (use channels for data flow)

### See Also

- [Architecture: Pregel BSP Model](architecture.md#pregel-bsp-model)
- [Examples: Parallel tasks with aggregators](https://github.com/hupe1980/agentmesh/tree/main/examples/parallel_tasks)
- [API Reference: Aggregator interface](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph#Aggregator)

---

## Custom Schedulers {#custom-schedulers}

Schedulers determine the execution order of vertices within each superstep. AgentMesh provides pluggable schedulers for different optimization strategies.

### Default: TopologicalScheduler

By default, vertices execute in lexicographic order (deterministic, predictable):

```go
import "github.com/hupe1980/agentmesh/pkg/pregel"

// Default scheduler (automatically used if not specified)
scheduler := pregel.NewTopologicalScheduler()

runtime, _ := pregel.NewRuntime(graph,
    pregel.WithScheduler(scheduler),
)
```

**Use cases**:
- ✅ Debugging (reproducible execution order)
- ✅ Testing (consistent results)
- ✅ Simple workflows (no priority requirements)

### Priority-Based Scheduling

Execute high-priority vertices first for critical path optimization:

```go
import "github.com/hupe1980/agentmesh/pkg/pregel"

// Define priorities (higher = more important)
priorities := map[string]int{
    "critical_llm_call": 100,
    "validation":        50,
    "logging":           10,
}

scheduler := pregel.NewPriorityScheduler(priorities, 50) // default=50

runtime, _ := pregel.NewRuntime(graph,
    pregel.WithScheduler(scheduler),
)
```

**Dynamic priority updates**:
```go
// Adjust priorities during execution
scheduler.SetPriority("urgent_task", 200)
priority := scheduler.GetPriority("urgent_task") // Returns 200
```

**Use cases**:
- ✅ Critical path optimization (blocking operations first)
- ✅ Cost-based execution (expensive operations early/late)
- ✅ User-defined importance (VIP requests first)

### Resource-Aware Scheduling

Order by resource consumption to maximize parallelism or reduce tail latency:

```go
import "github.com/hupe1980/agentmesh/pkg/pregel"

// Define resource costs (memory/CPU units)
costs := map[string]int{
    "llm_call":      100, // Expensive
    "validation":    10,  // Cheap
    "data_fetch":    50,  // Medium
}

// Low-cost first (maximize parallelism)
scheduler := pregel.NewResourceAwareScheduler(costs, 25, true)

// High-cost first (reduce tail latency)
// scheduler := pregel.NewResourceAwareScheduler(costs, 25, false)

runtime, _ := pregel.NewRuntime(graph,
    pregel.WithScheduler(scheduler),
)
```

**Dynamic cost updates**:
```go
// Adjust costs based on observed behavior
scheduler.SetResourceCost("llm_call", 150)
cost := scheduler.GetResourceCost("llm_call") // Returns 150
```

**Use cases**:
- ✅ Memory-constrained environments (small tasks first)
- ✅ CPU-bound workloads (distribute load evenly)
- ✅ Mixed workload optimization (I/O vs CPU separation)

### Custom Scheduler Implementation

Implement the `Scheduler` interface for advanced scheduling strategies:

```go
import (
    "context"
    "github.com/hupe1980/agentmesh/pkg/pregel"
)

type AdaptiveScheduler struct {
    executionTimes map[string]int64 // Track historical performance
}

func (s *AdaptiveScheduler) NextBatch(
    ctx context.Context,
    info pregel.SchedulerInfo,
) ([]string, error) {
    // Custom logic: Schedule fast tasks first
    batch := make([]string, 0, len(info.Frontier))
    for vertex := range info.Frontier {
        batch = append(batch, vertex)
    }
    
    sort.Slice(batch, func(i, j int) bool {
        return s.executionTimes[batch[i]] < s.executionTimes[batch[j]]
    })
    
    return batch, nil
}

func (s *AdaptiveScheduler) RecordCompletion(
    ctx context.Context,
    vertex string,
    info pregel.CompletionInfo,
) {
    // Learn from execution: update timing estimates
    s.executionTimes[vertex] = info.Duration
}
```

**SchedulerInfo provides**:
- `Frontier`: Vertices with pending messages
- `Superstep`: Current superstep number
- `Graph`: Topology access (outgoing edges, roots)
- `MessageCounts`: Messages per vertex

**CompletionInfo provides**:
- `Duration`: Execution time (nanoseconds)
- `MessagesSent`: Number of messages produced
- `Error`: Any error that occurred

### Performance Considerations

**Scheduler overhead**:
- O(n log n) for sorting-based schedulers
- Negligible for small graphs (< 100 vertices)
- Consider caching for large graphs (> 1000 vertices)

**When to use custom schedulers**:
- ✅ Performance-critical workflows
- ✅ Resource-constrained environments
- ✅ Dynamic workload patterns

**When to stick with default**:
- ✅ Simple workflows
- ✅ Debugging/testing
- ✅ Deterministic execution required

### See Also

- [Architecture: BSP Execution Model](architecture.md#pregel-bsp-model)
- [API Reference: Scheduler interface](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/pregel#Scheduler)
- [Core Concepts: Graph Execution](/core-concepts/#execution-model)

---

## Subgraphs {#subgraphs}

Subgraphs enable **hierarchical composition** by embedding compiled graphs as nodes within parent graphs. This pattern helps organize complex workflows into modular, reusable components.

### Basic Usage

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

// Define keys
var ValueKey = graph.NewKey[int]("value")
var ResultKey = graph.NewKey[int]("result")

// Create a subgraph that doubles the value
sub := graph.New[int, int](ValueKey, ResultKey)

sub.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    value := graph.Get(scope, ValueKey)
    doubled := value * 2
    return graph.Set(ResultKey, doubled).To(graph.END), nil
}, graph.END)

sub.Start("process")

// Compile the subgraph
compiledSub, _ := sub.Build()

// Create parent graph
parent := graph.New[int, int](ValueKey, ResultKey)

parent.Node("prepare", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
    return graph.Set(ValueKey, 21).To("doubler")
}, "doubler")

// Embed subgraph as a node using graph.Subgraph
parent.Node("doubler", graph.Subgraph(
    compiledSub,
    // InputMapper: pass the value from parent to subgraph
    func(ctx context.Context, scope graph.Scope[string]) (int, error) {
        return graph.Get(scope, ValueKey), nil
    },
    // OutputMapper: map subgraph result back to parent
    func(ctx context.Context, output int) (graph.Updates, error) {
        return graph.Updates{
            ResultKey.Name(): output,
        }, nil
    },
), graph.END)

parent.Start("prepare")
compiled, _ := parent.Build()

// Execute: 21 * 2 = 42
result, _ := graph.Last(compiled.Run(context.Background(), 0))
```

### State Mapping

Map parent state to subgraph state and back using `graph.Subgraph()` with InputMapper and OutputMapper functions:

```go
var DataKey = graph.NewKey[string]("data")
var InputKey = graph.NewKey[string]("input")
var OutputKey = graph.NewKey[string]("output")
var ProcessedDataKey = graph.NewKey[string]("processed_data")

parent := graph.New[string, string](DataKey, ProcessedDataKey)
sub := graph.New[string, string](InputKey, OutputKey)

// Build and compile the subgraph
compiledSub, _ := sub.Build()

// Use graph.Subgraph with mappers
parent.Node("processor", graph.Subgraph(
    compiledSub,
    // InputMapper: parent state -> subgraph input
    func(ctx context.Context, scope graph.Scope[string]) (string, error) {
        data := graph.Get(scope, DataKey)
        return data, nil
    },
    // OutputMapper: subgraph output -> parent state updates
    func(ctx context.Context, output string) (graph.Updates, error) {
        return graph.Updates{
            ProcessedDataKey.Name(): output,
        }, nil
    },
), graph.END)
```

**Type Aliases**: For cleaner code, AgentMesh provides type aliases:
- `graph.InputMapper[SI any]` - Maps parent state to subgraph input
- `graph.OutputMapper[SO any]` - Maps subgraph output to parent state updates

### Use Cases

**Multi-stage pipelines**:
```go
// Create separate graphs for each stage
validationSub, _ := createValidationGraph().Build()
enrichmentSub, _ := createEnrichmentGraph().Build()
analysisSub, _ := createAnalysisGraph().Build()

// Compose into pipeline
pipeline := graph.New[string, string](DataKey, ResultKey)

pipeline.Node("validate", graph.Subgraph(
    validationSub,
    func(ctx context.Context, scope graph.Scope[string]) (string, error) {
        return graph.Get(scope, DataKey), nil
    },
    func(ctx context.Context, output string) (graph.Updates, error) {
        return graph.Set(DataKey, output), nil
    },
), "enrich")

pipeline.Node("enrich", graph.Subgraph(
    enrichmentSub,
    func(ctx context.Context, scope graph.Scope[string]) (string, error) {
        return graph.Get(scope, DataKey), nil
    },
    func(ctx context.Context, output string) (graph.Updates, error) {
        return graph.Set(DataKey, output), nil
    },
), "analyze")

pipeline.Node("analyze", graph.Subgraph(
    analysisSub,
    func(ctx context.Context, scope graph.Scope[string]) (string, error) {
        return graph.Get(scope, DataKey), nil
    },
    func(ctx context.Context, output string) (graph.Updates, error) {
        return graph.Set(ResultKey, output), nil
    },
), graph.END)

pipeline.Start("validate")
compiled, _ := pipeline.Build()
```

**Reusable components**:
```go
// Create reusable authentication subgraph
authSub, _ := createAuthGraph().Build()

// Define standard mappers for auth flow
authInput := func(ctx context.Context, scope graph.Scope[string]) (AuthData, error) {
    return AuthData{Token: graph.Get(scope, TokenKey)}, nil
}
authOutput := func(ctx context.Context, output AuthResult) (graph.Updates, error) {
    return graph.Set(UserKey, output.User), nil
}

// Use in multiple parent graphs with the same mappers
apiGraph.Node("auth", graph.Subgraph(authSub, authInput, authOutput), "process")
adminGraph.Node("auth", graph.Subgraph(authSub, authInput, authOutput), "admin_process")
publicGraph.Node("auth", graph.Subgraph(authSub, authInput, authOutput), "public_process")
```

### Best Practices

- **Modular design**: Keep subgraphs focused on single responsibilities
- **State isolation**: Use state mapping to explicitly define data flow
- **Testing**: Test subgraphs independently before embedding
- **Avoid deep nesting**: Limit to 2-3 levels for maintainability

**See Also**:
- `examples/subgraph` - Complete multi-stage pipeline example
- [Core Concepts: Graphs](/core-concepts/#graphs-and-nodes) - Graph fundamentals
- API Reference: [`Subgraph`](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph#Builder.Subgraph)

---
