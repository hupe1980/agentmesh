---
layout: doc
title: Callbacks
permalink: /callbacks/
hero:
  title: Callback System
  description: Intercept and transform model and tool invocations with composable callbacks.
  primary_cta:
    label: View example
    href: "https://github.com/hupe1980/agentmesh/tree/main/examples/callback_integration"
    external: true
  secondary_cta:
    label: API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/callbacks"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Callback types
    url: "#callback-types"
  - title: Basic usage
    url: "#basic-usage"
  - title: Use cases
    url: "#use-cases"
  - title: Best practices
    url: "#best-practices"
---

# Callbacks

The callback system enables powerful extensions to AgentMesh workflows by intercepting and modifying model and tool invocations at multiple execution stages.

---

## Overview {#overview}

Callbacks provide hooks into the agent execution lifecycle:

- **BeforeModel/BeforeTool** - Intercept requests before execution
- **AfterModel/AfterTool** - Post-process responses after execution
- **OnModelError/OnToolError** - Handle errors with fallback logic

**Key features**:
- ✅ Composable - Chain multiple callbacks in sequence
- ✅ Thread-safe - Safe for concurrent execution
- ✅ Short-circuiting - Return early to skip execution
- ✅ Type-safe - Strongly typed request/response objects

---

## Callback Types {#callback-types}

### Model Callbacks

#### BeforeModelCallback

Intercepts model requests before execution. Can validate, transform, or short-circuit.

```go
type BeforeModelCallback func(
    ctx context.Context,
    req *callbacks.ModelRequest,
) (*callbacks.ModelResponse, error)
```

**Returns**:
- `nil, nil` - Continue to model execution
- `response, nil` - Short-circuit with cached/transformed response
- `nil, error` - Abort with error

#### AfterModelCallback

Post-processes model responses after execution.

```go
type AfterModelCallback func(
    ctx context.Context,
    req *callbacks.ModelRequest,
    resp *callbacks.ModelResponse,
) (*callbacks.ModelResponse, error)
```

**Returns**:
- `response, nil` - Return (potentially modified) response
- `nil, error` - Abort with error

#### OnModelErrorCallback

Handles model execution errors with fallback logic.

```go
type OnModelErrorCallback func(
    ctx context.Context,
    req *callbacks.ModelRequest,
    err error,
) (*callbacks.ModelResponse, error)
```

**Returns**:
- `response, nil` - Recover with fallback response
- `nil, error` - Propagate or transform error

### Tool Callbacks

Tool callbacks follow the same pattern as model callbacks:

- `BeforeToolCallback` - Intercept tool requests
- `AfterToolCallback` - Post-process tool responses
- `OnToolErrorCallback` - Handle tool errors

---

## Basic Usage {#basic-usage}

### Creating a Callback Manager

```go
import "github.com/hupe1980/agentmesh/pkg/callbacks"

// Create manager
manager := callbacks.NewManager()

// Register callbacks
manager.RegisterBeforeModel(validateInputCallback)
manager.RegisterAfterModel(loggingCallback)
manager.RegisterOnModelError(fallbackCallback)
```

### Using with ReAct Agents

```go
import "github.com/hupe1980/agentmesh/pkg/agent"

compiled, err := agent.NewReActAgent(
    model,
    tools,
    agent.WithModelCallbacks(manager),
    agent.WithToolCallbacks(manager),
)
```

### Complete Example

```go
package main

import (
    "context"
    "log"

    "github.com/hupe1980/agentmesh/pkg/callbacks"
    "github.com/hupe1980/agentmesh/pkg/agent"
)

func main() {
    manager := callbacks.NewManager()
    
    // Content filtering
    manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
        for _, msg := range req.Messages {
            if containsProfanity(msg) {
                return nil, errors.New("inappropriate content detected")
            }
        }
        return nil, nil // Continue
    })
    
    // Response logging
    manager.RegisterAfterModel(func(ctx context.Context, req *callbacks.ModelRequest, resp *callbacks.ModelResponse) (*callbacks.ModelResponse, error) {
        log.Printf("Model: %s, Tokens: %d", req.Model, resp.TokenUsage.Total)
        return resp, nil
    })
    
    // Fallback on error
    manager.RegisterOnModelError(func(ctx context.Context, req *callbacks.ModelRequest, err error) (*callbacks.ModelResponse, error) {
        if isRateLimitError(err) {
            log.Println("Rate limited, using fallback")
            return getFallbackResponse(req), nil
        }
        return nil, err
    })
    
    // Create agent with callbacks
    compiled, _ := agent.NewReActAgent(
        model,
        tools,
        agent.WithModelCallbacks(manager),
    )
    
    results, _ := compiled.Invoke(ctx, messages)
}
```

