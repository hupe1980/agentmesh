---
layout: doc
title: Advanced Patterns
description: Advanced AgentMesh patterns including retry policies, circuit breakers, aggregators, and subgraphs.
permalink: /advanced/
hero:
  title: Advanced Patterns
  description: Leverage retry policies, circuit breakers, BSP aggregators, and subgraph composition.
  primary_cta:
    label: Explore examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
  secondary_cta:
    label: Graph API →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph"
    external: true
sidebar:
  - title: Retry policies
    url: "#retry-policies"
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
> For state management patterns (checkpointing, time travel, message retention, human-in-loop), see **[State Management](/state-management/)**.

---

## Retry Policies {#retry-policies}

Configure automatic retries with exponential backoff for transient failures:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "time"
)

// Configure retry on individual nodes
builder.Node("unreliable_api", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    result, err := callExternalAPI()
    if err != nil {
        return nil, err
    }
    return &graph.NodeResult{
        Updates: map[string]any{"api_result": result},
    }, nil
}, graph.WithRetryPolicy(&graph.RetryPolicy{
    MaxAttempts: 3,
    Backoff: func(attempt int) time.Duration {
        return time.Duration(math.Pow(2, float64(attempt))) * time.Second
    },
    Retryable: func(err error) bool {
        // Only retry specific error types
        return isTransientError(err)
    },
}))
```

**Key Features**:
- **Exponential backoff**: Default 2^n seconds delay between attempts
- **Custom backoff**: Provide your own `Backoff` function
- **Selective retry**: Use `Retryable` to decide which errors warrant retry
- **Max attempts**: Limit total attempts (including initial execution)

**Default Backoff**:
```go
// Built-in: 2^attempt seconds (1s, 2s, 4s, 8s, ...)
RetryPolicy: &graph.RetryPolicy{
    MaxAttempts: 5,
    Backoff: graph.DefaultBackoff,  // Uses 2^attempt
}
```

**See Also**: Check graph retry tests in `pkg/graph/retry_test.go` for comprehensive examples.

---

## Circuit Breaker {#circuit-breaker}

The circuit breaker pattern prevents cascading failures when calling external services. AgentMesh implements this as a **callback policy** in the `pkg/callbacks/policies` package, allowing you to wrap model or tool calls with automatic failure detection.

### How It Works

The circuit breaker implements three states:

- **CLOSED** - Normal operation, requests flow through
- **OPEN** - Too many failures detected, reject requests immediately with error
- **HALF_OPEN** - Testing recovery after timeout, allow limited requests

### Basic Usage

Circuit breakers are registered as callbacks on the callback manager:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/callbacks"
    "github.com/hupe1980/agentmesh/pkg/callbacks/policies"
    "github.com/hupe1980/agentmesh/pkg/agent"
)

// Create callback manager
manager := callbacks.NewManager()

// Configure circuit breaker
config := policies.DefaultCircuitBreakerConfig()
config.MaxFailures = 3              // Open after 3 failures
config.Timeout = 5 * time.Second    // Wait 5s before half-open
config.FailureWindow = 1 * time.Minute  // Track failures over 1 minute

// Register circuit breaker callbacks
before, after, onError := policies.CircuitBreaker(config)
manager.RegisterBeforeModel(before)
manager.RegisterAfterModel(after)
manager.RegisterOnModelError(onError)

// Use with agent (circuit breaker protects model calls)
compiled, err := agent.NewReActAgent(
    model,
    tools,
    agent.WithModelCallbacks(manager),
)
```

### Integration with Retry Policy

Combine circuit breakers with retry policies for robust error handling:

```go
manager := callbacks.NewManager()

// Add circuit breaker
cbConfig := policies.DefaultCircuitBreakerConfig()
cbConfig.MaxFailures = 3
cbConfig.Timeout = 5 * time.Second
cbBefore, cbAfter, cbError := policies.CircuitBreaker(cbConfig)
manager.RegisterBeforeModel(cbBefore)
manager.RegisterAfterModel(cbAfter)
manager.RegisterOnModelError(cbError)

// Add retry policy with circuit breaker awareness
retryConfig := policies.DefaultRetryConfig()
retryConfig.MaxAttempts = 3
retryConfig.Retryable = func(err error) bool {
    // Don't retry if circuit is open (will fail immediately anyway)
    if strings.Contains(err.Error(), "circuit breaker open") {
        return false
    }
    // Retry other transient errors
    return isTransientError(err)
}
manager.RegisterOnModelError(policies.ExponentialBackoffRetry(retryConfig))
```

