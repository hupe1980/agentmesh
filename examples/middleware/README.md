# Middleware Example

This example demonstrates the middleware system for graph, model, and tool execution.

## Features Demonstrated

### Graph Middleware
- **Logging**: Logs graph execution start, completion, and errors
- **Events**: Publishes execution events to event bus
- **Visualization**: Integrates with viz server for real-time monitoring

### Model Middleware
- **Caching**: Caches model responses to reduce API calls
- **Retry**: Retries failed calls with exponential backoff
- **Rate Limiting**: Prevents quota exhaustion
- **Token Counting**: Tracks token usage for cost monitoring

### Tool Middleware
- **Caching**: Caches tool results for deterministic tools
- **Timeout**: Enforces execution timeouts
- **Circuit Breaker**: Prevents cascading failures
- **Audit**: Logs all tool executions for compliance

## Running the Example

```bash
go run main.go
```

## Architecture

The middleware system uses a layered approach:

```
Agent
  ├─ Graph Middleware (logging, events, viz)
  │   └─ Graph Executor
  │       ├─ Model Middleware (cache, retry, rate limit, tokens)
  │       │   └─ Model Executor
  │       └─ Tool Middleware (cache, timeout, circuit breaker, audit)
  │           └─ Tool Executor
  ```

Each layer can have multiple middleware composed together using the Chain function.
