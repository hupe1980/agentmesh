---
layout: doc
title: Observability
description: Instrument graphs with OpenTelemetry metrics and distributed tracing.
permalink: /observability/
hero:
  title: Monitor graph execution
  description: Track agent workflows with built-in OpenTelemetry metrics and distributed tracing support.
  primary_cta:
    label: Enable instrumentation
    href: "#instrumentation"
  secondary_cta:
    label: View example →
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/observability"
    external: true
sidebar:
  - title: Instrumentation
    url: "#instrumentation"
  - title: Metrics
    url: "#metrics"
  - title: Tracing
    url: "#tracing"
  - title: Integration example
    url: "#integration-example"
---

## Instrumentation {#instrumentation}

AgentMesh provides built-in instrumentation through the `graph.Instrumentation` type. Enable it when executing graphs to collect metrics and traces:

```go
import (
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/metrics"
    "github.com/hupe1980/agentmesh/pkg/trace"
)

// Create instrumentation with providers
metricsProvider := metrics.Noop()  // or metrics/opentelemetry.New(meterProvider)
traceProvider := trace.Noop()      // or trace/opentelemetry.New(tracerProvider)

inst := graph.NewInstrumentation(metricsProvider, traceProvider)
```

Use the instrumentation in your graph nodes:

```go
builder.Node("agent", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
    // Start a trace span for this node
    ctx, span := inst.TraceNodeExecution(ctx, "agent", superstep)
    defer span.End()
    
    // Record metrics
    inst.RecordNodeExecution(ctx, "agent", duration, err)
    
    // Your node logic...
    return &graph.NodeResult{...}, nil
})
```

---

## Metrics {#metrics}

The metrics package provides an abstraction over OpenTelemetry metrics:

The OpenTelemetry adapter (`metrics/opentelemetry`) bridges AgentMesh metrics to any OTLP backend. Swap it with your own implementation if you prefer Prometheus, StatsD, or another collector.

---

## Tracing {#tracing}

Tracing hooks connect spans around every run, agent, and tool invocation. Retrieve the provider via `trace.FromContext(ctx)` to start spans within your custom code.

### Built-in metrics

AgentMesh tracks:
- **Node execution time** – Duration of each node execution
- **Node errors** – Count of failed node executions
- **Graph execution time** – Total time for graph invocations
- **Superstep count** – Number of supersteps per execution

### OpenTelemetry integration

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/prometheus"
    "go.opentelemetry.io/otel/sdk/metric"
)

// Create OpenTelemetry meter provider
exporter, _ := prometheus.New()
provider := metric.NewMeterProvider(
    metric.WithReader(exporter),
)
otel.SetMeterProvider(provider)

// Use in AgentMesh
metricsProvider := metrics.NewOpenTelemetry(provider)
inst := graph.NewInstrumentation(metricsProvider, traceProvider)
```

---

## Tracing {#tracing}

Distributed tracing helps you understand the execution flow of complex graphs:

### Built-in traces

AgentMesh creates spans for:
- **Graph execution** – Top-level span for entire graph invocation
- **Node execution** – Individual spans for each node
- **Supersteps** – Group nodes executed in parallel

### OpenTelemetry integration

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/jaeger"
    "go.opentelemetry.io/otel/sdk/trace"
)

// Create OpenTelemetry tracer provider
exporter, _ := jaeger.New(jaeger.WithCollectorEndpoint())
provider := trace.NewTracerProvider(
    trace.WithBatcher(exporter),
)
otel.SetTracerProvider(provider)

// Use in AgentMesh
traceProvider := trace.NewOpenTelemetry(provider)
inst := graph.NewInstrumentation(metricsProvider, traceProvider)
```

---

## Integration example {#integration-example}

Complete example with Prometheus metrics and Jaeger tracing:

```go
package main

import (
    "context"
    "log"
    
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/graph"
    "github.com/hupe1980/agentmesh/pkg/message"
    "github.com/hupe1980/agentmesh/pkg/metrics"
    "github.com/hupe1980/agentmesh/pkg/model/openai"
    "github.com/hupe1980/agentmesh/pkg/trace"
    
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/prometheus"
    sdkmetric "go.opentelemetry.io/otel/sdk/metric"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
    // Setup OpenTelemetry
    promExporter, _ := prometheus.New()
    meterProvider := sdkmetric.NewMeterProvider(
        sdkmetric.WithReader(promExporter),
    )
    otel.SetMeterProvider(meterProvider)
    
    tracerProvider := sdktrace.NewTracerProvider()
    otel.SetTracerProvider(tracerProvider)
    
    // Create instrumentation
    metricsProvider := metrics.NewOpenTelemetry(meterProvider)
    traceProvider := trace.NewOpenTelemetry(tracerProvider)
    inst := graph.NewInstrumentation(metricsProvider, traceProvider)
    
    // Create agent with instrumentation
    compiled, err := agent.NewReActAgent(openai.NewModel(), tools)
    if err != nil {
        log.Fatal(err)
    }
    
    // Execute with tracing
    ctx := context.Background()
    ctx, span := inst.TraceGraphExecution(ctx, "my-agent")
    defer span.End()
    
    results, err := compiled.Invoke(ctx, messages)
    if err != nil {
        log.Fatal(err)
    }
    
    // Metrics are automatically exported to Prometheus
    // Traces are sent to Jaeger (if configured)
}
```

See `examples/observability` for a complete working example.