### State Transitions

```
CLOSED ──[failure threshold reached]──> OPEN
  ↑                                       │
  │                                       │
  └─[success]─────────────── HALF_OPEN ←─[timeout elapsed]
           │
           └─[any failure]──> OPEN
```

### Thread Safety

The circuit breaker policy is safe for concurrent use across multiple goroutines using atomic operations.

**See Also**:
- `examples/circuit_breaker` - Complete working example with flaky service
- [Callbacks Guide](/callbacks/) - Callback system overview
- `pkg/callbacks/policies` - Policy implementations (circuit breaker, retry, etc.)

---

## Aggregators & BSP Coordination {#aggregators}

### What are Aggregators?

Aggregators are a core concept in the Bulk Synchronous Parallel (BSP) model that AgentMesh implements. They provide a mechanism for **global coordination** across all nodes in a graph by accumulating values during superstep execution.

**Key characteristics**:
- **Global visibility**: All nodes can read the aggregated value
- **Read-only in nodes**: Nodes contribute values but read the result from the previous superstep
- **BSP-aligned**: Updated after each superstep barrier
- **Type-safe**: Strongly typed aggregate values

### Built-in Aggregators

AgentMesh provides several built-in aggregators for common use cases:

#### SumAggregator

Accumulates numeric values across all node contributions:

```go
import "github.com/hupe1980/agentmesh/pkg/graph"

compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "total_processed": &graph.SumAggregator{},
    }),
)
```

**Returns**: `float64` - Sum of all contributed values

#### MinAggregator

Tracks the minimum value across all nodes:

```go
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "min_cost": &graph.MinAggregator{},
    }),
)
```

**Returns**: `float64` - Minimum value observed

#### MaxAggregator

Tracks the maximum value across all nodes:

```go
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "max_priority": &graph.MaxAggregator{},
    }),
)
```

**Returns**: `float64` - Maximum value observed

#### AvgAggregator

Computes the running average of numeric values using Welford's algorithm for numerical stability:

```go
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "avg_latency": &graph.AvgAggregator{},
    }),
)

// In node
s.Aggregate("avg_latency", responseTime)

// Read result
snap := s.AggregatesSnapshot()
avgState := snap["avg_latency"].(graph.avgState)
average := avgState.Mean
count := avgState.Count
```

**Returns**: `avgState{Mean: float64, Count: int64}` - Running mean and sample count

#### VarianceAggregator

Computes the variance of numeric values using Welford's algorithm:

```go
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "latency_variance": &graph.VarianceAggregator{},
    }),
)

// In node
s.Aggregate("latency_variance", responseTime)

// Read result
snap := s.AggregatesSnapshot()
varState := snap["latency_variance"].(graph.varianceState)
variance := varState.M2 / float64(varState.Count)
stdDev := math.Sqrt(variance)
```

**Returns**: `varianceState{Mean: float64, M2: float64, Count: int64}` - Mean, sum of squared differences (M2), and count

#### CountAggregator

Counts non-nil contributions:

```go
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "active_nodes": &graph.CountAggregator{},
    }),
)

// In node
s.Aggregate("active_nodes", true) // Any non-nil value increments
```

**Returns**: `int` - Total count

#### AllTrueAggregator / AnyTrueAggregator

Boolean aggregators for convergence detection and monitoring:

```go
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "all_converged": &graph.AllTrueAggregator{},
        "has_errors":    &graph.AnyTrueAggregator{},
    }),
)

// Check after superstep
if snap["all_converged"].(bool) {
    // All nodes converged, can terminate early
}
```

**Returns**: `bool` - Logical AND (AllTrue) or OR (AnyTrue)

### Using Aggregators in Nodes

Nodes contribute to aggregators and read results from previous supersteps:

### Using Aggregators in Nodes

Nodes contribute to aggregators and read results from previous supersteps:

```go
node := &graph.Node{
    Name: "processor",
    RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
        // Process some items
        itemsProcessed := 42
        latency := 150.0
        
        // Contribute to multiple aggregates
        if err := s.Aggregate("total_processed", itemsProcessed); err != nil {
            return nil, err
        }
        if err := s.Aggregate("avg_latency", latency); err != nil {
            return nil, err
        }
        
        // Read aggregates from previous superstep
        snap := s.AggregatesSnapshot()
        if snap != nil {
            if total, ok := snap["total_processed"].(float64); ok {
                log.Printf("Total items processed so far: %.0f", total)
            }
            if avgState, ok := snap["avg_latency"].(graph.avgState); ok {
                log.Printf("Average latency: %.2fms (n=%d)", avgState.Mean, avgState.Count)
            }
        }
        
        return &graph.NodeResult{}, nil
    },
}
```

