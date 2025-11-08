// Package main demonstrates production-grade observability with metrics and distributed tracing.
//
// This example shows how to:
//   - Integrate OpenTelemetry for metrics collection and distributed tracing
//   - Track graph execution performance with timing metrics
//   - Create trace spans for node execution and message delivery
//   - Monitor graph health and performance in production
//   - Record custom metrics for supersteps, nodes, and operations
//
// Key concepts:
//   - Instrumentation: Wrapper for metrics and tracing providers
//   - MetricsProvider: Records performance metrics (node duration, message count)
//   - TraceProvider: Creates distributed trace spans for debugging
//   - Production Setup: Replace Noop() with OpenTelemetry providers
//
// Production integration:
//   import "github.com/hupe1980/agentmesh/pkg/metrics/opentelemetry"
//   import "github.com/hupe1980/agentmesh/pkg/trace/opentelemetry"
//   metricsProvider := opentelemetry.New(meterProvider)
//   traceProvider := opentelemetry.New(tracerProvider)
//
// Run: go run main.go

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// This example demonstrates using the observability instrumentation
// to track graph execution with metrics and tracing.

func main() {
	// For this example, we'll use a simple in-memory metrics collector
	// In production, you would use:
	// - metrics/opentelemetry.New() for Prometheus metrics
	// - trace/opentelemetry.New() for distributed tracing

	metricsProvider := metrics.Noop() // Replace with opentelemetry.New(meterProvider)
	traceProvider := trace.Noop()     // Replace with opentelemetry.New(tracerProvider)

	inst := graph.NewInstrumentation(metricsProvider, traceProvider)

	// Create a simple graph
	state := graph.NewStateManager(0) // Unlimited messages
	state.Set("counter", 0)
	g := graph.NewGraph(state)

	// Add nodes with instrumentation
	if err := g.AddNode(&graph.Node{
		Name: "step1",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			// Start a trace span for this node
			ctx, span := inst.TraceNodeExecution(ctx, "step1", 1)
			defer span.End(nil)

			start := time.Now()

			// Simulate work
			time.Sleep(10 * time.Millisecond)
			counter, _ := s.Get("counter").(int)
			result := &graph.NodeResult{
				Updates: map[string]any{"counter": counter + 1},
			}

			// Record metrics
			inst.RecordNodeExecution(ctx, "step1", time.Since(start), nil)

			return result, nil
		},
	}); err != nil {
		log.Fatal(err)
	}

	if err := g.AddNode(&graph.Node{
		Name: "step2",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			ctx, span := inst.TraceNodeExecution(ctx, "step2", 1)
			defer span.End(nil)

			start := time.Now()

			time.Sleep(15 * time.Millisecond)
			counter, _ := s.Get("counter").(int)
			result := &graph.NodeResult{
				Updates: map[string]any{"counter": counter + 10},
			}

			inst.RecordNodeExecution(ctx, "step2", time.Since(start), nil)

			return result, nil
		},
	}); err != nil {
		log.Fatal(err)
	}

	g.AddEdge(graph.StartNode, "step1")
	g.AddEdge("step1", "step2")

	compiled, err := g.Compile()
	if err != nil {
		log.Fatal(err)
	}

	// Execute with tracing
	ctx := context.Background()
	ctx, graphSpan := inst.TraceGraphExecution(ctx, "example-graph")

	startTime := time.Now()
	_, err = compiled.Invoke(ctx, nil)
	duration := time.Since(startTime)

	if err != nil {
		graphSpan.End(err)
		log.Fatal(err)
	}
	graphSpan.End(nil)

	// Record graph-level metrics
	inst.RecordGraphExecution(ctx, "example-graph", duration, true)

	// Print results
	counter, _ := compiled.State().Get("counter").(int)
	fmt.Printf("Final counter value: %d\n", counter)
	fmt.Printf("Total execution time: %v\n", duration)

	fmt.Println("\nObservability instrumentation configured!")
	fmt.Println("In production, metrics would be exported to Prometheus")
	fmt.Println("and traces would be sent to your distributed tracing backend (Jaeger, Zipkin, etc.)")
}
