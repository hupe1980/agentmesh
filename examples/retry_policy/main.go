// Package main demonstrates retry policies for resilient node execution.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

var counterKey = graph.NewKey[int]("counter")

func main() {
	ctx := context.Background()
	fmt.Println("=== Retry Policy Example ===")

	// Track attempt count for demonstration
	var attempts atomic.Int32

	// Create a flaky node that fails twice then succeeds
	flakyNode := func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		attempt := attempts.Add(1)
		fmt.Printf("  [flaky] Attempt %d\n", attempt)

		if attempt <= 2 {
			return graph.Fail(errors.New("temporary failure"))
		}
		return graph.Set(counterKey, int(attempt)).End()
	}

	// Build graph with retry policy
	g := graph.New[any, any](counterKey)

	// Create retry policy with exponential backoff
	policy := graph.NewRetryPolicyBuilder().
		WithMaxAttempts(5).
		WithExponentialBackoff(50*time.Millisecond, 2.0).
		WithMaxDelay(500 * time.Millisecond).
		Build()

	// Wrap node with retry policy
	g.Node("flaky_operation", graph.WithRetry(flakyNode, policy), graph.END)

	g.Start("flaky_operation")

	compiled, err := g.Build()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n--- Executing with retry policy ---")
	start := time.Now()

	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("\n  Total attempts: %d\n", attempts.Load())
	fmt.Printf("  Elapsed time: %v\n", elapsed)

	// Demonstrate different retry strategies
	fmt.Println("\n--- Retry Policy Types ---")

	// 1. Constant backoff
	constantPolicy := graph.NewRetryPolicyBuilder().
		WithMaxAttempts(3).
		WithConstantBackoff(100 * time.Millisecond).
		Build()
	fmt.Printf("  Constant: %d attempts, delay=%v\n",
		constantPolicy.MaxAttempts, constantPolicy.Delay)

	// 2. Linear backoff
	linearPolicy := graph.NewRetryPolicyBuilder().
		WithMaxAttempts(5).
		WithLinearBackoff(50 * time.Millisecond).
		Build()
	fmt.Printf("  Linear: %d attempts, delay=%v\n",
		linearPolicy.MaxAttempts, linearPolicy.Delay)

	// 3. Exponential backoff
	exponentialPolicy := graph.NewRetryPolicyBuilder().
		WithMaxAttempts(10).
		WithExponentialBackoff(100*time.Millisecond, 2.0).
		WithMaxDelay(5 * time.Second).
		Build()
	fmt.Printf("  Exponential: %d attempts, delay=%v, max=%v\n",
		exponentialPolicy.MaxAttempts, exponentialPolicy.Delay, exponentialPolicy.MaxDelay)

	// 4. Custom retryable function
	customPolicy := graph.NewRetryPolicyBuilder().
		WithMaxAttempts(3).
		WithExponentialBackoff(100*time.Millisecond, 2.0).
		WithRetryableFunc(func(err error) bool {
			// Only retry specific errors
			return errors.Is(err, context.DeadlineExceeded)
		}).
		Build()
	fmt.Printf("  Custom: %d attempts with custom error filter\n", customPolicy.MaxAttempts)

	fmt.Println("\n  Retry policies provide:")
	fmt.Println("    • Automatic failure recovery")
	fmt.Println("    • Configurable backoff strategies")
	fmt.Println("    • Maximum attempt limits")
	fmt.Println("    • Custom error filtering")
}
