package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"
	toolmw "github.com/hupe1980/agentmesh/pkg/tool/middleware"
)

// SimpleLogger demonstrates logging integration
type SimpleLogger struct{}

func (l *SimpleLogger) Debug(msg string, args ...any) {
	fmt.Printf("[DEBUG] %s %v\n", msg, args)
}

func (l *SimpleLogger) Info(msg string, args ...any) {
	fmt.Printf("[INFO] %s %v\n", msg, args)
}

func (l *SimpleLogger) Warn(msg string, args ...any) {
	fmt.Printf("[WARN] %s %v\n", msg, args)
}

func (l *SimpleLogger) Error(msg string, args ...any) {
	fmt.Printf("[ERROR] %s %v\n", msg, args)
}

func (l *SimpleLogger) With(args ...any) logging.Logger {
	return l // Simple implementation returns self
}

func main() {
	ctx := context.Background()
	logger := &SimpleLogger{}

	fmt.Println("=== Middleware System Demonstration ===")

	// 1. Model Middleware
	fmt.Println("1. Model Middleware (for LLM calls):")
	fmt.Println("   • Cache      - Reduces redundant API calls")
	fmt.Println("   • Retry      - Handles transient failures with backoff")
	fmt.Println("   • RateLimit  - Prevents quota exhaustion")
	fmt.Println("   • TokenCount - Tracks usage for cost monitoring")

	modelCache := modelmw.NewCacheMiddleware()
	_ = modelmw.NewRetryMiddleware(modelmw.WithMaxRetries(3)) // Example initialization
	modelRateLimit := modelmw.NewRateLimitMiddleware(10, 100*time.Millisecond)
	tokenCounter := modelmw.NewTokenCounterMiddleware()
	defer modelRateLimit.Close()

	fmt.Printf("   ✓ Initialized: cache=%d entries, rate_limit=%d tokens available\n\n",
		modelCache.Size(), modelRateLimit.Available())

	// 2. Tool Middleware
	fmt.Println("2. Tool Middleware (for tool executions):")
	fmt.Println("   • Cache          - Caches deterministic tool results")
	fmt.Println("   • Timeout        - Prevents hung executions")
	fmt.Println("   • CircuitBreaker - Stops cascading failures")
	fmt.Println("   • Audit          - Logs all executions for compliance")

	toolCache := toolmw.NewCacheMiddleware()
	_ = toolmw.NewTimeoutMiddleware(5 * time.Second) // Example initialization
	circuitBreaker := toolmw.NewCircuitBreakerMiddleware(3, 30*time.Second)
	_ = toolmw.NewAuditMiddleware(logger) // Example initialization

	fmt.Printf("   ✓ Initialized: cache=%d entries, circuit_breaker=%s\n\n",
		toolCache.Size(), circuitBreaker.State())

	// 3. Graph Middleware
	fmt.Println("3. Graph Middleware (for execution orchestration):")
	fmt.Println("   • Logging - Structured execution logging")
	fmt.Println("   • Events  - Publishes to event bus for observability")

	// Note: graphLogging is generic and requires type parameters matching your graph
	// For a message-based graph: graphmw.NewLoggingMiddleware[[]message.Message, message.Message](logger)
	fmt.Println("   ✓ Logging and event middleware available")
	fmt.Println()

	// 4. Event Bus Integration
	fmt.Println("4. Event Bus System:")
	eventBus := graph.NewEventBus()
	ctx = graph.WithEventBus(ctx, eventBus)

	eventCount := 0
	eventBus.Subscribe(graph.EventHandlerFunc(func(ctx context.Context, event graph.Event) error {
		eventCount++
		fmt.Printf("   [%s] %s at %s\n", event.Type, event.Node, event.Timestamp.Format("15:04:05"))
		return nil
	}))

	fmt.Println("   ✓ Event handler subscribed")
	fmt.Println()

	// 5. Usage Example
	fmt.Println("5. Integration with Agent:")
	fmt.Println("```go")
	fmt.Println("agent.NewReActAgent(model,")
	fmt.Println("    agent.WithGraphMiddleware(")
	fmt.Println("        graphmw.NewLoggingMiddleware[[]message.Message, message.Message](logger),")
	fmt.Println("        graphmw.NewEventMiddleware[[]message.Message, message.Message](),")
	fmt.Println("    ),")
	fmt.Println("    agent.WithModelMiddleware(")
	fmt.Println("        modelmw.NewCacheMiddleware(),")
	fmt.Println("        modelmw.NewRetryMiddleware(),")
	fmt.Println("        modelmw.NewRateLimitMiddleware(10, 100*time.Millisecond),")
	fmt.Println("        modelmw.NewTokenCounterMiddleware(),")
	fmt.Println("    ),")
	fmt.Println("    agent.WithToolMiddleware(")
	fmt.Println("        toolmw.NewCacheMiddleware(),")
	fmt.Println("        toolmw.NewTimeoutMiddleware(5*time.Second),")
	fmt.Println("        toolmw.NewCircuitBreakerMiddleware(3, 30*time.Second),")
	fmt.Println("        toolmw.NewAuditMiddleware(logger),")
	fmt.Println("    ),")
	fmt.Println(")")
	fmt.Println("```")
	fmt.Println()

	// 6. Statistics
	fmt.Println("6. Middleware Statistics:")
	stats := tokenCounter.Stats()
	fmt.Printf("   Token Usage:    %d calls, %d total tokens\n", stats.CallCount, stats.TotalTokens)
	fmt.Printf("   Model Cache:    %d entries\n", modelCache.Size())
	fmt.Printf("   Tool Cache:     %d entries\n", toolCache.Size())
	fmt.Printf("   Rate Limit:     %d tokens available\n", modelRateLimit.Available())
	fmt.Printf("   Circuit State:  %s\n", circuitBreaker.State())
	fmt.Printf("   Events Fired:   %d events\n\n", eventCount)

	fmt.Println("=== Demonstration Complete ===")
	fmt.Println("\nFor a complete working example with actual agent execution,")
	fmt.Println("see: examples/basic_agent/ or examples/observability/")
}
