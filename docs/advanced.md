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
agent.NewReActAgent(model,
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

**For Node-Level Retries**: Use `graph.WithRetryPolicy()` when adding nodes to retry specific operations.

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
agent.NewReActAgent(model,
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
- **Type-safe**: Registered via `state.RegisterAggregateKey[T]()`
- **Channel-based**: Integrated with state management system

### Built-in Aggregators

AgentMesh provides several built-in aggregators in the `pkg/state/aggregators` package:

#### SumAggregator

Accumulates numeric values across all node contributions:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/state"
    "github.com/hupe1980/agentmesh/pkg/state/aggregators"
)

// Create state manager builder
builder := state.NewManagerBuilder()

// Register aggregate key
totalProcessedKey := state.NewKey[any]("total_processed", 0)
state.RegisterAggregateKey(builder, totalProcessedKey, &aggregators.SumAggregator{})

mgr := builder.Build()

// In nodes - contribute via Command pattern
func processorNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Read current total
    total, _ := state.GetFromView(view, totalProcessedKey)
    fmt.Printf("Total processed: %v\n", total)
    
    // Contribute new count using Command pattern
    return command.New().
        Set(totalProcessedKey, 42.0).
        To(graph.EndNode)
}

// After execution, read final sum
total, _ := state.Get(ctx, mgr, totalProcessedKey)
```

**Returns**: `float64` - Sum of all contributed values

#### MinAggregator / MaxAggregator

Tracks the minimum or maximum value across all nodes:

```go
minCostKey := state.NewKey[any]("min_cost", float64(1e308))
maxPriorityKey := state.NewKey[any]("max_priority", float64(-1e308))

state.RegisterAggregateKey(mgr, minCostKey, &aggregators.MinAggregator{})
state.RegisterAggregateKey(mgr, maxPriorityKey, &aggregators.MaxAggregator{})

// Contribute via Command pattern
return command.New().
    Set(minCostKey, estimatedCost).
    Set(maxPriorityKey, taskPriority).
    To(graph.EndNode)
```

**Returns**: `float64` - Minimum or maximum value observed

#### AvgAggregator

Computes the running average of numeric values using Welford's algorithm for numerical stability:

```go
avgLatencyKey := state.NewKey[any]("avg_latency", nil)
state.RegisterAggregateKey(mgr, avgLatencyKey, &aggregators.AvgAggregator{})

// In node
return command.New().
    Set(avgLatencyKey, responseTime).
    To(graph.EndNode)

// Read result (returns AvgState)
avgStateAny, _ := state.Get(ctx, mgr, avgLatencyKey)
avgState := avgStateAny.(aggregators.AvgState)
average := avgState.Mean
count := avgState.Count
```

**Returns**: `aggregators.AvgState{Mean: float64, Count: int64}` - Running mean and sample count

#### VarianceAggregator

Computes the variance of numeric values using Welford's algorithm:

```go
varianceKey := state.NewKey[any]("latency_variance", nil)
state.RegisterAggregateKey(mgr, varianceKey, &aggregators.VarianceAggregator{})

// In node
return command.New().
    Set(varianceKey, responseTime).
    To(graph.EndNode)

// Read result
varStateAny, _ := state.Get(ctx, mgr, varianceKey)
varState := varStateAny.(aggregators.VarianceState)
variance := varState.M2 / float64(varState.Count)
stdDev := math.Sqrt(variance)
```

**Returns**: `aggregators.VarianceState{Mean: float64, M2: float64, Count: int64}` - Mean, sum of squared differences (M2), and count

#### CountAggregator

Counts non-nil contributions:

```go
activeNodesKey := state.NewKey[any]("active_nodes", 0)
state.RegisterAggregateKey(mgr, activeNodesKey, &aggregators.CountAggregator{})

// In node - any non-nil value increments
return command.New().
    Set(activeNodesKey, 1).
    To(graph.EndNode)
