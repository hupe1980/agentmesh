# Observability Example

This example demonstrates production-grade observability with automatic metrics collection and distributed tracing using explicit provider configuration.

## What You'll Learn

1. ✅ How to configure observability using explicit graph options
2. ✅ How OpenTelemetry integrates with AgentMesh  
3. ✅ What metrics and traces are automatically collected
4. ✅ How distributed tracing works across node execution
5. ✅ How to access providers in node RunFuncs for custom instrumentation

## Quick Start

```bash
cd examples/observability
go run main.go
```

## Configuration

### Basic Setup (Noop Providers)

```go
// For development/testing - zero overhead
logger := logging.NoopLogger{}
metricsProvider := metrics.Noop()
traceProvider := trace.Noop()

result, _ := graph.Last(compiled.Run(ctx, messages, 
    graph.WithLogger(logger),
    graph.WithTracer(traceProvider),
    graph.WithMetrics(metricsProvider),
))
```

### Production Setup (OpenTelemetry)

```go
import (
    "log/slog"
    "github.com/hupe1980/agentmesh/pkg/logging"
    "github.com/hupe1980/agentmesh/pkg/metrics/opentelemetry"
    "github.com/hupe1980/agentmesh/pkg/trace/opentelemetry"
)

// Configure observability providers
logger := logging.NewSlogAdapter(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
traceProvider := opentelemetry.NewProvider(
    opentelemetry.WithEndpoint("http://jaeger:4318"),
    opentelemetry.WithServiceName("my-service"),
)
metricsProvider := opentelemetry.NewMetricsProvider(
    opentelemetry.WithEndpoint("http://prometheus:9090"),
)

result, _ := graph.Last(compiled.Run(ctx, messages,
    graph.WithLogger(logger),
    graph.WithTracer(traceProvider),
    graph.WithMetrics(metricsProvider),
))
```

## What Gets Instrumented Automatically

When you configure observability providers, the framework automatically:

### 1. Creates Trace Spans

- **Graph execution span**: Covers entire Run() call
- **Node execution spans**: One span per node execution
- **Checkpoint spans**: For checkpoint save/restore operations

### 2. Records Metrics

- **Node execution metrics**:
  - `agentgraph.node.executions` - Counter for node executions
  - `agentgraph.node.latency_ms` - Histogram of node execution time
  - `agentgraph.node.errors` - Counter for node errors

- **Graph execution metrics**:
  - `agentgraph.graph.executions` - Counter for graph executions
  - `agentgraph.superstep.latency_ms` - Histogram of superstep duration

All metrics include relevant labels like `node.name`, `superstep`, `error.type`.

### 3. Propagates Context

All providers are automatically attached to context for node RunFuncs:

```go
func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
    log := logging.FromContext(ctx)
    tp := trace.FromContext(ctx)
    mp := metrics.FromContext(ctx)
    
    // Use them for custom instrumentation
    log.Info("Processing", "node", "my_node")
    
    tracer := tp.Tracer("my-service")
    ctx, span := tracer.Start(ctx, "operation")
    defer span.End(nil)
    
    counter := mp.Counter("operations.count")
    counter.Add(ctx, 1)
}
```

## Benefits

- ✅ **Automatic instrumentation** - Zero code changes needed
- ✅ **Explicit configuration** - Clear intent, type-safe
- ✅ **Production ready** - OpenTelemetry compatible
- ✅ **Zero overhead** - Noop providers when not configured
- ✅ **Custom instrumentation** - Access providers in nodes

## Related Examples

- [`custom_observability`](../custom_observability) - Custom provider usage
- [`streaming`](../streaming) - Real-time event streaming
- [`checkpointing`](../checkpointing) - State persistence with tracing
