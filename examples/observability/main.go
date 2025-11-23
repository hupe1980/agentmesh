// Package main demonstrates production-grade observability with metrics and distributed tracing.
//
// This example shows how to:
//   - Configure observability using context-based providers
//   - Automatically instrument graph execution with traces and metrics
//   - Use providers in node RunFuncs via FromContext()
//   - Monitor graph health and performance in production
//
// Key concepts:
//   - logging.WithLogger(ctx, logger): Attach logger to context
//   - trace.WithProvider(ctx, tp): Attach trace provider to context
//   - metrics.WithProvider(ctx, mp): Attach metrics provider to context
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

	"github.com/hupe1980/agentmesh/pkg/agent"

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

	// Define type-safe state counter
	counterKey := graphstate.NewKey("counter", 0)

	// Create a simple graph
	mgr := graphstate.NewManager()
	if err := agent.RegisterMessagesKey(mgr); err != nil {
		log.Fatal(err)
	}
	graphstate.RegisterKey(mgr, counterKey)

	g, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}

	// Add nodes that use providers via FromContext()
	// Automatic instrumentation happens behind the scenes
	if err := g.AddNode(&graph.BaseCommandNode{
		NodeName:        "step1",
		DeclaredTargets: graph.NewTargetSet("step2"),
		Fn: func(ctx context.Context, view *graphstate.ReadView) (*graph.Command, error) {
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

			// Increment counter
			counter := graphstate.GetFromView(view, counterKey)
			counter++

			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, counterKey, counter)
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.Goto("step2", updates), nil
		},
	}); err != nil {
		log.Fatal(err)
	}

	if err := g.AddNode(&graph.BaseCommandNode{
		NodeName:        "step2",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view *graphstate.ReadView) (*graph.Command, error) {
			log := logging.FromContext(ctx)
			log.Info("Processing step2", "node", "step2")

			// Optional: Record custom metrics
			mp := metrics.FromContext(ctx)
			counter := mp.Counter("custom.operations")
			counter.Add(ctx, 1, metrics.Attr{Key: "operation", Value: "step2"})

			time.Sleep(15 * time.Millisecond)

			// Read and update counter
			currentCounter := graphstate.GetFromView(view, counterKey)
			newValue := currentCounter + 10

			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, counterKey, newValue)
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.End(updates), nil
		},
	}); err != nil {
		log.Fatal(err)
	}

	if err := g.SetEntryPoint("step1"); err != nil {
		panic(err)
	}

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	// Attach observability providers to context
	// Nodes access these via FromContext() methods
	ctx := context.Background()
	ctx = logging.WithLogger(ctx, logger)
	ctx = trace.WithProvider(ctx, traceProvider)
	ctx = metrics.WithProvider(ctx, metricsProvider)

	startTime := time.Now()

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatalf("Execution failed: %v", err)
		}
	}

	duration := time.Since(startTime)

	// Print results
	finalView, err := mgr.CreateReadView(ctx)
	if err != nil {
		panic(err)
	}
	counter := graphstate.GetFromView(finalView, counterKey)
	fmt.Printf("Final counter value: %d\n", counter)
	fmt.Printf("Total execution time: %v\n", duration)

	fmt.Println("\n=== Observability Configuration ===")
	fmt.Println("✓ Observability providers attached to context via:")
	fmt.Println("  - logging.WithLogger(ctx, logger)")
	fmt.Println("  - trace.WithProvider(ctx, traceProvider)")
	fmt.Println("  - metrics.WithProvider(ctx, metricsProvider)")
	fmt.Println("\n✓ Nodes access providers via FromContext():")
	fmt.Println("  - logging.FromContext(ctx)")
	fmt.Println("  - trace.FromContext(ctx)")
	fmt.Println("  - metrics.FromContext(ctx)")
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
