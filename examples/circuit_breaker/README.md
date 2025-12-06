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

### Plugin Configuration
```go
import (
    "github.com/hupe1980/agentmesh/pkg/callbacks"
    "github.com/hupe1980/agentmesh/pkg/callbacks/plugins"
)

// Create circuit breaker plugin
cb := plugin.NewCircuitBreakerPlugin(
    3,              // maxFailures before opening
    5*time.Second,  // resetTimeout before half-open
    1,              // halfOpenLimit (requests in half-open)
)

// Register with plugin manager
pm := callbacks.NewPluginManager()
pm.Register(cb)
```

### Integration
```go
// Callbacks are automatically injected via context
reactAgent, _ := agent.NewReAct(
    model,
    agent.WithTools(tools...),
    agent.WithPluginManager(pm),
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
- [pkg/callbacks/plugins](../../pkg/callbacks/plugins) - Built-in plugins
- [examples/guardrails](../guardrails) - Security plugins
- [examples/callback_integration](../callback_integration) - Plugin composition
