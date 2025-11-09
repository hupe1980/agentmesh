package graph

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// Instrumentation provides observability hooks for graph execution.
// It is created internally from WithLogger(), WithTracer(), and WithMetrics() options.
type Instrumentation struct {
	metrics metrics.Provider
	trace   trace.Provider

	// Metrics
	nodeExecutions   metrics.Counter
	nodeErrors       metrics.Counter
	nodeLatency      metrics.Histogram
	superstepLatency metrics.Histogram
	graphExecutions  metrics.Counter

	// Tracer
	tracer trace.Tracer
}

// newInstrumentation creates observability instrumentation for graph execution.
// This is an internal function called by compiled_graph.go.
func newInstrumentation(mp metrics.Provider, tp trace.Provider) *Instrumentation {
	if mp == nil {
		mp = metrics.Noop()
	}
	if tp == nil {
		tp = trace.Noop()
	}

	inst := &Instrumentation{
		metrics: mp,
		trace:   tp,
		tracer:  tp.Tracer("agentgraph.graph"),
	}

	// Initialize metrics
	inst.nodeExecutions = mp.Counter("agentgraph.node.executions")
	inst.nodeErrors = mp.Counter("agentgraph.node.errors")
	inst.nodeLatency = mp.Histogram("agentgraph.node.latency_ms")
	inst.superstepLatency = mp.Histogram("agentgraph.superstep.latency_ms")
	inst.graphExecutions = mp.Counter("agentgraph.graph.executions")

	return inst
}

// TraceGraphExecution starts a trace span for graph execution.
func (i *Instrumentation) TraceGraphExecution(ctx context.Context, graphName string) (context.Context, trace.Span) {
	if i == nil || i.tracer == nil {
		return ctx, noopSpan{}
	}

	return i.tracer.Start(ctx, "graph.execute",
		trace.Attr{Key: "graph.name", Value: graphName},
	)
}

// TraceNodeExecution starts a trace span for node execution.
func (i *Instrumentation) TraceNodeExecution(ctx context.Context, nodeName string, superstep int64) (context.Context, trace.Span) {
	if i == nil || i.tracer == nil {
		return ctx, noopSpan{}
	}

	return i.tracer.Start(ctx, "node.execute",
		trace.Attr{Key: "node.name", Value: nodeName},
		trace.Attr{Key: "superstep", Value: superstep},
	)
}

type noopSpan struct{}

func (noopSpan) End(error) {}

// RecordNodeExecution records metrics for a completed node execution.
func (i *Instrumentation) RecordNodeExecution(ctx context.Context, nodeName string, duration time.Duration, err error) {
	if i == nil {
		return
	}

	attrs := []metrics.Attr{
		{Key: "node.name", Value: nodeName},
	}

	i.nodeExecutions.Add(ctx, 1, attrs...)
	i.nodeLatency.Record(ctx, float64(duration.Milliseconds()), attrs...)

	if err != nil {
		i.nodeErrors.Add(ctx, 1, append(attrs,
			metrics.Attr{Key: "error.type", Value: err.Error()},
		)...)
	}
}

// RecordSuperstep records metrics for a completed superstep.
func (i *Instrumentation) RecordSuperstep(ctx context.Context, superstep int64, duration time.Duration, nodeCount int) {
	if i == nil {
		return
	}

	attrs := []metrics.Attr{
		{Key: "superstep", Value: superstep},
		{Key: "node_count", Value: nodeCount},
	}

	i.superstepLatency.Record(ctx, float64(duration.Milliseconds()), attrs...)
}

// RecordGraphExecution records metrics for a completed graph execution.
func (i *Instrumentation) RecordGraphExecution(ctx context.Context, graphName string, duration time.Duration, success bool) {
	if i == nil {
		return
	}

	attrs := []metrics.Attr{
		{Key: "graph.name", Value: graphName},
		{Key: "success", Value: success},
	}

	i.graphExecutions.Add(ctx, 1, attrs...)
}

// TraceSuperstep starts a trace span for a superstep execution.
func (i *Instrumentation) TraceSuperstep(ctx context.Context, superstep int64) (context.Context, trace.Span) {
	if i == nil || i.tracer == nil {
		return ctx, noopSpan{}
	}

	return i.tracer.Start(ctx, "superstep.execute",
		trace.Attr{Key: "superstep", Value: superstep},
	)
}

// TraceCheckpoint starts a trace span for checkpoint save/restore operations.
func (i *Instrumentation) TraceCheckpoint(ctx context.Context, operation string, runID string, superstep int64) (context.Context, trace.Span) {
	if i == nil || i.tracer == nil {
		return ctx, noopSpan{}
	}

	return i.tracer.Start(ctx, "checkpoint."+operation,
		trace.Attr{Key: "run_id", Value: runID},
		trace.Attr{Key: "superstep", Value: superstep},
	)
}
