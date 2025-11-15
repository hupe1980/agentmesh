---
layout: doc
title: Observability
description: Instrument graphs with OpenTelemetry metrics and distributed tracing.
permalink: /observability/
hero:
  title: Monitor graph execution
  description: Track agent workflows with built-in OpenTelemetry metrics and distributed tracing support.
  primary_cta:
    label: Enable observability
    href: "#quick-start"
  secondary_cta:
    label: View example →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/observability"
    external: true
sidebar:
  - title: Quick Start
    url: "#quick-start"
  - title: Configuration
    url: "#configuration"
  - title: What Gets Instrumented
    url: "#what-gets-instrumented"
  - title: Custom Instrumentation
    url: "#custom-instrumentation"
  - title: Metrics Reference
    url: "#metrics-reference"
---

## Quick Start {#quick-start}

Enable observability with explicit options:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/logging"
    "github.com/hupe1980/agentmesh/pkg/metrics"
    "github.com/hupe1980/agentmesh/pkg/trace"
)

// Configure providers (noop for development)
logger := logging.NoopLogger{}
metricsProvider := metrics.Noop()
traceProvider := trace.Noop()

// Execute with automatic instrumentation
messages, err := graph.CollectMessages(compiled.Run(ctx, messages,
    graph.WithLogger(logger),
    graph.WithTracer(traceProvider),
    graph.WithMetrics(metricsProvider),
))
if err != nil {
    log.Fatal(err)
}
```

## Configuration {#configuration}

### Development (Noop Providers)

For testing with zero overhead:

```go
messages, err := graph.CollectMessages(compiled.Run(ctx, messages,
    graph.WithLogger(logging.NoopLogger{}),
    graph.WithTracer(trace.Noop()),
    graph.WithMetrics(metrics.Noop()),
))
if err != nil {
    log.Fatal(err)
}
```

### Production (OpenTelemetry)

For production with OpenTelemetry:

```go
import (
    "log/slog"
    "os"
    "github.com/hupe1980/agentmesh/pkg/logging"
    "github.com/hupe1980/agentmesh/pkg/metrics/opentelemetry"
    "github.com/hupe1980/agentmesh/pkg/trace/opentelemetry"
)

// Configure structured logging (built-in slog adapter)
logger := logging.NewSlogLogger(
    logging.LogLevelInfo,      // Debug, Info, Warn, Error
    logging.LogFormatJSON,     // JSON or Text format
)

// Configure OpenTelemetry tracing
traceProvider := opentelemetry.NewProvider(
    opentelemetry.WithEndpoint("http://jaeger:4318"),
    opentelemetry.WithServiceName("my-agent-service"),
)

// Configure OpenTelemetry metrics
metricsProvider := opentelemetry.NewMetricsProvider(
    opentelemetry.WithEndpoint("http://prometheus:9090"),
)