---

## Use Cases {#use-cases}

### 1. Content Guardrails

Filter unsafe content before and after model execution:

```go
// Input filtering
manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    for _, msg := range req.Messages {
        if containsPII(msg) {
            return nil, errors.New("PII detected in input")
        }
        if containsUnsafeContent(msg) {
            return nil, errors.New("unsafe content detected")
        }
    }
    return nil, nil
})

// Output filtering
manager.RegisterAfterModel(func(ctx context.Context, req *callbacks.ModelRequest, resp *callbacks.ModelResponse) (*callbacks.ModelResponse, error) {
    filtered := filterPII(resp.Message)
    resp.Message = filtered
    return resp, nil
})
```

### 2. Response Caching

Cache model responses to reduce latency and cost:

```go
var cache = make(map[string]*callbacks.ModelResponse)

manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    key := hashRequest(req)
    if cached, ok := cache[key]; ok {
        log.Println("Cache hit")
        return cached, nil // Short-circuit
    }
    return nil, nil // Continue to model
})

manager.RegisterAfterModel(func(ctx context.Context, req *callbacks.ModelRequest, resp *callbacks.ModelResponse) (*callbacks.ModelResponse, error) {
    key := hashRequest(req)
    cache[key] = resp
    return resp, nil
})
```

### 3. Metrics & Monitoring

Track model performance and usage:

```go
manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    ctx = context.WithValue(ctx, "startTime", time.Now())
    return nil, nil
})

manager.RegisterAfterModel(func(ctx context.Context, req *callbacks.ModelRequest, resp *callbacks.ModelResponse) (*callbacks.ModelResponse, error) {
    start := ctx.Value("startTime").(time.Time)
    latency := time.Since(start)
    
    metrics.RecordModelLatency(req.Model, latency)
    metrics.RecordTokens(resp.TokenUsage.Total)
    
    return resp, nil
})

manager.RegisterOnModelError(func(ctx context.Context, req *callbacks.ModelRequest, err error) (*callbacks.ModelResponse, error) {
    metrics.RecordModelError(req.Model, err)
    return nil, err
})
```

### 4. Policy Enforcement

Enforce business rules on model usage:

```go
manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    user := ctx.Value("user").(string)
    
    // Check rate limits
    if exceedsRateLimit(user) {
        return nil, errors.New("rate limit exceeded")
    }
    
    // Check permissions
    if !hasPermission(user, req.Model) {
        return nil, errors.New("unauthorized model access")
    }
    
    // Check cost limits
    estimatedCost := estimateTokenCost(req.Messages)
    if exceedsBudget(user, estimatedCost) {
        return nil, errors.New("budget exceeded")
    }
    
    return nil, nil
})
```

### 5. Retry & Fallback

Implement sophisticated retry logic with fallbacks:

```go
manager.RegisterOnModelError(func(ctx context.Context, req *callbacks.ModelRequest, err error) (*callbacks.ModelResponse, error) {
    // Retry on transient errors
    if isTransientError(err) {
        retries := ctx.Value("retries").(int)
        if retries < 3 {
            time.Sleep(backoff(retries))
            ctx = context.WithValue(ctx, "retries", retries+1)
            // Trigger retry by returning original error
            return nil, err
        }
    }
    
    // Fallback to cheaper model
    if isRateLimitError(err) && req.Model == "gpt-4" {
        log.Println("Falling back to gpt-3.5-turbo")
        req.Model = "gpt-3.5-turbo"
        // Execute with fallback model
        return executeFallbackModel(ctx, req)
    }
    
    return nil, err
})
```

### 6. Tool Validation

Validate tool arguments and results:

