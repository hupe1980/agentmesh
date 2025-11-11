# Input Validation Example

This example demonstrates AgentMesh's built-in input validation features for securing agent workflows.

## What It Does

Shows how to use the three input validation options to protect against:
- **DoS attacks** via extremely large messages
- **Resource exhaustion** from too many messages  
- **Excessive API costs** from bulk requests to LLM providers

## Security Features Demonstrated

### 1. Message Size Limit
```go
graph.WithMaxMessageSize(1_000_000) // 1MB per message
```
Prevents individual messages from exceeding a size limit.

### 2. Message Count Limit
```go
graph.WithMaxInputMessages(100) // Max 100 messages
```
Limits the total number of input messages accepted.

### 3. Total Size Limit
```go
graph.WithMaxTotalSize(10_000_000) // 10MB total
```
Ensures the combined size of all messages doesn't exceed a threshold.

## Running the Example

```bash
cd examples/input_validation
go run main.go
```

## Example Output

```
Test 1: Normal message within limits
✅ Succeeded

Test 2: Single message exceeds size limit
✅ Correctly blocked: message at index 0 exceeds size limit: 2000000 bytes > 1000000 bytes limit

Test 3: Too many messages
✅ Correctly blocked: too many messages: 150 messages > 100 limit

Test 4: Total size of all messages exceeds limit
✅ Correctly blocked: total message size exceeds limit: 1200000 bytes > 1000000 bytes limit

Test 5: Production-recommended configuration
✅ Production limits work correctly
```

## Production Recommendations

For production deployments, always set input validation limits:

```go
compiled.Run(ctx, messages,
    graph.WithMaxMessageSize(1_000_000),   // 1MB per message
    graph.WithMaxInputMessages(100),       // Max 100 messages
    graph.WithMaxTotalSize(10_000_000),    // 10MB total
)
```

### Recommended Limits by Use Case

**Public APIs:**
- `WithMaxMessageSize(500_000)` - 500KB per message
- `WithMaxInputMessages(10)` - Max 10 messages
- `WithMaxTotalSize(5_000_000)` - 5MB total

**Internal Tools:**
- `WithMaxMessageSize(2_000_000)` - 2MB per message
- `WithMaxInputMessages(100)` - Max 100 messages
- `WithMaxTotalSize(20_000_000)` - 20MB total

**Long-Running Workflows:**
- `WithMaxMessageSize(1_000_000)` - 1MB per message
- `WithMaxInputMessages(1000)` - Max 1000 messages
- `WithMaxTotalSize(50_000_000)` - 50MB total

## Error Handling

Validation errors are returned immediately before graph execution:

```go
_, err := graph.Last(compiled.Run(ctx, messages, 
    graph.WithMaxMessageSize(100),
))

if err != nil {
    var validationErr *graph.MessageValidationError
    if errors.As(err, &validationErr) {
        // Handle validation failure
        fmt.Printf("Validation failed: %s\n", validationErr.Type)
    }
}
```

## See Also

- [Security Best Practices](../../SECURITY.md)
- [Guardrails Example](../guardrails/) - Content filtering and PII redaction
- [Circuit Breaker Example](../circuit_breaker/) - Rate limiting and error handling
