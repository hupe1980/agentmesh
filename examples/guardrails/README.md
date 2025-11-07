# Example: Guardrails

## Overview
Demonstrates content filtering and PII redaction using callbacks. Shows how to implement security guardrails for production LLM applications.

## Key Concepts
- **Input Validation**: Block unsafe prompts before LLM
- **Output Filtering**: Redact sensitive information after LLM
- **Content Policies**: Enforce acceptable use policies
- **PII Protection**: Automatic detection and redaction
- **Short-Circuit**: Stop execution on policy violations

## Running
```bash
cd examples/guardrails
go run main.go
```

## Expected Output
```
=== Guardrails Example ===

Test 1: Normal Request
User: "What is the capital of France?"
✓ Passed validation
Assistant: "The capital of France is Paris."

Test 2: Blocked Content
User: "How to hack a system?"
❌ Blocked: content contains disallowed keyword 'hack'
Assistant: [Request blocked before reaching model]

Test 3: PII Redaction
User: "My email is john@example.com and SSN is 123-45-6789"
✓ Passed validation
Assistant: "Your email is [EMAIL] and SSN is [SSN]"

Test 4: URL Filtering
User: "Check out http://malicious-site.com"
✓ Passed validation (URL filtered)
Assistant: "Check out [FILTERED_URL]"
```

## Code Walkthrough

### 1. Create Input Validator
```go
func BlockUnsafeContent(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    blockedKeywords := []string{"hack", "exploit", "bypass"}
    
    messages := s.MessagesSnapshot()
    for _, msg := range messages {
        for _, part := range msg.Parts() {
            if textPart, ok := part.(message.TextPart); ok {
                for _, keyword := range blockedKeywords {
                    if strings.Contains(strings.ToLower(textPart.Text), keyword) {
                        return nil, fmt.Errorf(
                            "content blocked: disallowed keyword '%s'", 
                            keyword,
                        )
                    }
                }
            }
        }
    }
    return nil, nil // Pass validation
}
```

### 2. Create Output Filter
```go
func FilterPII(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    messages := s.MessagesSnapshot()
    lastMsg := messages[len(messages)-1]
    
    // Redact PII patterns
    text := getMessageText(lastMsg)
    text = emailRegex.ReplaceAllString(text, "[EMAIL]")
    text = ssnRegex.ReplaceAllString(text, "[SSN]")
    text = phoneRegex.ReplaceAllString(text, "[PHONE]")
    
    return message.NewAIMessageFromText(text), nil
}
```

### 3. Register Guardrails
```go
cbManager := callbacks.NewManager()
cbManager.RegisterBeforeModel(BlockUnsafeContent)
cbManager.RegisterAfterModel(FilterPII)

modelNode := agent.ModelNode(model,
    agent.WithModelCallbacks(cbManager),
)
```

## Common Patterns

### Email Redaction
```go
emailRegex := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
text = emailRegex.ReplaceAllString(text, "[EMAIL]")
```

### SSN Redaction
```go
ssnRegex := regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
text = ssnRegex.ReplaceAllString(text, "[SSN]")
```

### Credit Card Redaction
```go
ccRegex := regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`)
text = ccRegex.ReplaceAllString(text, "[CREDIT_CARD]")
```

### URL Filtering
```go
urlRegex := regexp.MustCompile(`https?://[^\s]+`)
text = urlRegex.ReplaceAllString(text, "[FILTERED_URL]")
```

## Content Policy Examples

### Profanity Filter
```go
func BlockProfanity(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    profanityList := loadProfanityList()
    // Check messages...
}
```

### Topic Restrictions
```go
func EnforceTopicPolicy(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    allowedTopics := []string{"weather", "news", "sports"}
    // Validate topic...
}
```

### Length Limits
```go
func EnforceLengthLimit(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    maxLength := 1000
    text := getMessageText(lastMessage)
    if len(text) > maxLength {
        return nil, fmt.Errorf("message too long: %d > %d", len(text), maxLength)
    }
    return nil, nil
}
```

## What This Example Teaches
- ✅ Input validation with BeforeModel callbacks
- ✅ Output filtering with AfterModel callbacks
- ✅ PII detection and redaction
- ✅ Content policy enforcement
- ✅ Security best practices

## Production Considerations

### Comprehensive PII Detection
- Use specialized libraries (e.g., Microsoft Presidio)
- Consider context (false positives)
- Support multiple languages
- Handle edge cases

### Performance
```go
// Compile regex patterns once
var (
    emailRegex = regexp.MustCompile(emailPattern)
    ssnRegex   = regexp.MustCompile(ssnPattern)
)
```

### Logging & Auditing
```go
func AuditBlockedContent(ctx context.Context, s graph.StateWriter) (message.Message, error) {
    if isBlocked {
        log.Printf("Blocked content: user=%s, reason=%s", userID, reason)
    }
}
```

## Next Steps
- Implement custom PII detectors
- Add compliance logging
- Integrate with DLP (Data Loss Prevention) systems
- See **examples/callback_integration** for advanced patterns

## See Also
- [pkg/callbacks](../../pkg/callbacks) - Callback system
- [examples/callback_integration](../callback_integration) - Callback patterns
- [examples/circuit_breaker](../circuit_breaker) - Error handling
