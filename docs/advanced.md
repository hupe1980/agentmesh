---
layout: doc
title: Advanced Features
description: Explore checkpointing, time travel, human-in-the-loop, and other advanced graph capabilities.
permalink: /advanced/
hero:
  title: Advanced graph features
  description: Leverage checkpointing, time travel debugging, human-in-the-loop workflows, and more.
  primary_cta:
    label: Explore examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples"
    external: true
  secondary_cta:
    label: Graph API →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/graph"
    external: true
sidebar:
  - title: Human-in-the-loop
    url: "#human-in-the-loop"
  - title: Message retention
    url: "#message-retention"
  - title: Retry policies
    url: "#retry-policies"
  - title: Circuit Breaker
    url: "#circuit-breaker"
  - title: Subgraphs
    url: "#subgraphs"
---

## Checkpointing & Time Travel

AgentMesh provides comprehensive state persistence and debugging capabilities. For complete documentation including:

- Checkpoint lifecycle and automatic state saving
- Storage backends (Memory, SQL, DynamoDB) with trade-off analysis
- Time-travel debugging patterns
- Production considerations and cleanup strategies
- Recovery and resume workflows

See the **[Checkpointing Guide](/checkpointing/)** for detailed coverage.

**Quick Example**:

```go
import "github.com/hupe1980/agentmesh/pkg/checkpoint"

// Enable checkpointing
store := checkpoint.NewMemory()
compiled, _ := builder.Compile(
    graph.WithCheckpointStore(store),
    graph.WithCheckpointInterval(1),
)

// Execute with automatic checkpointing
results, _ := compiled.Invoke(ctx, messages,
    graph.WithThreadID("workflow-123"),
)

// Resume from checkpoint after failure
results, _ = compiled.InvokeFromCheckpoint(ctx, "workflow-123")
```

**Examples**: 
- [Checkpointing example](https://github.com/hupe1980/agentmesh/tree/main/examples/checkpointing)
- [Time-travel debugging example](https://github.com/hupe1980/agentmesh/tree/main/examples/time_travel)

---

## Human-in-the-loop {#human-in-the-loop}

Pause execution for human approval or input:

```go
builder.Node("request_approval", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Request human input
    return &graph.NodeResult{
        Updates: map[string]any{
            "status": "awaiting_approval",
        },
        Interrupt: true,  // Pause execution
    }, nil
})

// Execution pauses here
results, _ := compiled.Invoke(ctx, messages)

// After human provides input, resume
results, _ = compiled.Resume(ctx, threadID, map[string]any{
    "approved": true,
})
```

See `examples/human_pause` for a complete workflow.

---

## Message retention {#message-retention}

Limit conversation history to prevent context overflow:

```go
// Keep only the last 10 messages
state := graph.NewGraphState(10)

builder := graph.NewBuilder()
builder.SetState(state)
```

Older messages are automatically pruned as new ones are added.

See `examples/message_retention` for details.

---

## Retry policies {#retry-policies}

Configure automatic retries for transient failures:

```go
import "time"

builder.Node("unreliable_api", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // ... call external API ...
}, graph.WithRetryPolicy(&graph.RetryPolicy{
    MaxAttempts:    3,
    InitialBackoff: 100 * time.Millisecond,
    MaxBackoff:     1 * time.Second,
    Multiplier:     2.0,
}))
```

The node will be retried with exponential backoff until it succeeds or reaches max attempts.

See `examples/retry` for various retry scenarios.

---

## Circuit Breaker {#circuit-breaker}

AgentMesh includes a built-in circuit breaker to prevent cascading failures when calling external services. The circuit breaker implements three states:

- **Closed**: Requests flow normally, failures are tracked
- **Open**: Requests fail fast without calling the protected function
- **HalfOpen**: Limited test requests check if the service has recovered

### Basic Usage

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "time"
)

// Create a circuit breaker
cb := graph.NewCircuitBreaker(
    5,              // failureThreshold: open after 5 failures
    2,              // successThreshold: close after 2 successes in half-open
    10*time.Second, // timeout: wait 10s before transitioning to half-open
)

// Protect service calls
err := cb.Call(ctx, func(ctx context.Context) error {
    return externalService.DoWork(ctx)
})
```

### Integration with Retry Policy

Combine circuit breakers with retry policies for robust error handling:

```go
cb := graph.NewCircuitBreaker(3, 2, 5*time.Second)

builder.Node("protected-service", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    err := cb.Call(ctx, func(ctx context.Context) error {
        return externalAPI.Call(ctx)
    })
    
    if err != nil {
        return nil, err
    }
    
    return &graph.NodeResult{
        Updates: map[string]any{"status": "success"},
    }, nil
}, graph.WithRetryPolicy(&graph.RetryPolicy{
    MaxAttempts: 10,
    Backoff: func(attempt int) time.Duration {
        // Wait longer when circuit is open
        if cb.State() == graph.StateOpen {
            return 6 * time.Second
        }
        return 500 * time.Millisecond
    },
    Retryable: func(err error) bool {
        // Don't retry circuit breaker open errors immediately
        return !errors.Is(err, graph.ErrCircuitBreakerOpen)
    },
}))
```

### State Transitions

```
CLOSED ──[failure threshold reached]──> OPEN
  ↑                                       │
  │                                       │
  └─[success threshold reached]─ HALF_OPEN ←[timeout elapsed]
           │
           └─[any failure]──> OPEN
```

### Manual Control

```go
// Check current state
state := cb.State() // Returns CircuitBreakerState (CLOSED, OPEN, HALF_OPEN)

// Manually reset to closed state
cb.Reset()
```

### Thread Safety

CircuitBreaker is safe for concurrent use across multiple goroutines using atomic operations.

**See**: `examples/circuit_breaker` for a complete working example.

---

## Aggregators & BSP Coordination

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
