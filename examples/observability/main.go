// Package main demonstrates production-grade observability with metrics and distributed tracing.
//
// This example shows how to:
//   - Configure observability using explicit graph options
//   - Automatically instrument graph execution with traces and metrics
//   - Use providers in node RunFuncs via FromContext()
//   - Monitor graph health and performance in production
//
// Key concepts:
//   - WithLogger/WithTracer/WithMetrics: Explicit observability configuration
//   - Automatic instrumentation: Framework creates spans/metrics automatically
//   - FromContext(): Access providers in node RunFuncs for custom instrumentation
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

	"github.com/hupe1980/agentmesh/pkg/exec"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

// This example demonstrates using automatic observability instrumentation
// with explicit provider configuration.

func main() {
	// Create observability providers (using noop for demonstration)
	// In production, replace with OpenTelemetry implementations
	logger := logging.NoopLogger{}
	metricsProvider := metrics.Noop() // Replace with opentelemetry.New(meterProvider)
	traceProvider := trace.Noop()     // Replace with opentelemetry.New(tracerProvider)

	// Create a simple graph
	state, err := graphstate.NewStateManager(0) // Unlimited messages
	if err != nil {
		panic(err)
	}
	state.Set("counter", 0)
	g, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}

	// Add nodes that use providers via FromContext()
	// Automatic instrumentation happens behind the scenes
	if err := g.AddNode(&graph.Node{
		Name: "step1",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			// Access logger and tracer from context if needed for custom instrumentation
			log := logging.FromContext(ctx)
			log.Info("Processing step1", "node", "step1")

			// Optional: Create custom spans for sub-operations
			tp := trace.FromContext(ctx)
			tracer := tp.Tracer("observability-example")
			ctx, span := tracer.Start(ctx, "business-logic")
			defer span.End(nil)

			// Simulate work
			time.Sleep(10 * time.Millisecond)
			counter, _ := s.Get("counter").(int)

			return &graph.NodeResult{
				Updates: map[string]any{"counter": counter + 1},
			}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}

	if err := g.AddNode(&graph.Node{
		Name: "step2",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			log := logging.FromContext(ctx)
			log.Info("Processing step2", "node", "step2")

			// Optional: Record custom metrics
			mp := metrics.FromContext(ctx)
			counter := mp.Counter("custom.operations")
			counter.Add(ctx, 1, metrics.Attr{Key: "operation", Value: "step2"})

			time.Sleep(15 * time.Millisecond)
			currentCounter, _ := s.Get("counter").(int)

			return &graph.NodeResult{
				Updates: map[string]any{"counter": currentCounter + 10},
			}, nil
		},
	}); err != nil {
		log.Fatal(err)
	}

	g.AddEdge(graph.StartNode, "step1")
	g.AddEdge("step1", "step2")

	compiled, err := exec.CompileGraph(g)
	if err != nil {
		log.Fatal(err)
	}
	rg := compiled.(*exec.RunnableGraph)
	if err != nil {
		log.Fatal(err)
	}

	// Execute with explicit observability configuration
	// Framework automatically creates spans and records metrics
	ctx := context.Background()
	startTime := time.Now()

	for _, err := range compiled.Run(ctx, nil,
		graph.WithLogger(logger),
		graph.WithTracer(traceProvider),
		graph.WithMetrics(metricsProvider),
	) {
		if err != nil {
			log.Fatalf("Execution failed: %v", err)
		}
	}

	duration := time.Since(startTime)

	// Print results
	counter, _ := rg.State().Get("counter").(int)
	fmt.Printf("Final counter value: %d\n", counter)
	fmt.Printf("Total execution time: %v\n", duration)

	fmt.Println("\n=== Observability Configuration ===")
	fmt.Println("✓ Automatic instrumentation enabled via:")
	fmt.Println("  - graph.WithLogger(logger)")
	fmt.Println("  - graph.WithTracer(traceProvider)")
	fmt.Println("  - graph.WithMetrics(metricsProvider)")
	fmt.Println("\n✓ Framework automatically creates:")
	fmt.Println("  - Trace spans for each node execution")
	fmt.Println("  - Metrics for node duration and errors")
	fmt.Println("  - Structured logs (if logger configured)")
	fmt.Println("\n✓ Nodes can access providers via FromContext()")
	fmt.Println("  - logging.FromContext(ctx)")
	fmt.Println("  - trace.FromContext(ctx)")
	fmt.Println("  - metrics.FromContext(ctx)")
	fmt.Println("\n=== Production Setup ===")
	fmt.Println("Replace noop providers with:")
	fmt.Println("- Tracer: trace.NewOpenTelemetryProvider(...)")
	fmt.Println("- Metrics: metrics.NewOpenTelemetryProvider(...)")
	fmt.Println("- Logger: logging.NewSlogAdapter(slog.New(...))")
}
