package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func main() {
	// Create a circuit breaker:
	// - Opens after 3 consecutive failures
	// - Requires 2 successes in half-open state to close
	// - Waits 5 seconds before transitioning to half-open
	cb := graph.NewCircuitBreaker(3, 2, 5*time.Second)

	state := graph.NewGraphState(10)
	g := graph.NewGraph(state)

	// Simulate a flaky external service
	callCount := 0
	err := g.AddNode(&graph.Node{
		Name: "flaky-service",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			callCount++

			// Use circuit breaker to protect the service call
			err := cb.Call(ctx, func(ctx context.Context) error {
				// Simulate service behavior:
				// Calls 1-3: Fail (circuit opens)
				// Calls 4-5: Circuit is open, don't call service
				// Call 6+: Circuit half-open, then succeeds and closes
				if callCount <= 3 {
					return fmt.Errorf("service unavailable (call %d)", callCount)
				}
				return nil
			})

			if err != nil {
				log.Printf("[Call %d] Circuit State: %s, Error: %v", callCount, cb.State(), err)
				return nil, err
			}

			log.Printf("[Call %d] Circuit State: %s, Success!", callCount, cb.State())
			return &graph.NodeResult{
				Updates: map[string]any{
					"status": "success",
					"call":   callCount,
				},
			}, nil
		},
		RetryPolicy: &graph.RetryPolicy{
			MaxAttempts: 15,
			Backoff: func(attempt int) time.Duration {
				// When circuit is open, wait longer to allow it to transition to half-open
				if cb.State() == graph.StateOpen && attempt >= 4 {
					return 6 * time.Second // Wait for circuit to transition to half-open
				}
				return 500 * time.Millisecond
			},
			Retryable: func(err error) bool {
				// Always retry - let the circuit breaker handle failures
				return true
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	g.AddEdge(graph.StartNode, "flaky-service")
	g.AddEdge("flaky-service", graph.EndNode)

	compiled, err := g.Compile()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Circuit Breaker Pattern Example ===")
	fmt.Println()
	fmt.Println("Demonstrating circuit breaker protecting a flaky service:")
	fmt.Println("- First 3 calls fail → Circuit opens")
	fmt.Println("- While open, requests fail fast without calling service")
	fmt.Println("- After 5s timeout → Circuit transitions to half-open")
	fmt.Println("- Successful calls in half-open → Circuit closes")
	fmt.Println()

	_, err = compiled.Invoke(context.Background(), []message.Message{
		message.NewHumanMessageFromText("Test circuit breaker"),
	})

	fmt.Println("\n=== Results ===")
	if err != nil {
		fmt.Printf("Final error: %v\n", err)
		fmt.Printf("Circuit state: %s\n", cb.State())
	} else {
		fmt.Println("✓ Service recovered successfully!")
		fmt.Printf("Circuit state: %s\n", cb.State())
		fmt.Printf("Total calls made: %d\n", callCount)
		fmt.Printf("Status: %v\n", compiled.State().Get("status"))
	}
}
