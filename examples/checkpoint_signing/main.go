// Package main demonstrates checkpoint signing for tamper detection.
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

var counterKey = graph.NewKey("counter", 0)

func main() {
	ctx := context.Background()
	fmt.Println("=== Checkpoint Signing Example ===")

	// Generate a secure signing key
	signingKey := make([]byte, 32)
	if _, err := rand.Read(signingKey); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  Generated 256-bit signing key")

	// Create checkpointer with signing enabled
	cp := checkpoint.NewInMemoryCheckpointer(checkpoint.WithSigning(signingKey))
	fmt.Println("  Created checkpointer with signing")

	runID := "signed-run-1"

	// Build graph
	g := graph.New[any, any](counterKey)

	g.Node("step1", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		fmt.Println("  [step1] Processing")
		return graph.Set(counterKey, 1).To("step2")
	}, "step2")

	g.Node("step2", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		fmt.Printf("  [step2] Counter: %d\n", counter)
		return graph.Set(counterKey, counter+1).To("step3")
	}, "step3")

	g.Node("step3", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		fmt.Printf("  [step3] Final counter: %d\n", counter)
		return graph.To(graph.END)
	}, graph.END)

	g.Start("step1")

	// Set checkpointer using builder method
	g.WithCheckpointer(cp, runID)

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	// Run the graph
	fmt.Println("\n--- Executing with signed checkpoints ---")
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	// Verify checkpoint can be loaded (signature verified automatically)
	fmt.Println("\n--- Loading and verifying checkpoint ---")
	loadedCP, err := cp.Load(ctx, runID)
	if err != nil {
		log.Fatal("Signature verification failed:", err)
	}
	fmt.Printf("  Loaded checkpoint: superstep=%d, signature verified ✓\n", loadedCP.Superstep)

	fmt.Println("\n  Checkpoint signing provides:")
	fmt.Println("    • Tamper detection")
	fmt.Println("    • Data integrity verification")
	fmt.Println("    • Protection against unauthorized modifications")
}
