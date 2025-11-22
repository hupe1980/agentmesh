# Retry Policy Builder Example

This example demonstrates the fluent builder API for creating retry policies in AgentMesh graphs.

## Features Demonstrated

1. **Basic Retry Configuration**: Simple policies with sensible defaults
2. **Backoff Strategies**: Exponential, linear, and constant backoff patterns
3. **Advanced Backoff**: Capped exponential to prevent unbounded wait times
4. **Selective Error Retry**: Only retry specific error types

## Running the Example

```bash
go run main.go
```

## Key Concepts

### Default Retry Policy

```go
policy := graph.NewRetryPolicy().Build()
// Creates: 3 attempts, exponential backoff (1s, 2s, 4s, ...)
```

### Exponential Backoff

```go
policy := graph.NewRetryPolicy().
    WithMaxAttempts(5).
    WithExponentialBackoff(time.Second, 2.0).
    Build()
// Wait times: 1s, 2s, 4s, 8s, 16s
```

### Linear Backoff

```go
policy := graph.NewRetryPolicy().
    WithLinearBackoff(500 * time.Millisecond).
    Build()
// Wait times: 500ms, 1s, 1.5s, 2s, ...
```

### Constant Backoff

```go
policy := graph.NewRetryPolicy().
    WithConstantBackoff(time.Second).
    Build()
// Wait times: 1s, 1s, 1s, ...
```

### Selective Error Retry

```go
policy := graph.NewRetryPolicy().
    WithRetryableErrors(ErrTransient, ErrTimeout).
    Build()
// Only retries if error matches ErrTransient or ErrTimeout
```

### Capped Exponential (Prevents Unbounded Growth)

```go
policy := &graph.RetryPolicy{
    MaxAttempts: 10,
    Backoff: graph.CappedExponentialBackoff(time.Second, 2.0, 30*time.Second),
}
// Wait times: 1s, 2s, 4s, 8s, 16s, 30s, 30s, 30s, ...
// Never exceeds 30 seconds
```

### Jittered Exponential (Prevents Thundering Herd)

```go
policy := &graph.RetryPolicy{
    MaxAttempts: 5,
    Backoff: graph.JitteredExponentialBackoff(time.Second, 2.0, 0.1),
}
// Wait times have ±10% random jitter to prevent synchronized retries
```

## Using in a Graph

```go
// Add node with retry policy
g.AddNodeFuncWithRetry("api_call", apiCallFunc, graph.NewRetryPolicy().
    WithMaxAttempts(5).
    WithExponentialBackoff(100*time.Millisecond, 2.0).
    WithRetryableErrors(ErrTransient, ErrTimeout).
        Build(),
})
```

## Available Builder Methods

### Configuration
- `NewRetryPolicy()` - Start with defaults (3 attempts, exponential backoff)
- `WithMaxAttempts(n)` - Set max retry attempts
- `WithNoRetries()` - Disable retries completely

### Backoff Strategies
- `WithExponentialBackoff(base, multiplier)` - Exponential growth
- `WithLinearBackoff(base)` - Linear increment
- `WithConstantBackoff(duration)` - Fixed wait time
- `WithCustomBackoff(fn)` - Provide custom function

### Error Matching
- `WithRetryableErrors(errs...)` - Only retry these specific errors
- `WithNonRetryableErrors(errs...)` - Retry all except these
- `WithRetryableFunc(fn)` - Custom retry logic

## See Also

- [Graph Documentation](../../pkg/graph/doc.go)
- [Retry Tests](../../pkg/graph/retry_test.go) - Complete integration examples
- [Retry Builder Tests](../../pkg/graph/retry_builder_test.go) - All builder features
