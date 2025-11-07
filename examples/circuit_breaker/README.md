# Example: Circuit Breaker

## Overview
Demonstrates production-grade fault tolerance using the circuit breaker policy pattern. Shows how to protect your system from cascading failures when calling unreliable external services.

## Key Concepts
- **Circuit Breaker Pattern**: Prevents system overload by failing fast when services are down
- **State Transitions**: Closed → Open → Half-Open → Closed lifecycle
- **Callback Policies**: Composable resilience patterns via callbacks
- **Failure Threshold**: Automatic circuit opening after consecutive failures
- **Recovery Testing**: Half-open state validates service recovery

## Running
```bash
cd examples/circuit_breaker
go run main.go
```

## Expected Output
```
=== Circuit Breaker Example ===

Simulating unreliable service with circuit breaker protection

[Call 1] ❌ Service failing
Circuit Breaker: Failure recorded (1/3)

[Call 2] ❌ Service failing
Circuit Breaker: Failure recorded (2/3)

[Call 3] ❌ Service failing
⚠️  Circuit Breaker OPENED (threshold reached)

[Call 4] Circuit is OPEN - Failing fast
❌ Error: circuit breaker is open

[Call 5] Circuit is OPEN - Failing fast
❌ Error: circuit breaker is open

[Half-Open State] Testing service recovery...
[Call 6] ✓ Service success
Circuit Breaker: Success in half-open state → CLOSED

[Call 7] ✓ Service success
Circuit state: CLOSED - Operating normally
```

## Code Walkthrough

### 1. Configure Circuit Breaker
```go
cbConfig := policies.CircuitBreakerConfig{
    FailureThreshold: 3,              // Open after 3 failures
    RecoveryTimeout:  2 * time.Second, // Try recovery after 2s
    HalfOpenMaxCalls: 1,              // Test with 1 call
}
```

### 2. Create Circuit Breaker Callbacks
```go
before, after, onError := policies.CircuitBreaker(cbConfig)
```

### 3. Register with Callback Manager
```go
manager := callbacks.NewManager()
manager.RegisterBeforeModel(before)
manager.RegisterAfterModel(after)
manager.RegisterOnModelError(onError)
```

### 4. Integrate with Graph Nodes
```go
modelNode := agent.ModelNode(flakyModel, 
    agent.WithModelCallbacks(manager),
)
```

## Circuit Breaker States

### Closed (Normal Operation)
- All requests pass through
- Failures are counted
- Opens when threshold reached

### Open (Failing Fast)
- All requests rejected immediately
- No calls to failing service
- Prevents resource exhaustion
- Transitions to Half-Open after timeout

### Half-Open (Testing Recovery)
- Limited requests allowed through
- Tests if service recovered
- Success → Closed (service healthy)
- Failure → Open (still broken)

## What This Example Teaches
- ✅ Circuit breaker pattern implementation
- ✅ Protection against cascading failures
- ✅ Automatic failure detection and recovery
- ✅ Callback-based policy composition
- ✅ State machine lifecycle management

## Production Considerations

### Tuning Parameters
```go
CircuitBreakerConfig{
    FailureThreshold: 5,        // Tolerate more failures
    RecoveryTimeout: 30s,       // Wait longer before retry
    HalfOpenMaxCalls: 3,        // More conservative testing
}
```

### Per-Node Circuit Breakers
```go
// Independent circuit breakers for each service
cb1, _, _ := policies.PerNodeCircuitBreaker(config)
cb2, _, _ := policies.PerNodeCircuitBreaker(config)

manager.RegisterBeforeModel(cb1) // Service A
manager.RegisterBeforeModel(cb2) // Service B
```

### Combining with Retry
```go
// Layer 1: Circuit breaker (fail fast)
manager.RegisterBeforeModel(cbBefore)
manager.RegisterAfterModel(cbAfter)
manager.RegisterOnModelError(cbOnError)

// Layer 2: Retry (for transient failures)
manager.RegisterOnModelError(policies.ExponentialBackoffRetry(retryConfig))
```

## Next Steps
- Combine with rate limiting for comprehensive protection
- Add metrics to track circuit breaker state changes
- Implement custom recovery logic in half-open state
- See **examples/callback_integration** for policy composition
- See **examples/observability** for monitoring circuit state

## See Also
- [pkg/callbacks/policies](../../pkg/callbacks/policies) - Policy implementations
- [examples/guardrails](../guardrails) - Input validation and output filtering
- [examples/observability](../observability) - Metrics and tracing