```

**Returns**: `int` - Total count

#### AllTrueAggregator / AnyTrueAggregator

Boolean aggregators for convergence detection and monitoring:

```go
allConvergedKey := state.NewKey[any]("all_converged", true)
hasErrorsKey := state.NewKey[any]("has_errors", false)

state.RegisterAggregateKey(mgr, allConvergedKey, &aggregators.AllTrueAggregator{})
state.RegisterAggregateKey(mgr, hasErrorsKey, &aggregators.AnyTrueAggregator{})

// In node
return command.New().
    Set(allConvergedKey, isConverged).
    Set(hasErrorsKey, hasError).
    To(graph.EndNode)

// Check convergence
if allConverged, _ := state.Get(ctx, mgr, allConvergedKey); allConverged.(bool) {
    // All nodes converged, can terminate early
}
```

**Returns**: `bool` - Logical AND (AllTrue) or OR (AnyTrue)

### Using Aggregators in Nodes

Nodes contribute to aggregators via normal `state.Updates` and read accumulated values from `state.ReadView`:

```go
func processorNode(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    // Read current aggregated values
    totalProcessed, _ := state.GetFromView(view, totalProcessedKey)
    avgLatency, _ := state.GetFromView(view, avgLatencyKey)
    
    fmt.Printf("Progress: %v items, avg latency: %v\n", totalProcessed, avgLatency)
    
    // Process some items
    itemsProcessed := 42
    latency := 150.0
    
    // Contribute to aggregators via Command pattern
    return command.New().
        Set(totalProcessedKey, float64(itemsProcessed)).
        Set(avgLatencyKey, latency).
        To(graph.EndNode)
}
```

**Use cases**:
- Count total messages processed
- Track cumulative errors  
- Calculate global statistics (mean, variance, min/max)
- Monitor convergence criteria
- Distributed coordination and decision-making

### Custom Aggregators

Implement the `state.Aggregator` interface for custom reduction logic:

```go
// From pkg/state/internal/channel/channel.go
type Aggregator interface {
    Zero() any
    Aggregate(current, value any) any
}
```

#### Example: Median Aggregator

Track values to compute median:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/state"
)

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
builder := state.NewManagerBuilder()
medianKey := state.NewKey[any]("latency_median", nil)
state.RegisterAggregateKey(builder, medianKey, &MedianAggregator{})

mgr := builder.Build()

// In node
return command.New().
    Set(medianKey, latency).
    To(graph.EndNode)

// After execution, compute median from collected values
medianStateAny, _ := state.Get(ctx, mgr, medianKey)
ms := medianStateAny.(medianState)
sort.Float64s(ms.Values)
median := ms.Values[len(ms.Values)/2]
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
histogramKey := state.NewKey[any]("response_time_histogram", nil)
state.RegisterAggregateKey(mgr, histogramKey, &HistogramAggregator{
    Bins: []float64{100, 200, 500, 1000}, // <100ms, 100-200ms, etc.
})
```

### Advanced Patterns

#### Convergence Detection

Use aggregators to detect when a graph has converged:

```go
type ErrorAggregator struct{}

func (a *ErrorAggregator) Aggregate(ctx context.Context, accumulated any, contributions []any) (any, error) {
    var totalError float64
    for _, contrib := range contributions {
        if err, ok := contrib.(float64); ok {
            totalError += err
        }
    }
    return totalError, nil
}

// In node: check for convergence
RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    // Calculate local error
    localError := computeLocalError()
    s.Aggregate("global_error", localError)
    
    // Check previous superstep's global error
    snap := s.AggregatesSnapshot()
    if snap != nil {
        if globalError, ok := snap["global_error"].(float64); ok {
            if globalError < 0.001 {
                // Converged! Route to END
                return &graph.NodeResult{
                    return map[string]any{"converged": true}, nil
            }
        }
    }
    
    // Continue processing
    return nil, nil
}
```

