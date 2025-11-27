package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hupe1980/agentmesh/pkg/tool"
	toolmw "github.com/hupe1980/agentmesh/pkg/tool/middleware"
)

// FlakyTool simulates an unreliable external service
type FlakyTool struct {
	callCount int
}

func (t *FlakyTool) Name() string {
	return "flaky_api"
}

func (t *FlakyTool) Description() string {
	return "Simulates an unreliable external API for circuit breaker demonstration"
}

func (t *FlakyTool) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  map[string]any{},
		},
	}
}

func (t *FlakyTool) Call(ctx context.Context, args string) (any, error) {
	t.callCount++

	// Simulate service behavior:
	// Calls 1-3: Fail (circuit opens after 3)
	// Calls 4+: Success (circuit recovers)
	if t.callCount <= 3 {
		log.Printf("[Call %d] ❌ Service failing", t.callCount)
		return nil, fmt.Errorf("service unavailable (call %d)", t.callCount)
	}

	log.Printf("[Call %d] ✓ Service success", t.callCount)
	return fmt.Sprintf("Success on call %d with args: %s", t.callCount, args), nil
}

func main() {
	fmt.Println("=== Circuit Breaker Middleware Example ===")
	fmt.Println()
	fmt.Println("Demonstrating tool circuit breaker:")
	fmt.Println("- First 3 failures → Circuit opens")
	fmt.Println("- While open, middleware rejects requests immediately")
	fmt.Println("- After 30s timeout → Circuit transitions to half-open")
	fmt.Println("- Successful call → Circuit closes")
	fmt.Println()

	// Create flaky tool
	flakyTool := &FlakyTool{}

	// Create circuit breaker middleware:
	// - Opens after 3 failures
	// - Waits 30 seconds before transitioning to half-open
	cb := toolmw.NewCircuitBreakerMiddleware(3, 30*time.Second)

	// Create tool executor with circuit breaker
	registry := map[string]tool.Tool{
		"flaky_api": flakyTool,
	}
	baseExecutor := tool.NewSequentialExecutor(registry)
	executor := tool.Chain(baseExecutor, cb)

	ctx := context.Background()

	// Make multiple attempts to demonstrate circuit breaker behavior
	for i := 1; i <= 10; i++ {
		fmt.Printf("\n--- Attempt %d (Circuit: %s) ---\n", i, cb.State())

		calls := []tool.Call{
			{
				ID:        fmt.Sprintf("call-%d", i),
				Name:      "flaky_api",
				Arguments: fmt.Sprintf(`{"test":"attempt %d"}`, i),
			},
		}

		results, err := executor.Execute(ctx, calls)

		if err != nil {
			fmt.Printf("❌ Execution failed: %v\n", err)
			time.Sleep(time.Second)
			continue
		}

		result := results[0]
		if result.Error != nil {
			fmt.Printf("❌ Tool failed: %v\n", result.Error)
		} else {
			fmt.Printf("✓ Success: %s\n", result.Result)
			break
		}

		// Wait before next attempt
		// (In real scenario, wait for resetTimeout to test half-open state)
		time.Sleep(time.Second)
	}

	fmt.Println("\n=== Results ===")
	fmt.Printf("Circuit breaker state: %s\n", cb.State())
	fmt.Printf("Total API calls made: %d (note: calls blocked by open circuit don't increment)\n", flakyTool.callCount)

	// Demonstrate state transitions
	fmt.Println("\n=== Circuit States ===")
	fmt.Println("1. CLOSED   - Normal operation, all requests pass through")
	fmt.Println("2. OPEN     - Fast fail, requests rejected immediately")
	fmt.Println("3. HALF_OPEN- Testing recovery, limited requests allowed")
	fmt.Println()
	fmt.Println("The circuit protects downstream services from being overwhelmed")
	fmt.Println("and provides fast failure when services are unavailable.")
}
