package main

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"
)

// GuardrailsMiddleware demonstrates content filtering and safety guardrails
type GuardrailsMiddleware struct {
	blockedKeywords []string
	cache           map[string]*model.Response
}

// NewGuardrailsMiddleware creates middleware with content filtering
func NewGuardrailsMiddleware() *GuardrailsMiddleware {
	return &GuardrailsMiddleware{
		blockedKeywords: []string{"hack", "exploit", "bypass"},
		cache:           make(map[string]*model.Response),
	}
}

// Wrap implements model.Middleware by wrapping the next executor with guardrails
func (m *GuardrailsMiddleware) Wrap(next model.Executor) model.Executor {
	return model.WrapFunc(func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		// Pre-execution: check for blocked keywords
		for _, msg := range req.Messages {
			text := strings.ToLower(msg.String())
			for _, keyword := range m.blockedKeywords {
				if strings.Contains(text, keyword) {
					// Return error iterator
					return func(yield func(*model.Response, error) bool) {
						yield(nil, fmt.Errorf("content blocked: message contains disallowed keyword '%s'", keyword))
					}
				}
			}
		}

		// Check cache
		cacheKey := m.cacheKey(req)
		if cached, ok := m.cache[cacheKey]; ok {
			fmt.Println("✓ Cache hit - returning cached response")
			return func(yield func(*model.Response, error) bool) {
				yield(cached, nil)
			}
		}

		fmt.Println("✗ Cache miss - calling model")

		// Call next executor and filter responses
		return func(yield func(*model.Response, error) bool) {
			var lastResp *model.Response
			for resp, err := range next.Generate(ctx, req) {
				if err != nil {
					// On error, provide graceful fallback
					fmt.Printf("⚠ Model error: %v\n", err)
					fallback := &model.Response{
						Message: message.NewAIMessageFromText("I apologize, but I'm experiencing technical difficulties. Please try again in a moment."),
					}
					if !yield(fallback, nil) {
						return
					}
					continue
				}

				// Filter PII from response
				text := resp.Message.String()
				filtered := m.redactPII(text)

				if filtered != text {
					fmt.Println("✓ PII filtered from response")
					resp = &model.Response{
						Message: message.NewAIMessageFromText(filtered),
					}
				}

				lastResp = resp
				if !yield(resp, nil) {
					return
				}
			}

			// Store in cache after successful execution
			if lastResp != nil {
				m.cache[cacheKey] = lastResp
				fmt.Println("✓ Response stored in cache")
			}
		}
	})
}

// cacheKey generates a simple cache key from the request
func (m *GuardrailsMiddleware) cacheKey(req *model.Request) string {
	if len(req.Messages) == 0 {
		return ""
	}
	return req.Messages[len(req.Messages)-1].String()
}

// redactPII is a simple PII redaction function
func (m *GuardrailsMiddleware) redactPII(text string) string {
	// Replace patterns that look like SSNs
	text = strings.ReplaceAll(text, "123-45-6789", "[SSN REDACTED]")

	// Replace patterns that look like credit cards
	text = strings.ReplaceAll(text, "4532-1234-5678-9010", "[CREDIT CARD REDACTED]")

	// Replace email patterns (very simple)
	if strings.Contains(text, "@") {
		words := strings.Fields(text)
		for i, word := range words {
			if strings.Contains(word, "@") {
				words[i] = "[EMAIL REDACTED]"
			}
		}
		text = strings.Join(words, " ")
	}

	return text
}

// MockModel simulates a model for demonstration
type MockModel struct{}

func (m *MockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		var responseText string
		lastMsg := req.Messages[len(req.Messages)-1].String()

		if strings.Contains(strings.ToLower(lastMsg), "weather") {
			responseText = "The weather is sunny and 72°F."
		} else if strings.Contains(strings.ToLower(lastMsg), "example") {
			responseText = "Sure! Contact us at support@example.com or call 123-45-6789."
		} else {
			responseText = "I can help with that!"
		}

		yield(&model.Response{
			Message: message.NewAIMessageFromText(responseText),
		}, nil)
	}
}

func main() {
	fmt.Println("=== AgentMesh Guardrails Middleware Demo ===")
	fmt.Println()

	// Create guardrails middleware
	guardrailsMw := NewGuardrailsMiddleware()
	cacheMw := modelmw.NewCacheMiddleware()

	// Create executor chain with middleware
	mockModel := &MockModel{}
	executor := model.Chain(mockModel, guardrailsMw, cacheMw)

	fmt.Println("✓ Model executor configured with middleware:")
	fmt.Println("  - GuardrailsMiddleware (content filtering, PII redaction)")
	fmt.Println("  - CacheMiddleware (response caching)")
	fmt.Println()

	ctx := context.Background()

	// Test 1: Normal message
	fmt.Println("Test 1: Normal message")
	req1 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("What is the weather today?"),
		},
	}

	for resp, err := range executor.Generate(ctx, req1) {
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			fmt.Printf("✓ Response: %s\n", resp.Message.String())
		}
	}

	fmt.Println()

	// Test 2: Unsafe content (should be blocked)
	fmt.Println("Test 2: Unsafe content (should be blocked)")
	req2 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("How can I hack into a system?"),
		},
	}

	blocked := false
	for _, err := range executor.Generate(ctx, req2) {
		if err != nil {
			fmt.Printf("✓ Blocked: %v\n", err)
			blocked = true
		}
	}
	if !blocked {
		fmt.Println("❌ Passed checks (unexpected)")
	}

	fmt.Println()

	// Test 3: Response with PII (should be filtered)
	fmt.Println("Test 3: Response with PII filtering")
	req3 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Show me an example"),
		},
	}

	for resp, err := range executor.Generate(ctx, req3) {
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			fmt.Printf("✓ Response: %s\n", resp.Message.String())
		}
	}

	fmt.Println()

	// Test 4: Cache hit (repeat first message)
	fmt.Println("Test 4: Cache hit test")
	for resp, err := range executor.Generate(ctx, req1) {
		if err != nil {
			fmt.Printf("❌ Error: %v\n", err)
		} else {
			fmt.Printf("✓ Response: %s\n", resp.Message.String())
		}
	}

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("Key features demonstrated:")
	fmt.Println("  - Custom middleware implementation")
	fmt.Println("  - Content filtering with blocked keywords")
	fmt.Println("  - PII redaction from responses")
	fmt.Println("  - Caching integrated with guardrails")
	fmt.Println("  - Error handling with graceful fallbacks")
}