#### Distributed Counting

Track statistics across parallel branches:

```go
// Set up counters
executor := graph.NewPregelExecutor(
    graph.WithPregelAggregators(map[string]pregel.Aggregator{
        "success_count": pregel.SumAggregator{},
        "failure_count": pregel.SumAggregator{},
        "total_latency": pregel.SumAggregator{},
    }),
)
g.WithExecutor(executor)
compiled, _ := exec.CompileGraph(g)

// In each parallel node (Pregel-style aggregation)
func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    start := time.Now()
    
    result, err := doWork()
    latency := time.Since(start).Milliseconds()
    
    // Note: Aggregation API depends on Pregel runtime context
    // This is a conceptual example
    if err != nil {
        return nil, err
    }
    return result, nil
}

// In final reporting node (Pregel-style)
func(ctx context.Context, view *state.ReadView) (state.Updates, error) {
    // Note: Aggregates are specific to Pregel runtime
    // This is a conceptual example
    
    successCount := 0.0 // Would come from aggregates snapshot
    failureCount := snap["failure_count"].(float64)
    totalLatency := snap["total_latency"].(float64)
    avgLatency := totalLatency / (successCount + failureCount)
    
    log.Printf("Success: %.0f, Failures: %.0f, Avg Latency: %.2fms",
        successCount, failureCount, avgLatency)
    
    return &graph.NodeResult{
        Updates: map[string]any{
            "success_rate": successCount / (successCount + failureCount),
            "avg_latency_ms": avgLatency,
        },
    }, nil
}
```

### BSP Semantics & Aggregators

Understanding the BSP (Bulk Synchronous Parallel) model is key to using aggregators effectively:

#### Superstep Execution Model

```
Superstep N:
1. All nodes execute in parallel
2. Nodes contribute to aggregators via Aggregate()
3. Barrier: wait for all nodes to complete
4. Aggregate values computed by combining contributions
5. New aggregate values become visible

Superstep N+1:
1. Nodes read aggregates from superstep N via AggregatesSnapshot()
2. Nodes contribute to aggregators for superstep N+1
3. ... repeat
```

#### Important Rules

1. **Contributions are isolated**: Values aggregated in superstep N are NOT visible until superstep N+1
2. **Thread-safe by design**: Aggregation happens after the barrier, no need for locks
3. **Multiple contributions**: Same node can call `Aggregate()` multiple times in one superstep
4. **Reset between retries**: If a node fails and retries, its aggregate contributions are cleared

#### Example: Multi-Superstep Coordination

```go
// Node A contributes in superstep 0
RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    s.Aggregate("counter", 10)  // Contributed in superstep 0
    
    snap := s.AggregatesSnapshot()
    // snap["counter"] is NOT 10 yet - it's the value from superstep -1 (initial value)
    
    return &graph.NodeResult{}, nil
}

// Node B reads in superstep 1
RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    snap := s.AggregatesSnapshot()
    if snap != nil {
        counter := snap["counter"].(float64)
        // Now counter is 10 (from superstep 0)
    }
    
    s.Aggregate("counter", 5)  // Add 5 more for superstep 1
    return &graph.NodeResult{}, nil
}

// Node C reads in superstep 2
RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    snap := s.AggregatesSnapshot()
    counter := snap["counter"].(float64)
    // Now counter is 15 (10 + 5 from supersteps 0 and 1)
    
    return &graph.NodeResult{}, nil
}
```

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

## Subgraphs {#subgraphs}

Subgraphs enable **hierarchical composition** by embedding compiled graphs as nodes within parent graphs. This pattern helps organize complex workflows into modular, reusable components.

### Basic Usage

