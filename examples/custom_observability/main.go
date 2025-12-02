// Package main demonstrates custom observability with metrics collection.
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

var taskKey = graph.NewKey("task", "")

// MetricsCollector tracks execution metrics
type MetricsCollector struct {
	mu             sync.Mutex
	nodeExecutions map[string]int
	nodeDurations  map[string]time.Duration
	errors         []string
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		nodeExecutions: make(map[string]int),
		nodeDurations:  make(map[string]time.Duration),
	}
}

// HandleEvent implements event.EventHandler
func (m *MetricsCollector) HandleEvent(ctx context.Context, e event.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch e.Type {
	case event.EventNodeStart:
		m.nodeExecutions[e.Node]++
	case event.EventNodeComplete:
		m.nodeDurations[e.Node] += e.Duration
	case event.EventNodeError:
		m.errors = append(m.errors, fmt.Sprintf("%s: %s", e.Node, e.Error))
	}
	return nil
}

func (m *MetricsCollector) Report() {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Println("\n  Metrics Report:")
	fmt.Println("    Node Executions:")
	for node, count := range m.nodeExecutions {
		fmt.Printf("      %s: %d executions\n", node, count)
	}
	fmt.Println("    Node Durations:")
	for node, dur := range m.nodeDurations {
		fmt.Printf("      %s: %v\n", node, dur)
	}
	if len(m.errors) > 0 {
		fmt.Println("    Errors:")
		for _, err := range m.errors {
			fmt.Printf("      - %s\n", err)
		}
	}
}

func main() {
	ctx := context.Background()
	fmt.Println("=== Custom Observability Example ===")

	// Create event bus and metrics collector
	eventBus := event.NewBus()
	metrics := NewMetricsCollector()

	// Subscribe metrics collector to all events
	eventBus.Subscribe(metrics)

	// Also add a simple logger for node events
	eventBus.Subscribe(event.HandlerFunc(func(ctx context.Context, e event.Event) error {
		if e.Type == event.EventNodeComplete {
			fmt.Printf("  [LOG] Node %s completed in %v\n", e.Node, e.Duration)
		}
		return nil
	}), event.EventNodeComplete)

	// Build graph
	g := graph.New[any, any](taskKey)

	g.Node("fetch", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		time.Sleep(10 * time.Millisecond) // Simulate work
		return graph.Set(taskKey, "fetched_data").To("process")
	}, "process")

	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		time.Sleep(20 * time.Millisecond) // Simulate work
		return graph.Set(taskKey, "processed_data").To("store")
	}, "store")

	g.Node("store", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		time.Sleep(5 * time.Millisecond) // Simulate work
		return graph.To(graph.END)
	}, graph.END)

	g.Start("fetch")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nExecuting graph with custom metrics...")

	// Attach event bus to context
	ctx = event.WithBus(ctx, eventBus)

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	// Print metrics report
	metrics.Report()

	fmt.Println("\n  Custom observability enables:")
	fmt.Println("    • Aggregated metrics collection")
	fmt.Println("    • Performance monitoring")
	fmt.Println("    • Error tracking")
	fmt.Println("    • Custom dashboards/alerts")
}
