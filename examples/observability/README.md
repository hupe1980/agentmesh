# Example: Observability

## Overview
Demonstrates production-grade observability with OpenTelemetry metrics and distributed tracing. Shows how to instrument AgentMesh for monitoring in production environments.

## Key Concepts
- **Metrics Collection**: Track performance and health
- **Distributed Tracing**: Debug execution flows
- **Instrumentation**: Wrapper for observability providers
- **Production Monitoring**: Real-time insights

## Running
```bash
cd examples/observability
go run main.go
```

## Expected Output
```
=== Observability Example ===

Instrumenting graph with metrics and tracing...
✓ Metrics provider: Noop (replace with OpenTelemetry in production)
✓ Trace provider: Noop (replace with OpenTelemetry in production)

Executing instrumented workflow...

[Metrics Recorded]
  - graph.superstep.duration: 1.234s
  - node.execution.duration{node=step1}: 0.512s
  - node.execution.duration{node=step2}: 0.722s
  - graph.message.count: 5

[Traces Created]
  - Span: graph.execution (1.234s)
    - Span: node.step1 (0.512s)
    - Span: node.step2 (0.722s)

Workflow complete!

Note: In production, replace Noop() providers with:
  - metrics.opentelemetry.New(meterProvider)
  - trace.opentelemetry.New(tracerProvider)
```

## Code Walkthrough

### 1. Create Instrumentation (Development)
```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/metrics"
    "github.com/hupe1980/agentmesh/pkg/trace"
)

instr := graph.NewInstrumentation(
    metrics.Noop(),  // No-op for development
    trace.Noop(),    // No-op for development
)
```

### 2. Create Instrumentation (Production)
```go
import (
    metricsOtel "github.com/hupe1980/agentmesh/pkg/metrics/opentelemetry"
    traceOtel "github.com/hupe1980/agentmesh/pkg/trace/opentelemetry"
)

// Setup OpenTelemetry (see OpenTelemetry docs)
meterProvider := /* ... */
tracerProvider := /* ... */

instr := graph.NewInstrumentation(
    metricsOtel.New(meterProvider),
    traceOtel.New(tracerProvider),
)
```

### 3. Compile with Instrumentation
```go
compiled, _ := builder.Compile(
    graph.WithInstrumentation(instr),
)
```

### 4. Execute and Collect Metrics
```go
result, _ := compiled.Invoke(ctx, messages)

// Metrics automatically recorded:
//  - Superstep duration
//  - Node execution time
//  - Message count
//  - Error rates
```

## Metrics Collected

### Graph Metrics
- `graph.superstep.duration`: Time per superstep
- `graph.execution.total`: Total graph execution time
- `graph.message.count`: Messages processed
- `graph.error.count`: Errors encountered

### Node Metrics
- `node.execution.duration{node=name}`: Per-node execution time
- `node.retry.count{node=name}`: Retry attempts
- `node.error.count{node=name}`: Node-specific errors

## Trace Spans

### Graph Execution Span
```
Span: graph.execution
  Attributes:
    - run_id: "workflow-123"
    - supersteps: 5
    - message_count: 10
```

### Node Execution Spans
```
Span: node.execution
  Parent: graph.execution
  Attributes:
    - node.name: "process"
    - node.superstep: 2
    - node.duration_ms: 512
```

## Production Setup

### Complete OpenTelemetry Integration
```go
package main

import (
    "context"
    
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/prometheus"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/metric"
    "go.opentelemetry.io/otel/sdk/trace"
    
    metricsOtel "github.com/hupe1980/agentmesh/pkg/metrics/opentelemetry"
    traceOtel "github.com/hupe1980/agentmesh/pkg/trace/opentelemetry"
)

func main() {
    ctx := context.Background()
    
    // Setup metrics exporter (Prometheus)
    promExporter, _ := prometheus.New()
    meterProvider := metric.NewMeterProvider(
        metric.WithReader(promExporter),
    )
    otel.SetMeterProvider(meterProvider)
    
    // Setup trace exporter (Jaeger)
    jaegerExporter, _ := jaeger.New(jaeger.WithCollectorEndpoint(
        jaeger.WithEndpoint("http://localhost:14268/api/traces"),
    ))
    tracerProvider := trace.NewTracerProvider(
        trace.WithBatcher(jaegerExporter),
    )
    otel.SetTracerProvider(tracerProvider)
    
    // Create instrumentation
    instr := graph.NewInstrumentation(
        metricsOtel.New(meterProvider),
        traceOtel.New(tracerProvider),
    )
    
    // Use in graph compilation
    compiled, _ := builder.Compile(
        graph.WithInstrumentation(instr),
    )
}
```

### Expose Metrics Endpoint
```go
http.Handle("/metrics", promhttp.Handler())
log.Fatal(http.ListenAndServe(":2112", nil))
```

## What This Example Teaches
- ✅ OpenTelemetry integration
- ✅ Metrics collection
- ✅ Distributed tracing
- ✅ Production instrumentation
- ✅ Observability best practices

## Monitoring Dashboards

### Grafana Metrics
- Graph execution duration (P50, P95, P99)
- Node execution breakdown
- Error rates and retry counts
- Message throughput

### Jaeger Tracing
- Execution flow visualization
- Performance bottleneck identification
- Error propagation tracking
- Distributed system debugging

## Next Steps
- Set up Prometheus + Grafana
- Configure Jaeger or Zipkin
- Create alerting rules
- Build custom dashboards

## See Also
- [pkg/metrics/opentelemetry](../../pkg/metrics/opentelemetry) - Metrics integration
- [pkg/trace/opentelemetry](../../pkg/trace/opentelemetry) - Tracing integration
- [OpenTelemetry Go Docs](https://opentelemetry.io/docs/instrumentation/go/)
