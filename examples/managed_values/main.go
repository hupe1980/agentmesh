// Package main demonstrates managed values in AgentMesh.
//
// Managed values are ephemeral runtime state that is NOT included in checkpoints.
// They're ideal for:
//   - API keys and authentication tokens
//   - Session state (user context, preferences)
//   - Runtime metrics collectors
//   - Cached computed values
//   - Resource handles (connections, caches)
//
// This example shows two types of managed values:
//  1. StaticManagedValue - thread-safe storage for runtime config
//  2. ManagedValueProvider - computed values with optional TTL caching
package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
)

// Define a key for storing results
var resultKey = graph.NewKey("result", "")

// RuntimeConfig holds runtime configuration that shouldn't be checkpointed
type RuntimeConfig struct {
	APIKey     string
	Timeout    time.Duration
	MaxRetries int
}

// Define managed values at package level for type-safe access
var (
	runtimeConfigMV  *graph.StaticManagedValue[*RuntimeConfig]
	executionCountMV *graph.ManagedValueProvider[int64]
	cachedTimeMV     *graph.ManagedValueProvider[time.Time]
)

func main() {
	ctx := context.Background()

	// 1. Static managed value - runtime config
	runtimeConfigMV = graph.NewManagedValue("runtime_config", &RuntimeConfig{
		APIKey:     getEnvOrDefault("API_KEY", "sk_demo_key"),
		Timeout:    30 * time.Second,
		MaxRetries: 3,
	},
		graph.WithManagedValueRequired(),
		graph.WithManagedValueRehydrator(func(ctx context.Context) error {
			cfg, err := runtimeConfigMV.Get(ctx)
			if err != nil {
				return err
			}
			cfg.APIKey = getEnvOrDefault("API_KEY", cfg.APIKey)
			return nil
		}),
	)

	// 2. Provider without caching - always recomputes (e.g., execution counter)
	var executionCount int64
	executionCountMV = graph.NewManagedValueProvider("execution_count", func(ctx context.Context) (int64, error) {
		return atomic.AddInt64(&executionCount, 1), nil
	})

	// 3. Provider with caching - recomputes when TTL expires
	cachedTimeMV = graph.NewManagedValueProvider("cached_time", func(ctx context.Context) (time.Time, error) {
		return time.Now(), nil
	}, graph.WithCacheTTL(5*time.Second))

	// Create a simple graph
	g := graph.New[string, string](resultKey)

	// Process node - uses managed values via Scope (same pattern as regular state)
	g.Node("process", func(ctx context.Context, scope graph.Scope[string]) (*graph.Command, error) {
		// Access managed values via scope - same pattern as graph.Get(scope, key)
		config := graph.GetManaged(ctx, scope, runtimeConfigMV)
		execCount := graph.GetManaged(ctx, scope, executionCountMV)
		cachedTs := graph.GetManaged(ctx, scope, cachedTimeMV)

		result := fmt.Sprintf(
			"Execution #%d | API Key: %s... | Timeout: %s | Cached Time: %s",
			execCount,
			config.APIKey[:min(10, len(config.APIKey))],
			config.Timeout,
			cachedTs.Format(time.RFC3339),
		)

		fmt.Println("Node 'process' executed:")
		fmt.Println("  " + result)

		return graph.Set(resultKey, result).End()
	}, graph.END)

	// Set entry point
	g.Start("process")

	// Build the graph
	compiled, err := g.Build()
	if err != nil {
		fmt.Println("Error building graph:", err)
		os.Exit(1)
	}

	fmt.Println("=== Managed Values Demo ===")
	fmt.Println()
	fmt.Println("Managed values are ephemeral runtime state NOT included in checkpoints.")
	fmt.Println("They're perfect for API keys, session state, metrics, and cached values.")
	fmt.Println()

	// Run the graph with managed values - pass them directly, no registry needed!
	for i := 1; i <= 3; i++ {
		fmt.Printf("--- Run %d ---\n", i)

		for output, err := range compiled.Run(ctx, "test input",
			graph.WithManagedValues(runtimeConfigMV, executionCountMV, cachedTimeMV)) {
			if err != nil {
				fmt.Println("Error:", err)
				os.Exit(1)
			}
			fmt.Println("Output:", output)
		}

		fmt.Println()
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("=== Key Points ===")
	fmt.Println("1. Managed values are NOT persisted in checkpoints")
	fmt.Println("2. Access via graph.GetManaged(ctx, view, managedValue) - same pattern as state")
	fmt.Println("3. NewManagedValue(name, value) - static thread-safe storage")
	fmt.Println("4. NewManagedValueProvider(name, fn) - recomputed on every access")
	fmt.Println("5. NewManagedValueProvider(name, fn, WithCacheTTL(ttl)) - cached with TTL")
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
