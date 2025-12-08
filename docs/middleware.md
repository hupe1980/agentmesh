---
layout: doc
title: Middleware
description: Extend and customize graph, model, and tool execution with composable middleware.
permalink: /middleware/
example: middleware
hero:
  title: Middleware Architecture
  description: Build resilient, observable workflows with composable middleware for caching, retries, circuit breakers, and more.
  primary_cta:
    label: View examples
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/middleware"
    external: true
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/model"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Architecture
    url: "#architecture"
  - title: Model Middleware
    url: "#model-middleware"
  - title: Tool Middleware
    url: "#tool-middleware"
  - title: Built-in Middleware
    url: "#built-in-middleware"
  - title: Custom Middleware
    url: "#custom-middleware"
---

## Overview {#overview}

AgentMesh provides a comprehensive middleware system for extending and customizing execution behavior across all layers: graph, model, and tool execution.

The middleware system enables you to:
- **Observe** execution with logging and events
- **Modify** behavior with caching, retries, and rate limiting
- **Protect** services with circuit breakers and timeouts
- **Monitor** usage with token counting and audit logs
- **Visualize** execution in real-time

## Architecture {#architecture}

```
┌─────────────────────────────────────────────────────┐
│                     Agent                            │
│  ┌───────────────────────────────────────────────┐  │
│  │         Graph Middleware Stack                 │  │
│  │  • Logging                                     │  │
│  │  • Events                                      │  │
│  │  • Visualization                               │  │
│  └─────────────────┬─────────────────────────────┘  │
│                    │                                 │
│         ┌──────────┴──────────┐                     │
│         │                     │                     │
│  ┌──────▼─────────┐   ┌──────▼─────────┐           │
│  │ Model Middleware│   │ Tool Middleware │           │
│  │  • Cache        │   │  • Cache       │           │
│  │  • Retry        │   │  • Timeout     │           │
│  │  • Rate Limit   │   │  • Circuit     │           │
│  │  • Token Count  │   │  • Audit       │           │
│  └────────────────┘   └────────────────┘           │
└─────────────────────────────────────────────────────┘
```

## Middleware Types

### Graph Middleware

Graph middleware wraps node execution, providing observability and lifecycle hooks. These functions are in the `graph` package.

**Available Middleware:**

- **LoggingMiddleware** - Structured logging of node execution
- **TimingMiddleware** - Tracks execution time with callbacks
- **RecoveryMiddleware** - Recovers from panics and converts them to errors
- **ConditionalMiddleware** - Applies middleware only when conditions are met
- **NodeMiddleware** - Applies middleware only to specific nodes
- **VizMiddleware** - Integrates with visualization server (in `pkg/viz/middleware`)

**Usage:**

```go
import (
    "log/slog"
    "github.com/hupe1980/agentmesh/pkg/graph"
)

// Apply logging middleware
b.WithMiddleware(graph.LoggingMiddleware(slog.Default()))

// Track timing
b.WithMiddleware(graph.TimingMiddleware(func(node string, d time.Duration) {
    metrics.RecordLatency(node, d)
}))

// Recover from panics
b.WithMiddleware(graph.RecoveryMiddleware(func(node string, r any) {
    log.Printf("panic in %s: %v", node, r)
}))

// Apply middleware to specific nodes only
b.WithMiddleware(graph.NodeMiddleware(
    []string{"slow_node", "external_api"},
    graph.LoggingMiddleware(slog.Default()),
))
```

### Model Middleware

Model middleware wraps LLM calls, enabling caching, retries, and usage tracking.

**Available Middleware:**

- **CacheMiddleware** - Caches responses to reduce API calls
- **RetryMiddleware** - Retries failed calls with exponential backoff
- **RateLimitMiddleware** - Prevents quota exhaustion
- **TokenCounterMiddleware** - Tracks token usage

**Usage:**

```go
import modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"

// Create middleware instances
cache := modelmw.NewCacheMiddleware()
retry := modelmw.NewRetryMiddleware(
    modelmw.WithMaxRetries(3),
    modelmw.WithInitialBackoff(100*time.Millisecond),
)
rateLimit := modelmw.NewRateLimitMiddleware(10, 100*time.Millisecond)
tokenCounter := modelmw.NewTokenCounterMiddleware()

// Apply to agent
agent.NewReAct(model,
    agent.WithModelMiddleware(cache, retry, rateLimit, tokenCounter),
)

// Access statistics
stats := tokenCounter.Stats()
fmt.Printf("Total tokens: %d\n", stats.TotalTokens)
fmt.Printf("API calls: %d\n", stats.CallCount)
```

### Tool Middleware

Tool middleware wraps tool executions, providing timeouts, circuit breakers, and audit logs.

**Available Middleware:**

- **CacheMiddleware** - Caches deterministic tool results
- **TimeoutMiddleware** - Enforces execution timeouts
- **CircuitBreakerMiddleware** - Implements circuit breaker pattern
- **AuditMiddleware** - Logs all tool executions

**Usage:**

```go
import toolmw "github.com/hupe1980/agentmesh/pkg/tool/middleware"

cache := toolmw.NewCacheMiddleware()
timeout := toolmw.NewTimeoutMiddleware(5*time.Second)
cb := toolmw.NewCircuitBreakerMiddleware(3, 30*time.Second)
audit := toolmw.NewAuditMiddleware(logger)

agent.NewReAct(model,
    agent.WithTools(tools...),
    agent.WithToolMiddleware(cache, timeout, cb, audit),
)

// Check circuit breaker state
fmt.Printf("Circuit state: %s\n", cb.State())
```

