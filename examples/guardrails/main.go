package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/agent/callbacks"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/plugin"
	"github.com/hupe1980/agentmesh/pkg/plugin/plugins"
)

// GuardrailsPlugin demonstrates content filtering and safety guardrails
type GuardrailsPlugin struct {
	plugin.NoopPlugin
	blockedKeywords []string
	cache           map[string]*model.Response
}

// NewGuardrailsPlugin creates a plugin with content filtering
func NewGuardrailsPlugin() *GuardrailsPlugin {
	return &GuardrailsPlugin{
		blockedKeywords: []string{"hack", "exploit", "bypass"},
		cache:           make(map[string]*model.Response),
	}
}

// BeforeModel blocks unsafe prompts from reaching the model
func (p *GuardrailsPlugin) BeforeModel(ctx context.Context, req *model.Request) (*model.Response, error) {
	// Check for blocked keywords in all messages
	for _, msg := range req.Messages {
		text := strings.ToLower(message.Stringify(msg))
		for _, keyword := range p.blockedKeywords {
			if strings.Contains(text, keyword) {
				// Short-circuit with error
				return nil, fmt.Errorf("content blocked: message contains disallowed keyword '%s'", keyword)
			}
		}
	}

	// Check cache
	cacheKey := p.cacheKey(req)
	if cached, ok := p.cache[cacheKey]; ok {
		fmt.Println("✓ Cache hit - returning cached response")
		return cached, nil
	}

	fmt.Println("✗ Cache miss - continuing to model")
	return nil, nil // Continue to model
}

// AfterModel redacts PII from model responses
func (p *GuardrailsPlugin) AfterModel(ctx context.Context, req *model.Request, resp *model.Response) (*model.Response, error) {
	// Store in cache
	cacheKey := p.cacheKey(req)
	p.cache[cacheKey] = resp
	fmt.Println("✓ Response stored in cache")

	// Filter PII from response
	text := message.Stringify(resp.Message)
	filtered := p.redactPII(text)

	if filtered != text {
		fmt.Println("✓ PII filtered from response")
		return &model.Response{
			Message: message.NewAIMessageFromText(filtered),
		}, nil
	}

	return nil, nil // Keep original
}

// OnModelError provides fallback responses when model calls fail
func (p *GuardrailsPlugin) OnModelError(ctx context.Context, req *model.Request, err error) (*model.Response, error) {
	fmt.Printf("⚠ Model error: %v\n", err)

	// Provide graceful fallback response
	return &model.Response{
		Message: message.NewAIMessageFromText("I apologize, but I'm experiencing technical difficulties. Please try again in a moment."),
	}, nil
}

// cacheKey generates a simple cache key from the request
func (p *GuardrailsPlugin) cacheKey(req *model.Request) string {
	if len(req.Messages) == 0 {
		return ""
	}
	return message.Stringify(req.Messages[len(req.Messages)-1])
}

// redactPII is a simple PII redaction function
func (p *GuardrailsPlugin) redactPII(text string) string {
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

func main() {
	fmt.Println("=== AgentMesh Guardrails Plugin Demo ===")
	fmt.Println()

	// Create plugin manager with guardrails
	pluginMgr := callbacks.NewPluginManager()

	// Register guardrails plugin
	guardrailsPlugin := NewGuardrailsPlugin()
	if err := pluginMgr.Register(context.Background(), guardrailsPlugin); err != nil {
		log.Fatal(err)
	}

	// Also add the built-in cache plugin
	cachePlugin := plugins.NewCachePlugin(100)
	if err := pluginMgr.Register(context.Background(), cachePlugin); err != nil {
		log.Fatal(err)
	}

	fmt.Println("✓ Plugin manager configured:")
	fmt.Println("  - GuardrailsPlugin (content filtering, PII redaction)")
	fmt.Println("  - CachePlugin (response caching)")
	fmt.Println()

	// Test 1: Normal message
	fmt.Println("Test 1: Normal message")
	req1 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("What is the weather today?"),
		},
	}

	result1, err := pluginMgr.ExecuteBeforeModel(context.Background(), req1)
	if err != nil {
		fmt.Printf("❌ Blocked: %v\n", err)
	} else if result1 != nil {
		fmt.Println("✓ Short-circuited with cached response")
	} else {
		fmt.Println("✓ Passed checks - would call model")

		// Simulate model response
		resp1 := &model.Response{
			Message: message.NewAIMessageFromText("The weather is sunny and 72°F."),
		}

		transformed, _ := pluginMgr.ExecuteAfterModel(context.Background(), req1, resp1)
		if transformed != nil {
			fmt.Printf("Final response (transformed): %s\n", message.Stringify(transformed.Message))
		} else {
			fmt.Printf("Final response (original): %s\n", message.Stringify(resp1.Message))
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

	result2, err := pluginMgr.ExecuteBeforeModel(context.Background(), req2)
	if err != nil {
		fmt.Printf("✓ Blocked: %v\n", err)
	} else if result2 != nil {
		fmt.Println("Short-circuited with response")
	} else {
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

	_, err = pluginMgr.ExecuteBeforeModel(context.Background(), req3)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		// Simulate model response with PII
		resp3 := &model.Response{
			Message: message.NewAIMessageFromText("Sure! Contact us at support@example.com or call 123-45-6789."),
		}

		transformed, _ := pluginMgr.ExecuteAfterModel(context.Background(), req3, resp3)
		if transformed != nil {
			fmt.Printf("✓ PII filtered response: %s\n", message.Stringify(transformed.Message))
		} else {
			fmt.Printf("Response: %s\n", message.Stringify(resp3.Message))
		}
	}

	fmt.Println()

	// Test 4: Cache hit (repeat first message)
	fmt.Println("Test 4: Cache hit test")
	result4, err := pluginMgr.ExecuteBeforeModel(context.Background(), req1)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else if result4 != nil {
		fmt.Println("✓ Cache returned response - model call skipped")
		fmt.Printf("Cached response: %s\n", message.Stringify(result4.Message))
	} else {
		fmt.Println("❌ Cache miss (unexpected)")
	}

	fmt.Println()

	// Show cache statistics
	stats := cachePlugin.GetStats()
	fmt.Printf("Cache Statistics:\n")
	fmt.Printf("  - Size: %d\n", stats.Size)
	fmt.Printf("  - Hits: %d\n", stats.Hits)
	fmt.Printf("  - Misses: %d\n", stats.Misses)
	fmt.Printf("  - Hit Rate: %.1f%%\n", stats.HitRate*100)

	fmt.Println("\n=== Demo Complete ===")
}
