# Example: Guardrails

## Overview
Demonstrates content filtering and PII redaction using plugins. Shows how to implement security guardrails for production LLM applications with the type-safe plugin system.

## Key Concepts
- **Security Plugins**: Content validation and PII protection
- **Response Caching**: Performance optimization with CachePlugin
- **Plugin Composition**: Multiple plugins working together
- **Short-Circuit Returns**: Block unsafe content before model invocation

## Running
```bash
cd examples/guardrails
go run main.go
```

## Expected Output
```
=== Guardrails Example ===

Plugin manager configured with:
- GuardrailsPlugin (content filtering + PII redaction)
- CachePlugin (response caching)

Test 1: Normal Request
Input: "What is the capital of France?"
Output: "The capital of France is Paris."
✓ Passed validation

Test 2: Blocked Content
Input: "How to hack a system?"
❌ Blocked: Content violates safety policy

Test 3: PII Redaction
Input: "My email is john@example.com and phone is 555-1234"
Output: "My email is [EMAIL_REDACTED] and phone is [PHONE_REDACTED]"
✓ PII redacted

Test 4: Cache Hit
Input: "What is the capital of France?"  # Same as Test 1
Output: "The capital of France is Paris." (from cache)
⚡ Cache hit! (0ms)

Statistics:
- Total requests: 4
- Blocked: 1
- PII redactions: 2
- Cache hits: 1
- Cache hit rate: 25%
```

## Implementation

### GuardrailsPlugin
```go
type GuardrailsPlugin struct {
    callbacks.NoopPlugin
    
    stats GuardrailStats
    mu    sync.Mutex
}

func (p *GuardrailsPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
    for _, msg := range req.Messages {
        content := message.Stringify(msg)
        
        // Check for unsafe content
        if containsUnsafeContent(content) {
            p.recordBlocked()
            return nil, fmt.Errorf("blocked: unsafe content detected")
        }
    }
    return nil, nil  // Continue to model
}

func (p *GuardrailsPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
    content := message.Stringify(resp.Message)
    
    // Redact PII
    redacted, count := redactPII(content)
    p.recordPIIRedactions(count)
    
    return &model.Response{
        Message: message.NewAIMessage(message.NewTextPart(redacted)),
        Usage:   resp.Usage,
    }, nil
}
```

### CachePlugin
```go
cache := plugin.NewCachePlugin(100)  // max 100 entries

pm := callbacks.NewPluginManager()
pm.Register(&GuardrailsPlugin{})
pm.Register(cache)
```

### Integration
```go
// Callbacks are automatically injected via context
reactAgent, _ := agent.NewReActAgent(
    model,
    agent.WithTools(tools...),
    agent.WithPluginManager(pm),
)
```

## Security Patterns

### Content Filtering (BeforeModel)
- Block unsafe prompts
- Enforce content policies
- Validate input constraints

### PII Redaction (AfterModel)
- Email addresses → `[EMAIL_REDACTED]`
- Phone numbers → `[PHONE_REDACTED]`
- SSN/Credit cards → `[SENSITIVE_REDACTED]`

### Combined with Caching
- Cache safe responses
- Skip blocked requests
- Performance + Security

## Use Cases
- ✅ Input validation with BeforeModel plugins
- ✅ Output filtering with AfterModel plugins
- ✅ PII protection for compliance (GDPR, HIPAA)
- ✅ Content moderation for user-facing apps
- ✅ Rate limiting and quota enforcement
- ✅ Audit logging for security

## Plugin Composition

Plugins execute in registration order:
1. **GuardrailsPlugin** - Security first
2. **CachePlugin** - Performance optimization

Both plugins share the same PluginManager and can maintain independent state.

## Statistics

```go
stats := guardrailsPlugin.GetStats()
fmt.Printf("Blocked: %d\n", stats.Blocked)
fmt.Printf("PII Redactions: %d\n", stats.PIIRedactions)

cacheStats := cachePlugin.GetStats()
fmt.Printf("Cache Hits: %d\n", cacheStats.Hits)
fmt.Printf("Hit Rate: %.1f%%\n", cacheStats.HitRate*100)
```

## Related Resources
- [pkg/callbacks/plugins](../../pkg/callbacks/plugins) - Built-in plugins
- [examples/circuit_breaker](../circuit_breaker) - Resilience plugins
- [examples/callback_integration](../callback_integration) - Plugin basics