// Execute with full observability
messages, err := graph.CollectMessages(compiled.Run(ctx, messages,
    graph.WithLogger(logger),
    graph.WithTracer(traceProvider),
    graph.WithMetrics(metricsProvider),
))
if err != nil {
    log.Fatal(err)
}
```

## What Gets Instrumented {#what-gets-instrumented}

When you configure providers, AgentMesh **automatically**:

### 1. Emits Structured Logs

Throughout execution, the runtime emits structured logs using `logging.FromContext()`:

**Graph Runtime:**
- Graph execution start/completion (Info level)
- Checkpoint save/restore operations (Info/Debug)
- Checkpoint failures (Error level)
- Graph execution failures (Error level)

**Node Execution:**
- Node start/completion (Debug level)
- Node failures (Error level)
- Human pause events (Info level)

**Pregel Runtime:**
- Superstep start/completion (Debug level)
- Frontier consumption (Debug level)
- Runtime failures (Error level)

All logs include structured attributes like `run_id`, `superstep`, `node`, `duration_ms`, etc.

### 2. Creates Trace Spans

- **Graph execution** - Overall `Run()` duration
- **Node execution** - Every node that runs, including timing
- **Checkpoint operations** - Save and restore operations

Example trace hierarchy:
```
graph.execute (1.5s)
├── node.execute[step1] (500ms)
├── node.execute[step2] (700ms)
└── checkpoint.save (50ms)
```

### 3. Records Metrics

All metrics include relevant labels (`node.name`, `superstep`, etc.):

**Node Metrics:**
- `agentgraph.node.executions` (counter) - Number of node executions
- `agentgraph.node.latency_ms` (histogram) - Node execution duration
- `agentgraph.node.errors` (counter) - Node execution errors

**Graph Metrics:**
- `agentgraph.graph.executions` (counter) - Number of graph executions
- `agentgraph.superstep.latency_ms` (histogram) - Superstep duration

### 4. Propagates Context

All providers are automatically attached to context and available in node RunFuncs:

```go
func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    log := logging.FromContext(ctx)
    tp := trace.FromContext(ctx)
    mp := metrics.FromContext(ctx)
    
    // Use for custom instrumentation
    log.Info("Processing data", "count", len(data))
    // ... more below
}
```

## Custom Instrumentation {#custom-instrumentation}

Access providers in your node RunFuncs for custom instrumentation:

### Logging

```go
func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    log := logging.FromContext(ctx)
    
    log.Info("Starting processing", "node", "data_processor")
    log.Debug("Details", "records", len(data))
    log.Warn("Slow operation detected", "duration_ms", elapsed)
    
    return result, nil
}
```

### Custom Spans

```go
func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    tp := trace.FromContext(ctx)
    tracer := tp.Tracer("my-service")
    
    // Create custom span for sub-operation
    ctx, span := tracer.Start(ctx, "database-query",
        trace.Attr{Key: "query", Value: sql},
    )
    defer span.End(nil)
    
    // Execute operation
    results := queryDatabase(ctx, sql)
    
    return &graph.NodeResult{...}, nil
}
```

### Custom Metrics

```go
func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    mp := metrics.FromContext(ctx)
    
    // Record counter
    processedCounter := mp.Counter("records.processed")
    processedCounter.Add(ctx, int64(len(data)),
        metrics.Attr{Key: "type", Value: "user_data"},
    )
    
    // Record histogram
    duration := mp.Histogram("operation.duration_ms")
    duration.Record(ctx, float64(elapsed.Milliseconds()))
    
    return result, nil
}
```

## Metrics Reference {#metrics-reference}

### Automatically Collected

| Metric | Type | Description | Labels |
|--------|------|-------------|--------|
| `agentgraph.node.executions` | Counter | Node execution count | `node.name` |
| `agentgraph.node.latency_ms` | Histogram | Node execution time | `node.name` |
| `agentgraph.node.errors` | Counter | Node execution errors | `node.name`, `error.type` |
| `agentgraph.graph.executions` | Counter | Graph execution count | `graph.name`, `success` |
| `agentgraph.superstep.latency_ms` | Histogram | Superstep duration | `superstep`, `node_count` |

### Zero Overhead

If you don't configure providers, AgentMesh uses **noop** implementations with **zero performance overhead**:

```go
// No providers = zero overhead
messages, err := graph.CollectMessages(compiled.Run(ctx, messages))
if err != nil {
    log.Fatal(err)
}
```

## Benefits

✅ **Automatic instrumentation** - No code changes needed  
✅ **Explicit configuration** - Clear, type-safe options  
✅ **Production ready** - OpenTelemetry compatible  
✅ **Zero overhead** - Noop providers when not configured  
✅ **Custom instrumentation** - Full provider access in nodes  
✅ **Context propagation** - Providers automatically available everywhere

## Examples

- [**observability**](https://github.com/hupe1980/agentmesh/tree/main/examples/observability) - Automatic instrumentation setup
- [**custom_observability**](https://github.com/hupe1980/agentmesh/tree/main/examples/custom_observability) - Custom instrumentation in nodes
