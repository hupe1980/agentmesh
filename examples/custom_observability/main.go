// Package main demonstrates using custom observability providers (logging, tracing, metrics)
// in node RunFuncs via context propagation.
//
// This example shows:
// 1. How to attach custom providers to context
// 2. How to retrieve them in node RunFuncs using FromContext()
// 3. How providers propagate through the entire execution chain
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/metrics"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/trace"
)

func main() {
	fmt.Println("=== Custom Observability Example ===")
	fmt.Println("Demonstrating context-based provider propagation to node RunFuncs")

	// ============================================================
	// Step 1: Create Custom Observability Providers
	// ============================================================

	// Create a structured JSON logger
	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := logging.NewSlogAdapter(slog.New(slogHandler))

	// Create trace and metrics providers (using noop for demo, but you can use OpenTelemetry)
	traceProvider := trace.Noop()
	metricsProvider := metrics.Noop()

	fmt.Println("✓ Created custom observability providers")
	fmt.Println("  - Logger: Structured JSON logger (slog)")
	fmt.Println("  - Tracer: Noop (replace with OpenTelemetry in production)")
	fmt.Println("  - Metrics: Noop (replace with Prometheus in production)")

	// ============================================================
	// Step 2: Build Graph with Nodes that Use Context Providers
	// ============================================================

	state, err := graphstate.NewStateManager(0)
	if err != nil {
		panic(err)
	}
	g, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}

	// Node 1: Data Ingestion - demonstrates logger usage
	if err := g.AddNode(&graph.Node{
		Name: "ingest_data",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			// Retrieve logger from context
			log := logging.FromContext(ctx)
			log.Info("Starting data ingestion", "node", "ingest_data")

			// Simulate data ingestion
			time.Sleep(50 * time.Millisecond)

			data := map[string]any{
				"records":   []string{"record1", "record2", "record3"},
				"timestamp": time.Now().Format(time.RFC3339),
			}

			log.Info("Data ingested successfully",
				"record_count", len(data["records"].([]string)),
				"timestamp", data["timestamp"])

			return &graph.NodeResult{
				Updates: map[string]any{"raw_data": data},
			}, nil
		},
	}); err != nil {
		panic(err)
	}

	// Node 2: Data Processing - demonstrates tracer usage
	if err := g.AddNode(&graph.Node{
		Name: "process_data",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			log := logging.FromContext(ctx)
			log.Info("Starting data processing", "node", "process_data")

			// Retrieve trace provider and create custom span
			tp := trace.FromContext(ctx)
			tracer := tp.Tracer("custom-observability-example")
			ctx, span := tracer.Start(ctx, "data-processing-operation",
				trace.Attr{Key: "node", Value: "process_data"},
				trace.Attr{Key: "operation", Value: "transform"},
			)
			defer span.End(nil)

			// Get raw data
			rawData, _ := s.Get("raw_data").(map[string]any)
			records, _ := rawData["records"].([]string)

			log.Info("Processing records", "count", len(records))

			// Simulate processing with nested span
			_, innerSpan := tracer.Start(ctx, "transform-records")
			processedRecords := make([]string, len(records))
			for i, record := range records {
				processedRecords[i] = fmt.Sprintf("processed_%s", record)
				time.Sleep(10 * time.Millisecond)
			}
			innerSpan.End(nil)

			result := map[string]any{
				"processed_records": processedRecords,
				"processed_at":      time.Now().Format(time.RFC3339),
			}

			log.Info("Processing completed", "processed_count", len(processedRecords))

			return &graph.NodeResult{
				Updates: map[string]any{"processed_data": result},
			}, nil
		},
	}); err != nil {
		panic(err)
	}

	// Node 3: Data Validation - demonstrates metrics usage
	if err := g.AddNode(&graph.Node{
		Name: "validate_data",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			log := logging.FromContext(ctx)
			log.Info("Starting data validation", "node", "validate_data")

			// Retrieve metrics provider and record metrics
			mp := metrics.FromContext(ctx)
			validationCounter := mp.Counter("validation.operations")
			validationDuration := mp.Histogram("validation.duration_ms")

			start := time.Now()

			// Record validation started
			validationCounter.Add(ctx, 1,
				metrics.Attr{Key: "node", Value: "validate_data"},
				metrics.Attr{Key: "status", Value: "started"},
			)

			// Get processed data
			processedData, _ := s.Get("processed_data").(map[string]any)
			records, _ := processedData["processed_records"].([]string)

			log.Info("Validating records", "count", len(records))

			// Simulate validation
			validRecords := 0
			invalidRecords := 0
			for _, record := range records {
				time.Sleep(5 * time.Millisecond)
				if len(record) > 5 { // Simple validation rule
					validRecords++
				} else {
					invalidRecords++
				}
			}

			// Record metrics
			duration := time.Since(start)
			validationDuration.Record(ctx, float64(duration.Milliseconds()),
				metrics.Attr{Key: "node", Value: "validate_data"},
			)

			validationCounter.Add(ctx, 1,
				metrics.Attr{Key: "node", Value: "validate_data"},
				metrics.Attr{Key: "status", Value: "completed"},
			)

			result := map[string]any{
				"valid_count":            validRecords,
				"invalid_count":          invalidRecords,
				"validation_duration_ms": duration.Milliseconds(),
			}

			log.Info("Validation completed",
				"valid", validRecords,
				"invalid", invalidRecords,
				"duration_ms", duration.Milliseconds())

			return &graph.NodeResult{
				Updates: map[string]any{"validation_result": result},
			}, nil
		},
	}); err != nil {
		panic(err)
	}

	// Node 4: Summary - demonstrates all providers together
	if err := g.AddNode(&graph.Node{
		Name: "generate_summary",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			log := logging.FromContext(ctx)
			tp := trace.FromContext(ctx)
			mp := metrics.FromContext(ctx)

			// Create span for summary generation
			tracer := tp.Tracer("custom-observability-example")
			ctx, span := tracer.Start(ctx, "summary-generation")
			defer span.End(nil)

			log.Info("Generating summary", "node", "generate_summary")

			// Get validation results
			validationResult, _ := s.Get("validation_result").(map[string]any)

			summary := fmt.Sprintf("Validation Summary: %d valid, %d invalid records processed in %dms",
				validationResult["valid_count"],
				validationResult["invalid_count"],
				validationResult["validation_duration_ms"])

			// Record summary generation metric
			summaryCounter := mp.Counter("summary.generated")
			summaryCounter.Add(ctx, 1,
				metrics.Attr{Key: "node", Value: "generate_summary"},
			)

			log.Info("Summary generated", "summary", summary)

			return &graph.NodeResult{
				Updates: map[string]any{"summary": summary},
			}, nil
		},
	}); err != nil {
		panic(err)
	}

	// Build execution graph
	g.AddEdge(graph.StartNode, "ingest_data")
	g.AddEdge("ingest_data", "process_data")
	g.AddEdge("process_data", "validate_data")
	g.AddEdge("validate_data", "generate_summary")
	g.AddEdge("generate_summary", graph.EndNode)

	compiled, err := exec.CompileGraph(g)
	if err != nil {
		log.Fatal(err)
	}
	rg := compiled.(*exec.RunnableGraph)
	if err != nil {
		panic(err)
	}

	fmt.Println("✓ Built graph with 4 nodes:")
	fmt.Println("  START → ingest_data → process_data → validate_data → generate_summary → END")

	// ============================================================
	// Step 3: Execute Graph with Explicit Provider Options
	// ============================================================

	fmt.Println("Executing graph with custom observability providers...")
	fmt.Println("--- Structured Logs (JSON) ---")

	// Execute graph with explicit provider options
	// The graph will automatically attach providers to context for all nodes
	ctx := context.Background()
	start := time.Now()
	_, err = graph.Last(compiled.Run(ctx, nil,
		graph.WithLogger(logger),
		graph.WithTracer(traceProvider),
		graph.WithMetrics(metricsProvider),
	))
	if err != nil {
		panic(err)
	}
	duration := time.Since(start)

	fmt.Println("\n--- Execution Complete ---")
	fmt.Printf("Total execution time: %v\n", duration)

	// Get final state
	finalState := rg.State()
	summary := finalState.Get("summary")

	fmt.Printf("\nFinal Summary: %v\n", summary)

	fmt.Println("\n=== Key Takeaways ===")
	fmt.Println("✓ Providers configured using explicit graph options:")
	fmt.Println("  - graph.WithLogger(logger)")
	fmt.Println("  - graph.WithTracer(traceProvider)")
	fmt.Println("  - graph.WithMetrics(metricsProvider)")
	fmt.Println("✓ Graph automatically attaches providers to context")
	fmt.Println("✓ Each node retrieves providers using FromContext()")
	fmt.Println("✓ Logger: logging.FromContext(ctx)")
	fmt.Println("✓ Tracer: trace.FromContext(ctx)")
	fmt.Println("✓ Metrics: metrics.FromContext(ctx)")
	fmt.Println("✓ All nodes share the same providers automatically")
	fmt.Println("\n=== Production Usage ===")
	fmt.Println("Replace noop providers with real implementations:")
	fmt.Println("- Tracer: trace.NewOpenTelemetryProvider(...)")
	fmt.Println("- Metrics: metrics.NewOpenTelemetryProvider(...)")
	fmt.Println("- Logger: Your custom slog/zap/zerolog adapter")
}