```go
// Create a subgraph
subState := graph.NewStateManager(0)
subGraph := graph.NewGraph(subState)

subGraph.AddNodeFunc("process", func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
    value := s.Get("value").(int)
    doubled := value * 2
    return &graph.NodeResult{
        Updates: map[string]any{"result": doubled},
    }, nil
})

subGraph.AddNode(&graph.BaseNode{
    NodeName:        "process",
    DeclaredTargets: []string{graph.EndNode},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        value := state.GetFromView(view, ValueKey)
        doubled := value * 2
        return command.New().
            Set(resultKey, doubled).
            To(graph.EndNode)
    },
})
subGraph.SetEntryPoint("process")

// Compile the subgraph
compiledSub, err := exec.CompileGraph(subGraph)

// Use as a node in parent graph
parentState := graph.NewStateManager(0)
parent := graph.NewGraph(parentState)

parent.AddNodeFunc("prepare", func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
    return command.New().
        Set(valueKey, 21).
        To("doubler")
})

// Embed subgraph as a node
parent.AddSubgraph("doubler", compiledSub)

// Parent nodes use tuple return
parent.AddNode(&graph.BaseNode{
    NodeName:        "prepare",
    DeclaredTargets: []string{"doubler"},
    Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
        return command.New().
            Set(valueKey, 21).
            To("doubler")
    },
})
parent.SetEntryPoint("prepare")
```

### State Mapping

Map parent state to subgraph state and back using `AsNodeWithStateMapping`:

```go
parent.AddNode(compiledSub.AsNodeWithStateMapping(
    "processor",
    // Map parent state -> subgraph input (values and messages)
    func(parentState state.Reader) (map[string]any, []graph.MessageEvent) {
        return map[string]any{
            "input": parentState.Get("data"),
        }, nil
    },
    // Map subgraph output -> parent state updates (values and messages)
    func(subState state.Reader) (map[string]any, []graph.MessageEvent) {
        return map[string]any{
            "processed_data": subState.Get("output"),
        }, nil
    },
))
```

### Use Cases

**Multi-stage pipelines**:
```go
// Validation -> Enrichment -> Analysis
validationSub, _ := exec.CompileGraph(createValidationGraph())
enrichmentSub, _ := exec.CompileGraph(createEnrichmentGraph())
analysisSub, _ := exec.CompileGraph(createAnalysisGraph())

pipeline := graph.NewGraph(state)
pipeline.AddSubgraph("validate", validationSub)
pipeline.AddSubgraph("enrich", enrichmentSub)
pipeline.AddSubgraph("analyze", analysisSub)

// Pipeline stages with tuple return routing
pipeline.AddNode(&graph.BaseNode{
    NodeName:        "validate",
    DeclaredTargets: []string{"enrich"},
    Fn:              validateFunc,
})
pipeline.AddNode(&graph.BaseNode{
    NodeName:        "enrich",
    DeclaredTargets: []string{"analyze"},
    Fn:              enrichFunc,
})
pipeline.AddNode(&graph.BaseNode{
    NodeName:        "analyze",
    DeclaredTargets: []string{graph.EndNode},
    Fn:              analyzeFunc,
})
pipeline.SetEntryPoint("validate")
```

**Reusable components**:
```go
// Create reusable authentication subgraph
authSub, _ := exec.CompileGraph(createAuthGraph())

// Use in multiple parent graphs
apiGraph.AddSubgraph("auth", authSub)
adminGraph.AddSubgraph("auth", authSub)
publicGraph.AddSubgraph("auth", authSub)
```

### Best Practices

- **Modular design**: Keep subgraphs focused on single responsibilities
- **State isolation**: Use state mapping to explicitly define data flow
- **Testing**: Test subgraphs independently before embedding
- **Avoid deep nesting**: Limit to 2-3 levels for maintainability

**See Also**:
- `examples/subgraph` - Complete multi-stage pipeline example
- [Core Concepts: Graphs](/core-concepts/#graphs-and-nodes) - Graph fundamentals
- API Reference: [`AddSubgraph`](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph#Builder.AddSubgraph)

---
