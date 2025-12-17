// Package main demonstrates observability with the event bus.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/event"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

var counterKey = graph.NewKey[int]("counter")

func main() {
	ctx := context.Background()
	fmt.Println("=== Observability Example ===")

	// Create event bus for observability
	eventBus := event.NewBus()

	// Subscribe to node events
	eventBus.Subscribe(event.HandlerFunc(func(ctx context.Context, e event.Event) error {
		switch e.Type {
		case event.EventNodeStart:
			fmt.Printf("  [EVENT] Node started: %s (superstep %d)\n", e.Node, e.Superstep)
		case event.EventNodeComplete:
			fmt.Printf("  [EVENT] Node completed: %s (duration: %v)\n", e.Node, e.Duration)
		case event.EventNodeError:
			fmt.Printf("  [EVENT] Node error: %s - %s\n", e.Node, e.Error)
		}
		return nil
	}), event.EventNodeStart, event.EventNodeComplete, event.EventNodeError)

	// Build graph
	g := graph.New(counterKey)

	g.Node("step1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(counterKey, 1).To("step2")
	}, "step2")

	g.Node("step2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		return graph.Set(counterKey, counter+1).To("step3")
	}, "step3")

	g.Node("step3", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		fmt.Printf("  Final counter value: %d\n", counter)
		return graph.To(graph.END)
	}, graph.END)

	g.Start("step1")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nExecuting graph with event observability...")

	// Attach event bus to context
	ctx = event.WithBus(ctx, eventBus)

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("\n  Event bus enables:")
	fmt.Println("    • Monitoring node execution")
	fmt.Println("    • Tracking duration and errors")
	fmt.Println("    • Integration with external monitoring")
}
