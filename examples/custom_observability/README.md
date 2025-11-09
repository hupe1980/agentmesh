# Custom Observability Example

This example demonstrates how to use custom observability providers (logging, tracing, metrics) in node `RunFunc` using explicit graph options.

## What This Example Shows

1. ✅ How to configure providers using explicit graph options
2. ✅ How to retrieve them in node `RunFunc` using `FromContext()`
3. ✅ How providers propagate through the entire execution chain
4. ✅ Using logger, tracer, and metrics in the same node

## Key Pattern: Explicit Provider Configuration

```go
// Step 1: Create custom providers
logger := logging.NewSlogAdapter(slog.New(...))
traceProvider := trace.NewOpenTelemetryProvider(...)
metricsProvider := metrics.NewPrometheusProvider(...)

// Step 2: Pass providers as graph options
results, err := compiled.Invoke(ctx, messages,
    graph.WithLogger(logger),
    graph.WithTracer(traceProvider),
    graph.WithMetrics(metricsProvider),
)
```

## Using Providers in Node RunFunc

```go
g.AddNode(&graph.Node{
    Name: "my_node",
    RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
        // Retrieve logger from context
        log := logging.FromContext(ctx)
        log.Info("Processing started", "node", "my_node")
        
        // Retrieve trace provider from context
        tp := trace.FromContext(ctx)
        tracer := tp.Tracer("my-service")
        ctx, span := tracer.Start(ctx, "operation")
        defer span.End(nil)
        
        // Retrieve metrics provider from context
        mp := metrics.FromContext(ctx)
        counter := mp.Counter("operations.count")
        counter.Add(ctx, 1, metrics.Attr{Key: "node", Value: "my_node"})
        
        // Your business logic here
        
        return &graph.NodeResult{...}, nil
    },
})
```

## Graph Flow

```
START → ingest_data → process_data → validate_data → generate_summary → END
```

### Node Responsibilities

1. **ingest_data** - Demonstrates logger usage
   - Logs ingestion start and completion
   - Records timestamp and record count

2. **process_data** - Demonstrates tracer usage
   - Creates custom spans for operations
   - Nests spans for sub-operations
   - Traces data transformation

3. **validate_data** - Demonstrates metrics usage
   - Records validation counters
   - Measures validation duration with histogram
   - Tracks valid/invalid record counts

4. **generate_summary** - Demonstrates all providers together
   - Uses logger, tracer, and metrics in combination
   - Shows how providers work seamlessly together

## Output

The example produces structured JSON logs:

```json
{"time":"2025-11-09T01:26:00.142355+01:00","level":"INFO","msg":"Starting data ingestion","node":"ingest_data"}
{"time":"2025-11-09T01:26:00.193393+01:00","level":"INFO","msg":"Data ingested successfully","record_count":3,"timestamp":"2025-11-09T01:26:00+01:00"}
{"time":"2025-11-09T01:26:00.193462+01:00","level":"INFO","msg":"Starting data processing","node":"process_data"}
...
```

## Running the Example

```bash
cd examples/custom_observability
go run main.go
```

## Production Usage

In production, replace noop providers with real implementations:

### OpenTelemetry Tracing

```go
import "github.com/hupe1980/agentmesh/pkg/trace/opentelemetry"

traceProvider := opentelemetry.NewProvider(
    opentelemetry.WithEndpoint("http://jaeger:4318"),
    opentelemetry.WithServiceName("my-service"),
)
ctx = trace.WithProvider(ctx, traceProvider)
```

### Prometheus Metrics

```go
import "github.com/hupe1980/agentmesh/pkg/metrics/opentelemetry"

metricsProvider := opentelemetry.NewMetricsProvider(
    opentelemetry.WithEndpoint("http://prometheus:9090"),
)
ctx = metrics.WithProvider(ctx, metricsProvider)
```

### Custom Logger

```go
import "log/slog"
import "github.com/hupe1980/agentmesh/pkg/logging"

handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
    AddSource: true,
})
logger := logging.NewSlogAdapter(slog.New(handler))
ctx = logging.WithLogger(ctx, logger)
```

## Key Takeaways

✅ **Context-based dependency injection** is the idiomatic Go way  
✅ **All providers** (logger, tracer, metrics) use the same pattern  
✅ **Automatic propagation** through all node executions  
✅ **No explicit passing** needed - context carries everything  
✅ **Type-safe retrieval** with `FromContext()` helpers  
✅ **Zero overhead** when providers not configured (noop defaults)

## Benefits of Explicit Options

✅ **Single configuration point** - No confusion between context-based and option-based approaches  
✅ **Automatic context propagation** - Graph attaches providers to context for all nodes  
✅ **Type-safe** - Explicit options prevent mixing incompatible providers  
✅ **Clear intent** - Code explicitly states what observability is configured  
✅ **Idiomatic Go** - Options pattern for config, context for propagation

## Related Examples

- `examples/observability` - Shows observability patterns
- `examples/streaming` - Shows real-time event streaming
- `examples/checkpointing` - Shows state persistence

## API Reference

- [`pkg/logging`](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/logging) - Logger interface and adapters
- [`pkg/trace`](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/trace) - Trace provider interface
- [`pkg/metrics`](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/metrics) - Metrics provider interface