## Event Bus Integration

The middleware system integrates with a powerful event bus for loose coupling and observability.

### Subscribing to Events

```go
import "github.com/hupe1980/agentmesh/pkg/event"

// Create event bus
eventBus := event.NewBus()
ctx = event.WithBus(ctx, eventBus)

// Subscribe to all events
eventBus.Subscribe(event.HandlerFunc(func(ctx context.Context, evt event.Event) error {
    log.Printf("[%s] %s at node %s", evt.Type, evt.Timestamp, evt.Node)
    return nil
}))

// Subscribe to specific event types
eventBus.Subscribe(handler,
    event.EventNodeStart,
    event.EventNodeComplete,
    event.EventNodeError,
)
```

> **Throughput tip:** The bus snapshots handler lists under a short mutex and
> invokes handlers after releasing it. Slow handlers should spin up their own
> goroutines or buffering if they need to perform blocking I/O so publishers
> remain fast.

### Available Event Types

**Graph Events:**
- `EventGraphStart` - Graph execution started
- `EventGraphComplete` - Graph execution completed
- `EventGraphError` - Graph execution failed

**Node Events:**
- `EventNodeStart` - Node started executing
- `EventNodeComplete` - Node completed successfully
- `EventNodeError` - Node execution failed

**Model Events:**
- `EventModelStart` - Model call started
- `EventModelComplete` - Model call completed
- `EventModelError` - Model call failed

**Tool Events:**
- `EventToolStart` - Tool execution started
- `EventToolComplete` - Tool execution completed
- `EventToolError` - Tool execution failed

**Checkpoint Events:**
- `EventCheckpointSave` - Checkpoint saved
- `EventCheckpointLoad` - Checkpoint loaded
- `EventCheckpointError` - Checkpoint operation failed

**Interrupt Events:**
- `EventInterrupt` - Execution interrupted
- `EventResume` - Execution resumed

## Custom Middleware {#custom-middleware}

You can create custom middleware to extend execution behavior.

### Custom Graph Middleware

Graph middleware is a function of type `func(next NodeFunc) NodeFunc`:

```go
// Custom middleware function
func MyCustomMiddleware(logger *slog.Logger) graph.Middleware {
    return func(next graph.NodeFunc) graph.NodeFunc {
        return func(ctx context.Context, view graph.View) (*graph.Command, error) {
            nodeName := graph.NodeNameFromContext(ctx)
            
            // Pre-execution logic
            logger.Info("node starting", "node", nodeName)
            start := time.Now()
            
            // Execute the wrapped node
            cmd, err := next(ctx, view)
            
            // Post-execution logic
            duration := time.Since(start)
            if err != nil {
                logger.Error("node failed", "node", nodeName, "error", err, "duration", duration)
            } else {
                logger.Info("node completed", "node", nodeName, "duration", duration)
            }
            
            return cmd, err
        }
    }
}

// Usage
b.WithMiddleware(MyCustomMiddleware(slog.Default()))
```

### Custom Model Middleware

```go
type CustomModelMiddleware struct {
    // Your fields
}

func (m *CustomModelMiddleware) Wrap(next model.Executor) model.Executor {
    return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
        return func(yield func(*model.Response, error) bool) {
            // Pre-execution
            log.Println("Model call starting")
            
            // Execute and wrap responses
            for resp, err := range next.Generate(ctx, req) {
                // Process/modify response
                if resp != nil {
                    log.Printf("Received %d tokens", resp.Usage.TotalTokens)
                }
                if !yield(resp, err) {
                    return
                }
            }
        }
    })
}
```

### Custom Tool Middleware

```go
type CustomToolMiddleware struct {
    // Your fields
}

func (m *CustomToolMiddleware) Wrap(next tool.Executor) tool.Executor {
    return tool.WrapFunc(func(ctx context.Context, calls []tool.Call) ([]tool.ExecutionResult, error) {
        // Pre-execution
        log.Printf("Executing %d tools", len(calls))
        
        // Execute
        results, err := next.Execute(ctx, calls)
        
        // Post-execution
        if err == nil {
            for _, result := range results {
                if result.Error != nil {
                    log.Printf("Tool %s failed: %v", result.ToolName, result.Error)
                }
            }
        }
        
        return results, err
    })
}
```

## Middleware Composition

Middleware is composed using the `Chain` function, which applies middleware in reverse order:

```go
executor = graph.Chain(executor,
    middleware1, // Applied first (outermost)
    middleware2,
    middleware3, // Applied last (innermost)
)

// Execution flow:
// middleware1 → middleware2 → middleware3 → executor
```

When using agent options, middleware is automatically chained:

```go
agent.WithModelMiddleware(cache, retry, rateLimit)
// Results in: cache(retry(rateLimit(executor)))
```

## Best Practices

1. **Order Matters**: Place logging/observability middleware first, then caching, then retry logic
2. **Resource Cleanup**: Use `defer` for cleanup (e.g., `defer rateLimit.Close()`)
3. **Error Handling**: Middleware should be resilient - don't fail execution on observability errors
4. **State Management**: Use context for request-scoped state, use middleware fields for global state
5. **Performance**: Cache and rate limit middleware can significantly reduce costs
6. **Testing**: Test middleware independently before composing them

## Examples

See the following examples for complete implementations:

- **Basic Usage**: `examples/middleware/` - Demonstrates all middleware types
- **Observability**: `examples/observability/` - Event bus and monitoring
- **Visualization**: `examples/viz_ui_demo/` - Real-time visualization integration
- **Custom Middleware**: `examples/guardrails/` - Building custom middleware for content filtering