```go
manager.RegisterBeforeTool(func(ctx context.Context, req *callbacks.ToolRequest) (*callbacks.ToolResponse, error) {
    // Validate arguments
    if err := validateToolArgs(req.Name, req.Arguments); err != nil {
        return nil, fmt.Errorf("invalid tool arguments: %w", err)
    }
    
    // Check authorization
    if !isAuthorizedTool(ctx, req.Name) {
        return nil, errors.New("unauthorized tool access")
    }
    
    return nil, nil
})

manager.RegisterAfterTool(func(ctx context.Context, req *callbacks.ToolRequest, resp *callbacks.ToolResponse) (*callbacks.ToolResponse, error) {
    // Validate result format
    if !isValidToolResult(resp.Result) {
        return nil, errors.New("invalid tool result format")
    }
    
    // Sanitize output
    resp.Result = sanitizeToolOutput(resp.Result)
    
    return resp, nil
})
```

---

## Best Practices {#best-practices}

### 1. Keep Callbacks Fast

Callbacks execute in the hot path. Avoid expensive operations:

```go
// ❌ Bad - expensive operation
manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    // Don't do expensive database lookups
    rules := fetchRulesFromDatabase()
    return validateAgainstRules(req, rules)
})

// ✅ Good - use cached rules
var rulesCache = loadRulesAtStartup()

manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    return validateAgainstRules(req, rulesCache)
})
```

### 2. Use Context for State

Pass state between callbacks using context:

```go
manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    ctx = context.WithValue(ctx, "requestID", uuid.New())
    ctx = context.WithValue(ctx, "startTime", time.Now())
    return nil, nil
})

manager.RegisterAfterModel(func(ctx context.Context, req *callbacks.ModelRequest, resp *callbacks.ModelResponse) (*callbacks.ModelResponse, error) {
    requestID := ctx.Value("requestID")
    startTime := ctx.Value("startTime").(time.Time)
    log.Printf("[%s] Completed in %v", requestID, time.Since(startTime))
    return resp, nil
})
```

### 3. Handle Errors Gracefully

Decide whether to fail fast or recover:

```go
manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    // Critical validation - fail fast
    if containsMalware(req) {
        return nil, errors.New("security violation")
    }
    
    // Optional enhancement - log and continue
    if err := logToAnalytics(req); err != nil {
        log.Printf("Analytics logging failed: %v", err)
        // Don't fail the request
    }
    
    return nil, nil
})
```

### 4. Order Matters

Callbacks execute in registration order:

```go
// Register in logical order
manager.RegisterBeforeModel(authCallback)      // 1. Authenticate
manager.RegisterBeforeModel(validationCallback) // 2. Validate
manager.RegisterBeforeModel(cacheCallback)      // 3. Check cache
manager.RegisterBeforeModel(loggingCallback)    // 4. Log
```

### 5. Thread Safety

CallbackManager is thread-safe, but your callbacks should be too:

```go
// ✅ Thread-safe cache
var cache sync.Map

manager.RegisterBeforeModel(func(ctx context.Context, req *callbacks.ModelRequest) (*callbacks.ModelResponse, error) {
    key := hashRequest(req)
    if val, ok := cache.Load(key); ok {
        return val.(*callbacks.ModelResponse), nil
    }
    return nil, nil
})
```

### 6. Testing Callbacks

Test callbacks in isolation:

```go
func TestContentFilterCallback(t *testing.T) {
    cb := contentFilterCallback()
    
    req := &callbacks.ModelRequest{
        Messages: []message.Message{
            message.NewHumanMessageFromText("inappropriate content"),
        },
    }
    
    resp, err := cb(context.Background(), req)
    assert.Error(t, err)
    assert.Nil(t, resp)
}
```

---

## Examples

- **[callback_integration](https://github.com/hupe1980/agentmesh/tree/main/examples/callback_integration)** - Complete callback system demonstration
- **[guardrails](https://github.com/hupe1980/agentmesh/tree/main/examples/guardrails)** - Content filtering with callbacks

---

## See Also

- [Agents Guide](/agents/) - Using callbacks with ReAct agents
- [Observability](/observability/) - Combining callbacks with metrics
- [API Reference](https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/callbacks)
