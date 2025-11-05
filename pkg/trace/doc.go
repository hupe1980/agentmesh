/*
Package trace provides OpenTelemetry distributed tracing for graph execution flows.

# Overview

The trace package instruments graph and agent execution with distributed traces:
  - Graph spans: Top-level trace for entire invocation
  - Node spans: Individual node execution within graph
  - Superstep spans: BSP superstep boundaries
  - Tool spans: Tool invocation timing

# Quick Start

Enable tracing in your graph:

	import (
		"github.com/hupe1980/agentmesh/pkg/graph"
		"github.com/hupe1980/agentmesh/pkg/trace"
		"go.opentelemetry.io/otel"
	)

	tracer := otel.Tracer("agentmesh")
	recorder := trace.NewOpenTelemetryRecorder(tracer)

	compiled, _ := builder.Compile(
		graph.WithInstrumentation(&graph.Instrumentation{
			TraceRecorder: recorder,
		}),
	)

# Span Hierarchy

Traces follow this structure:

	graph.invoke
	├── superstep.0
	│   ├── node.agent
	│   │   ├── model.generate
	│   │   └── tool.get_weather
	│   └── node.tools
	├── superstep.1
	│   └── node.agent
	└── superstep.2

# Jaeger Integration

Export traces to Jaeger:

	import (
		"go.opentelemetry.io/otel/exporters/jaeger"
		sdktrace "go.opentelemetry.io/otel/sdk/trace"
	)

	exporter, _ := jaeger.New(jaeger.WithCollectorEndpoint(
		jaeger.WithEndpoint("http://localhost:14268/api/traces"),
	))

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("agentmesh-app"),
		)),
	)
	otel.SetTracerProvider(provider)

# Trace Attributes

Spans include attributes for debugging:
  - node.name: Node identifier
  - node.type: Node type (agent, tool, etc.)
  - superstep: BSP superstep number
  - graph.run_id: Unique run identifier
  - error: true if node failed
  - retry.attempt: Retry attempt number

# Custom Recorders

Implement the Recorder interface for custom backends:

	type Recorder interface {
		StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span)
		RecordEvent(ctx context.Context, event string, attrs ...attribute.KeyValue)
		RecordError(ctx context.Context, err error)
		EndSpan(ctx context.Context)
	}
*/
package trace
