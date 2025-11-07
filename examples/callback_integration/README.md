# Example: Callback Integration

## Overview
Demonstrates comprehensive callback system integration with ModelNode and ToolNode. Shows how to intercept and transform requests/responses at multiple execution stages.

## Key Concepts
- **BeforeModel**: Request validation before LLM calls
- **AfterModel**: Response transformation after LLM calls
- **BeforeTool**: Tool access control and parameter validation
- **AfterTool**: Tool result transformation
- **OnToolError**: Error handling and recovery
- **Callback Composition**: Multiple callbacks in sequence

## Running
```bash
cd examples/callback_integration
go run main.go
```

## Expected Output
```
=== AgentMesh Callback Integration Demo ===

✓ Callback manager configured with 5 callbacks
  - BeforeModel: validateRequest
  - AfterModel: sanitizeResponse
  - BeforeTool: validateToolAccess
  - AfterTool: transformToolResult
  - OnToolError: handleToolError

Example demonstrates:
1. Request validation (BeforeModel)
   - Check message content before LLM
   - Validate user permissions
   - Enforce rate limits

2. Response sanitization (AfterModel)
   - Remove sensitive information
   - Format/transform output
   - Add metadata

3. Tool access control (BeforeTool)
   - Permission checks
   - Parameter validation
   - Usage logging

4. Tool result transformation (AfterTool)
   - Parse and validate results
   - Add context/metadata
   - Cache responses

5. Error handling (OnToolError)
   - Graceful degradation
   - Retry logic
   - Error reporting
```

## Code Walkthrough

### 1. Create Callback Manager
```go
cbManager := callbacks.NewManager()
```

### 2. Define Request Validator
```go
validateRequest := func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    messages := s.MessagesSnapshot()
    // Validate request...
    return nil, nil // Continue to model
}
```

### 3. Define Response Sanitizer
```go
sanitizeResponse := func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    // Get last message
    messages := s.MessagesSnapshot()
    lastMsg := messages[len(messages)-1]
    // Sanitize and return modified message
    return sanitizedMsg, nil
}
```

### 4. Register All Callbacks
```go
cbManager.RegisterBeforeModel(validateRequest)
cbManager.RegisterAfterModel(sanitizeResponse)
cbManager.RegisterBeforeTool(validateToolAccess)
cbManager.RegisterAfterTool(transformToolResult)
cbManager.RegisterOnToolError(handleToolError)
```

### 5. Attach to Nodes
```go
modelNode := agent.ModelNode(model, 
    agent.WithModelCallbacks(cbManager),
)

toolNode := agent.ToolNode(toolset,
    agent.WithToolCallbacks(cbManager),
)
```

## Callback Execution Order

### Model Invocation
1. **BeforeModel** → Validate request
2. **Model.Generate()** → LLM inference  
3. **AfterModel** → Transform response

### Tool Invocation
1. **BeforeTool** → Access control
2. **Tool.Call()** → Execute tool
3. **AfterTool** → Transform result
4. **OnToolError** → Handle failures (if error occurred)

## What This Example Teaches
- ✅ Complete callback lifecycle
- ✅ Request/response transformation
- ✅ Access control implementation
- ✅ Error handling strategies
- ✅ Callback composition patterns

## Common Use Cases

### Security & Compliance
```go
// PII detection and redaction
cbManager.RegisterAfterModel(redactPII)

// Content policy enforcement
cbManager.RegisterBeforeModel(enforceContentPolicy)
```

### Observability
```go
// Request logging
cbManager.RegisterBeforeModel(logRequest)

// Performance tracking
cbManager.RegisterAfterModel(recordLatency)
```

### Cost Management
```go
// Token counting
cbManager.RegisterAfterModel(countTokens)

// Budget enforcement
cbManager.RegisterBeforeModel(checkBudget)
```

## Next Steps
- Implement custom callbacks for your use case
- Combine multiple callbacks for complex workflows
- See **examples/guardrails** for content filtering patterns
- See **examples/circuit_breaker** for resilience patterns

## See Also
- [pkg/callbacks](../../pkg/callbacks) - Callback system documentation
- [pkg/callbacks/policies](../../pkg/callbacks/policies) - Pre-built policies
- [examples/guardrails](../guardrails) - Security callbacks