**Use cases**:
- Count total messages processed
- Track cumulative errors  
- Calculate global statistics (mean, variance, min/max)
- Monitor convergence criteria

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
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "latency_median": &MedianAggregator{},
    }),
)
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
RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
                    Updates: map[string]any{"converged": true},
                }, nil
            }
        }
    }
    
    // Continue processing
    return &graph.NodeResult{}, nil
}
```

#### Distributed Counting

Track statistics across parallel branches:

```go
// Set up counters
compiled, err := builder.Compile(
    graph.WithAggregators(map[string]graph.Aggregator{
        "success_count": &graph.SumAggregator{},
        "failure_count": &graph.SumAggregator{},
        "total_latency": &graph.SumAggregator{},
    }),
)

// In each parallel node
RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    start := time.Now()
    
    result, err := doWork()
    latency := time.Since(start).Milliseconds()
    
    s.Aggregate("total_latency", latency)
    if err != nil {
        s.Aggregate("failure_count", 1)
        return nil, err
    }
    
    s.Aggregate("success_count", 1)
    return result, nil
}

// In final reporting node
RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    snap := s.AggregatesSnapshot()
    
    successCount := snap["success_count"].(float64)
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
RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    s.Aggregate("counter", 10)  // Contributed in superstep 0
    
    snap := s.AggregatesSnapshot()
    // snap["counter"] is NOT 10 yet - it's the value from superstep -1 (initial value)
    
    return &graph.NodeResult{}, nil
}

// Node B reads in superstep 1
RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    snap := s.AggregatesSnapshot()
    if snap != nil {
        counter := snap["counter"].(float64)
        // Now counter is 10 (from superstep 0)
    }
    
    s.Aggregate("counter", 5)  // Add 5 more for superstep 1
    return &graph.NodeResult{}, nil
}

// Node C reads in superstep 2
RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
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
subState := graph.NewGraphState(0)
subGraph := graph.NewGraph(subState)

subGraph.AddNode(&graph.Node{
    Name: "process",
    RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
        value := s.Get("value").(int)
        doubled := value * 2
        return &graph.NodeResult{
            Updates: map[string]any{"result": doubled},
        }, nil
    },
})

subGraph.AddEdge(graph.StartNode, "process")
subGraph.AddEdge("process", graph.EndNode)

// Compile the subgraph
compiledSub, err := subGraph.Compile()

// Use as a node in parent graph
parentState := graph.NewGraphState(0)
parent := graph.NewGraph(parentState)

parent.AddNode(&graph.Node{
    Name: "prepare",
    RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
        return &graph.NodeResult{
            Updates: map[string]any{"value": 21},
        }, nil
    },
})

// Embed subgraph as a node
parent.AddSubgraph("doubler", compiledSub)

parent.AddEdge(graph.StartNode, "prepare")
parent.AddEdge("prepare", "doubler")
parent.AddEdge("doubler", graph.EndNode)
```

### State Mapping

Map parent state to subgraph state and back using `AddSubgraphWithMapping`:

```go
parent.AddSubgraphWithMapping(
    "processor",
    compiledSub,
    // Map parent state -> subgraph state
    func(parentState graph.State) map[string]any {
        return map[string]any{
            "input": parentState.Get("data"),
        }
    },
    // Map subgraph state -> parent state updates
    func(subState graph.State) map[string]any {
        return map[string]any{
            "processed_data": subState.Get("output"),
        }
    },
)
```

### Use Cases

**Multi-stage pipelines**:
```go
// Validation -> Enrichment -> Analysis
validationSub, _ := createValidationGraph().Compile()
enrichmentSub, _ := createEnrichmentGraph().Compile()
analysisSub, _ := createAnalysisGraph().Compile()

pipeline := graph.NewGraph(state)
pipeline.AddSubgraph("validate", validationSub)
pipeline.AddSubgraph("enrich", enrichmentSub)
pipeline.AddSubgraph("analyze", analysisSub)

pipeline.AddEdge(graph.StartNode, "validate")
pipeline.AddEdge("validate", "enrich")
pipeline.AddEdge("enrich", "analyze")
pipeline.AddEdge("analyze", graph.EndNode)
```

**Reusable components**:
```go
// Create reusable authentication subgraph
authSub, _ := createAuthGraph().Compile()

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
