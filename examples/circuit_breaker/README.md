# Example: Circuit Breaker

## Overview
Demonstrates circuit breaker pattern for preventing cascading failures when calling unreliable external services. Uses the built-in `CircuitBreakerPlugin` for resilient model invocations.

## Key Concepts
- **Circuit Breaker Plugin**: Built-in plugin for resilience patterns
- **Three States**: Closed (normal) → Open (failing) → Half-Open (testing)
- **Automatic Recovery**: Reset after timeout in half-open state
- **Failure Tracking**: Counts consecutive failures to trip circuit

## Running
```bash
cd examples/circuit_breaker
go run main.go
```

## Expected Output
```
=== Circuit Breaker Pattern Demo ===

Plugin manager configured with CircuitBreakerPlugin
- Max Failures: 3
- Reset Timeout: 5s
- Half-Open Limit: 1

Simulating flaky model...

Call #1: Success ✓
Call #2: Failed (1/3 failures) ✗
Call #3: Failed (2/3 failures) ✗
Call #4: Failed (3/3 failures) ✗
⚠️  Circuit opened!

Call #5: Blocked by circuit breaker 🛑
Call #6: Blocked by circuit breaker 🛑

⏳ Waiting for reset timeout...

Call #7: Half-open - Testing...
Call #7: Success! Circuit closed ✓
```

## Circuit Breaker States

### Closed (Normal Operation)
- All requests pass through
- Tracks consecutive failures
- Opens when threshold exceeded

### Open (Fast Fail)
- All requests immediately rejected
- No load on failing service
- Waits for reset timeout

### Half-Open (Recovery Test)
- Limited requests allowed
- Tests service recovery
- Closes on success, reopens on failure

## Implementation

### Middleware Configuration
```go
import (
    "github.com/hupe1980/agentmesh/pkg/tool"
    toolmw "github.com/hupe1980/agentmesh/pkg/tool/middleware"
)

// Create circuit breaker middleware:
// - Opens after 3 failures
// - Waits 30 seconds before transitioning to half-open
cb := toolmw.NewCircuitBreakerMiddleware(3, 30*time.Second)

// Create tool executor with circuit breaker
registry := map[string]tool.Tool{"my_tool": myTool}
baseExecutor := tool.NewSequentialExecutor(registry)
executor := tool.Chain(baseExecutor, cb)
```

### Integration with Agent
```go
// Create ReAct agent with circuit breaker middleware
reactAgent, _ := agent.NewReAct(
    model,
    agent.WithTools(tools...),
    agent.WithToolMiddleware(cb),
)
```

### Monitoring
```go
// Check circuit state
state := cb.GetState()  // "closed", "open", "half-open"

// Reset circuit manually
cb.Reset()
```

## Use Cases
- **Microservice Resilience**: Protect against cascading failures
- **API Rate Limiting**: Fast fail when quota exceeded
- **Database Overload**: Prevent connection pool exhaustion
- **External Service Failures**: Graceful degradation

## Related Resources
- [Middleware Documentation](/middleware/) - Middleware system guide
- [examples/guardrails](../guardrails) - Security middleware
- [examples/custom_observability](../custom_observability) - Event handling and observability
