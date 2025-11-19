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

	"github.com/hupe1980/agentmesh/pkg/agent"

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

	recordsKey := graphstate.NewKey("records", []string{})
	timestampKey := graphstate.NewKey("timestamp", "")
	processedKey := graphstate.NewKey("processed", 0)
	qualityScoreKey := graphstate.NewKey("quality_score", 0.0)
	statusKey := graphstate.NewKey("status", "")

	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, agent.MessagesKey.Key)
	graphstate.RegisterKey(mgr, recordsKey)
	graphstate.RegisterKey(mgr, timestampKey)
	graphstate.RegisterKey(mgr, processedKey)
	graphstate.RegisterKey(mgr, qualityScoreKey)
	graphstate.RegisterKey(mgr, statusKey)

	g, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}

	// Node 1: Data Ingestion - demonstrates logger usage
	if err := g.AddNode(graph.NewBaseNode("ingest_data",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
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

			return map[string]any{"raw_data": data}, nil
		},
	)); err != nil {
		panic(err)
	}

	// Node 2: Data Processing - demonstrates tracer usage
	if err := g.AddNode(graph.NewBaseNode("process_data",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
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
			rawDataKey := graphstate.NewKey("raw_data", map[string]any{})
			rawData := graphstate.GetFromView(view, rawDataKey)
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

			return map[string]any{"processed_data": result}, nil
		},
	)); err != nil {
		panic(err)
	}

	// Node 3: Data Validation - demonstrates metrics usage
	if err := g.AddNode(graph.NewBaseNode("validate_data",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
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
			processedDataKey := graphstate.NewKey("processed_data", map[string]any{})
			processedData := graphstate.GetFromView(view, processedDataKey)
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

			return map[string]any{"validation_result": result}, nil
		},
	)); err != nil {
		panic(err)
	}

	// Node 4: Summary - demonstrates all providers together
	if err := g.AddNode(graph.NewBaseNode("generate_summary",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			log := logging.FromContext(ctx)
			tp := trace.FromContext(ctx)
			mp := metrics.FromContext(ctx)

			// Create span for summary generation
			tracer := tp.Tracer("custom-observability-example")
			ctx, span := tracer.Start(ctx, "summary-generation")
			defer span.End(nil)

			log.Info("Generating summary", "node", "generate_summary")

			// Get validation results
			validationResultKey := graphstate.NewKey("validation_result", map[string]any{})
			validationResult := graphstate.GetFromView(view, validationResultKey)

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

			return map[string]any{"summary": summary}, nil
		},
	)); err != nil {
		panic(err)
	}

	// Build execution graph
	g.AddEdge(graph.StartNode, "ingest_data")
	g.AddEdge("ingest_data", "process_data")
	g.AddEdge("process_data", "validate_data")
	g.AddEdge("validate_data", "generate_summary")
	g.AddEdge("generate_summary", graph.EndNode)

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Built graph with 4 nodes:")
	fmt.Println("  START → ingest_data → process_data → validate_data → generate_summary → END")

	// ============================================================
	// Step 3: Execute Graph with Explicit Provider Options
	// ============================================================

	fmt.Println("Executing graph with custom observability providers...")
	fmt.Println("--- Structured Logs (JSON) ---")

	// Attach providers to context - nodes access via FromContext()
	ctx := context.Background()
	ctx = logging.WithLogger(ctx, logger)
	ctx = trace.WithProvider(ctx, traceProvider)
	ctx = metrics.WithProvider(ctx, metricsProvider)

	start := time.Now()
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			panic(fmt.Sprintf("Execution failed: %v", err))
		}
	}
	duration := time.Since(start)

	fmt.Println("\n--- Execution Complete ---")
	fmt.Printf("Total execution time: %v\n", duration)

	// Get final state
	finalState := compiled.Manager()
	summaryKey := graphstate.NewKey("summary", "")
	view, err := finalState.CreateReadView(context.Background())
	if err != nil {
		panic(err)
	}
	summary := graphstate.GetFromView(view, summaryKey)

	fmt.Printf("\nFinal Summary: %v\n", summary)

	fmt.Println("\n=== Key Takeaways ===")
	fmt.Println("✓ Providers attached to context before execution:")
	fmt.Println("  - logging.WithLogger(ctx, logger)")
	fmt.Println("  - trace.WithProvider(ctx, traceProvider)")
	fmt.Println("  - metrics.WithProvider(ctx, metricsProvider)")
	fmt.Println("✓ Nodes retrieve providers using FromContext():")
	fmt.Println("  - logging.FromContext(ctx)")
	fmt.Println("  - trace.FromContext(ctx)")
	fmt.Println("  - metrics.FromContext(ctx)")
	fmt.Println("✓ All nodes share the same providers via context")
	fmt.Println("\n=== Production Usage ===")
	fmt.Println("Replace noop providers with real implementations:")
	fmt.Println("- Tracer: trace.NewOpenTelemetryProvider(...)")
	fmt.Println("- Metrics: metrics.NewOpenTelemetryProvider(...)")
	fmt.Println("- Logger: Your custom slog/zap/zerolog adapter")
}
