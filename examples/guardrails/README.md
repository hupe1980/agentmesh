# Example: Guardrails

## Overview
Demonstrates content validation using the `pkg/guardrail` package. Shows how to implement guardrails for LLM applications using built-in guardrail types and middleware.

## Key Concepts
- **Guardrail Package**: Type-safe content validation with `Guardrail[T]` interface
- **Built-in Guardrails**: ContentFilter, Length, Regex
- **Guardrail Actions**: Allow, Reject (soft), and Raise (hard tripwire)
- **Middleware Integration**: Apply guardrails at model layer via middleware
- **Guardrail Chaining**: Combine multiple guardrails with `guardrail.Chain()`

## Running
```bash
cd examples/guardrails
go run main.go
```

## Expected Output
```
=== AgentMesh Guardrails Demo ===

✓ Model executor configured with middleware:
  - ContentFilterGuardrail (keyword blocking)
  - LengthGuardrail (min/max content length)
  - RegexGuardrail (custom pattern matching)

Test 1: Normal message
-----------------------
✓ Response: The weather is sunny and 72°F.

Test 2: Blocked content (should be rejected)
--------------------------------------------
✓ Rejected by guardrail: Content contains blocked keyword (hack)

Test 3: Too short input (should be rejected)
--------------------------------------------
✓ Rejected: Content too short: 2 < 5

Test 4: Response too long (output guardrail)
--------------------------------------------
✓ Output rejected: Content too long: 1500 > 500

Test 5: Direct guardrail chain usage
-------------------------------------
Action: allow
Message: (allowed)

=== Demo Complete ===
```

## Implementation

### Creating Guardrails
```go
import "github.com/hupe1980/agentmesh/pkg/guardrail"

// Content filtering with blocked keywords
contentFilter := guardrail.NewContentFilterGuardrail(
    []string{"hack", "exploit", "bypass"},
    guardrail.WithContentFilterAction(guardrail.ActionReject),
)

// Length validation
lengthGuardrail := guardrail.NewLengthGuardrail(
    guardrail.WithMinLength(5),
    guardrail.WithMaxLength(500),
)

// Custom regex pattern
profanityFilter := guardrail.NewRegexGuardrail(
    "profanity_filter",
    regexp.MustCompile(`(?i)\b(badword)\b`),
    guardrail.WithRegexAction(guardrail.ActionRaise),
    guardrail.WithDescription("Profanity detected"),
)
```

### Using Middleware
```go
import modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"

// Create guardrail middleware
guardrailMw := modelmw.NewGuardrailMiddleware(
    modelmw.WithInputGuardrails(contentFilter, lengthGuardrail),
    modelmw.WithOutputGuardrails(lengthGuardrail),
)

// Chain with model
executor := model.Chain(myModel, guardrailMw)
```

### Direct Guardrail Chaining
```go
// Check content directly
result, err := guardrail.Chain(ctx, "test message", contentFilter, lengthGuardrail)
if result.Action == guardrail.ActionReject {
    // Handle rejection
}
```

### Error Handling
```go
for resp, err := range executor.Generate(ctx, req) {
    if err != nil {
        if rejection, ok := err.(*guardrail.Rejection); ok {
            // Soft rejection - can retry with modifications
            log.Printf("Rejected: %s", rejection.Message)
        } else if tripwire, ok := err.(*guardrail.TripwireError); ok {
            // Hard tripwire - escalate
            log.Printf("Tripwire: %s", tripwire.Message)
        }
    }
}
```

## Guardrail Actions

| Action | Description | Error Type |
|--------|-------------|------------|
| `ActionAllow` | Content is safe, continue processing | None |
| `ActionReject` | Soft rejection, can retry with modifications | `*guardrail.Rejection` |
| `ActionRaise` | Hard tripwire, escalate | `*guardrail.TripwireError` |

## Built-in Guardrails

| Guardrail | Purpose | Default Action |
|-----------|---------|----------------|
| `ContentFilterGuardrail` | Block keywords/phrases | `ActionReject` |
| `LengthGuardrail` | Validate content length | `ActionReject` |
| `RegexGuardrail` | Custom pattern matching | `ActionReject` |

## Production Security: External Service Integrations

For production security use cases (PII detection, content moderation, etc.), use specialized external services that provide robust, battle-tested detection:

- `pkg/guardrail/openai` - OpenAI Moderation API
- `pkg/guardrail/amazoncomprehend` - AWS Comprehend (sentiment, PII)

> **Note**: Simple regex-based patterns for security-sensitive detection (SQL injection, PII, etc.) are easily bypassed and should not be used in production. Always use specialized services for security-critical content validation.
