/*
Package metrics provides OpenTelemetry metrics collection for graph execution and agent performance.

# Overview

The metrics package instruments graph execution with key performance indicators:
  - Node execution latency (histogram)
  - Graph invocation duration (histogram)
  - Superstep counts (counter)
  - Error rates per node (counter)
  - Message throughput (counter)

# Quick Start

Enable metrics in your graph:

	import (
		"github.com/hupe1980/agentmesh/pkg/graph"
		"github.com/hupe1980/agentmesh/pkg/metrics"
	)

	// Create OpenTelemetry meter
	meter := otel.Meter("agentmesh")
	recorder := metrics.NewOpenTelemetryRecorder(meter)

	compiled, _ := g.Build(
		graph.WithInstrumentation(&graph.Instrumentation{
			MetricsRecorder: recorder,
		}),
	)

# Metrics Collected

Node Execution:
  - agentmesh.node.duration: Histogram of node execution time
  - agentmesh.node.errors: Counter of errors per node
  - agentmesh.node.invocations: Counter of node executions

Graph Execution:
  - agentmesh.graph.duration: Histogram of full graph execution
  - agentmesh.graph.supersteps: Counter of supersteps per run
  - agentmesh.graph.messages: Counter of messages processed

# Prometheus Integration

Export metrics to Prometheus:

	import (
		"github.com/prometheus/client_golang/prometheus/promhttp"
		"go.opentelemetry.io/otel/exporters/prometheus"
	)

	exporter, _ := prometheus.New()
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":9090", nil)

# Custom Recorders

Implement the Recorder interface for custom backends:

	type Recorder interface {
		RecordNodeLatency(ctx context.Context, node string, duration time.Duration)
		RecordGraphLatency(ctx context.Context, duration time.Duration)
		RecordSuperstep(ctx context.Context, superstep int)
		RecordError(ctx context.Context, node string, err error)
	}
*/
package metrics
