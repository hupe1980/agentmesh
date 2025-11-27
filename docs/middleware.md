# Middleware Architecture

AgentMesh provides a comprehensive middleware system for extending and customizing execution behavior across all layers: graph, model, and tool execution.

## Overview

The middleware system enables you to:
- **Observe** execution with logging and events
- **Modify** behavior with caching, retries, and rate limiting
- **Protect** services with circuit breakers and timeouts
- **Monitor** usage with token counting and audit logs
- **Visualize** execution in real-time

## Architecture

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

Graph middleware wraps the entire graph execution, providing observability and lifecycle hooks.

**Available Middleware:**

- **LoggingMiddleware** - Structured logging of graph execution
- **EventMiddleware** - Publishes events to event bus
- **VizMiddleware** - Integrates with visualization server

**Usage:**

```go
import graphmw "github.com/hupe1980/agentmesh/pkg/graph/middleware"

agent.NewReActAgent(model,
    agent.WithGraphMiddleware(
        graphmw.NewLoggingMiddleware[[]message.Message, message.Message](logger),
        graphmw.NewEventMiddleware[[]message.Message, message.Message](),
    ),
)
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
agent.NewReActAgent(model,
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

agent.NewReActAgent(model,
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
// Create event bus
eventBus := graph.NewEventBus()
ctx = graph.WithEventBus(ctx, eventBus)

// Subscribe to all events
eventBus.Subscribe(graph.EventHandlerFunc(func(ctx context.Context, event graph.Event) error {
    log.Printf("[%s] %s at node %s", event.Type, event.Timestamp, event.Node)
    return nil
}))

// Subscribe to specific event types
eventBus.Subscribe(handler, 
    graph.EventNodeStart,
    graph.EventNodeComplete,
    graph.EventNodeError,
)
```

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

## Custom Middleware

You can create custom middleware by implementing the `Middleware` interface:

### Custom Graph Middleware

```go
type CustomMiddleware[I, O any] struct {
    // Your fields
}

func (m *CustomMiddleware[I, O]) Wrap(next graph.Executor[I, O]) graph.Executor[I, O] {
    return graph.WrapFunc(func(ctx context.Context, compiled *graph.Compiled[I, O], input I, opts ...graph.RunOption) iter.Seq2[O, error] {
        // Pre-execution logic
        log.Println("Before execution")
        
        // Execute
        results := next.Run(ctx, compiled, input, opts...)
        
        // Post-execution logic (wrap iterator)
        return func(yield func(O, error) bool) {
            for result, err := range results {
                if err != nil {
                    log.Printf("Error: %v", err)
                }
                if !yield(result, err) {
                    return
                }
            }
            log.Println("After execution")
        }
    })
}
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
- **Visualization**: `examples/viz_server/` - Real-time visualization integration
- **Custom Middleware**: `examples/guardrails/` - Building custom middleware for content filtering
