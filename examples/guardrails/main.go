package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/callbacks"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// BlockUnsafeContent is a BeforeModel callback that prevents unsafe prompts from reaching the model.
// It checks for blocked keywords and returns an error if found.
func BlockUnsafeContent(ctx context.Context, s graph.StateWriter) (message.Message, error) {
	blockedKeywords := []string{"hack", "exploit", "bypass"}

	// Check all messages for blocked content
	events := s.EventsSnapshot()
	for _, evt := range events {
		parts := evt.Message.Parts()
		for _, part := range parts {
			if textPart, ok := part.(message.TextPart); ok {
				lowerText := strings.ToLower(textPart.Text)
				for _, keyword := range blockedKeywords {
					if strings.Contains(lowerText, keyword) {
						// Short-circuit with error instead of calling the model
						return nil, fmt.Errorf("content blocked: message contains disallowed keyword '%s'", keyword)
					}
				}
			}
		}
	}

	return nil, nil // Continue to model
}

// FilterPII is an AfterModel callback that redacts sensitive information from model responses.
// It demonstrates output transformation for compliance and privacy.
func FilterPII(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
	// Only filter AIMessage responses
	aiMsg, ok := response.(*message.AIMessage)
	if !ok {
		return nil, nil // Keep original
	}

	parts := aiMsg.Parts()
	filteredParts := make(message.Parts, len(parts))
	modified := false

	for i, part := range parts {
		if textPart, ok := part.(message.TextPart); ok {
			filtered := redactPII(textPart.Text)
			if filtered != textPart.Text {
				modified = true
				filteredParts[i] = message.NewTextPart(filtered)
			} else {
				filteredParts[i] = part
			}
		} else {
			filteredParts[i] = part
		}
	}

	if modified {
		return message.NewAIMessage(filteredParts), nil
	}

	return nil, nil // Keep original
}

// redactPII is a simple PII redaction function (in production, use a proper PII detection library)
func redactPII(text string) string {
	// This is a simplified example - production code should use proper PII detection
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

// CacheResponses demonstrates a BeforeModel callback that implements response caching
func CacheResponses(cache map[string]message.Message) callbacks.BeforeModelCallback {
	return func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		// Simple cache key from last message
		events := s.EventsSnapshot()
		if len(events) == 0 {
			return nil, nil
		}

		lastMsg := events[len(events)-1].Message
		parts := lastMsg.Parts()
		if len(parts) == 0 {
			return nil, nil
		}

		cacheKey := ""
		for _, part := range parts {
			if textPart, ok := part.(message.TextPart); ok {
				cacheKey += textPart.Text
			}
		}

		if cacheKey == "" {
			return nil, nil
		}

		// Check cache
		if cached, ok := cache[cacheKey]; ok {
			fmt.Println("✓ Cache hit - returning cached response")
			return cached, nil
		}

		fmt.Println("✗ Cache miss - continuing to model")
		return nil, nil
	}
}

// StoreInCache is an AfterModel callback that stores responses in the cache
func StoreInCache(cache map[string]message.Message) callbacks.AfterModelCallback {
	return func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		events := s.EventsSnapshot()
		if len(events) == 0 {
			return nil, nil
		}

		lastMsg := events[len(events)-1].Message
		parts := lastMsg.Parts()
		if len(parts) == 0 {
			return nil, nil
		}

		cacheKey := ""
		for _, part := range parts {
			if textPart, ok := part.(message.TextPart); ok {
				cacheKey += textPart.Text
			}
		}

		if cacheKey != "" {
			cache[cacheKey] = response
			fmt.Println("✓ Response stored in cache")
		}

		return nil, nil // Keep original
	}
}

// mockStateWriter is a simple implementation of graph.StateWriter for demo purposes
type mockStateWriter struct {
	messages []message.Message
	state    map[string]any
}

func newMockState(messages []message.Message) *mockStateWriter {
	return &mockStateWriter{
		messages: messages,
		state:    make(map[string]any),
	}
}

func (m *mockStateWriter) Get(key string) any {
	return m.state[key]
}

func (m *mockStateWriter) GetAll() map[string]any {
	return m.state
}

func (m *mockStateWriter) Set(key string, value any) {
	m.state[key] = value
}

func (m *mockStateWriter) EventsSnapshot() []graph.Event {
	events := make([]graph.Event, len(m.messages))
	for i, msg := range m.messages {
		events[i] = *graph.NewEvent(msg, "", "mock")
	}
	return events
}

func (m *mockStateWriter) AggregatesSnapshot() map[string]any {
	return make(map[string]any)
}

func (m *mockStateWriter) Aggregate(name string, value any) error {
	return nil
}

// HandleModelErrors provides fallback responses when model calls fail
func HandleModelErrors(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
	// Log the error
	fmt.Printf("⚠ Model error: %v\n", err)

	// Provide graceful fallback response
	return message.NewAIMessageFromText("I apologize, but I'm experiencing technical difficulties. Please try again in a moment."), nil
}

func main() {
	// Create callback manager
	manager := callbacks.NewManager()

	// Register safety guardrails
	manager.RegisterBeforeModel(BlockUnsafeContent)
	manager.RegisterAfterModel(FilterPII)
	manager.RegisterOnModelError(HandleModelErrors)

	// Set up caching
	cache := make(map[string]message.Message)
	manager.RegisterBeforeModel(CacheResponses(cache))
	manager.RegisterAfterModel(StoreInCache(cache))

	fmt.Println("=== AgentMesh Callback System Demo ===")
	fmt.Println()

	// Test 1: Normal message (should pass all checks)
	fmt.Println("Test 1: Normal message")
	messages1 := []message.Message{
		message.NewHumanMessageFromText("What is the weather today?"),
	}

	result1, err := manager.ExecuteBeforeModel(context.Background(), newMockState(messages1))
	if err != nil {
		log.Printf("BeforeModel error: %v\n", err)
	} else if result1 != nil {
		fmt.Println("Short-circuited with cached response")
	} else {
		fmt.Println("Passed BeforeModel checks - would call model")

		// Simulate model response
		modelResponse := message.NewAIMessageFromText("The weather is sunny and 72°F.")
		finalResponse, err := manager.ExecuteAfterModel(context.Background(), newMockState(messages1), modelResponse)
		if err != nil {
			log.Printf("AfterModel error: %v\n", err)
		} else if finalResponse != nil {
			fmt.Printf("Final response (transformed): %v\n", finalResponse)
		} else {
			fmt.Printf("Final response (original): %v\n", modelResponse)
		}
	}

	fmt.Println()

	// Test 2: Unsafe content (should be blocked)
	fmt.Println("Test 2: Unsafe content (should be blocked)")
	messages2 := []message.Message{
		message.NewHumanMessageFromText("How can I hack into a system?"),
	}

	result2, err := manager.ExecuteBeforeModel(context.Background(), newMockState(messages2))
	if err != nil {
		fmt.Printf("✓ Blocked: %v\n", err)
	} else if result2 != nil {
		fmt.Println("Short-circuited with response")
	} else {
		fmt.Println("Passed checks (unexpected)")
	}

	fmt.Println()

	// Test 3: Response with PII (should be filtered)
	fmt.Println("Test 3: Response with PII filtering")
	messages3 := []message.Message{
		message.NewHumanMessageFromText("Show me an example"),
	}

	_, err = manager.ExecuteBeforeModel(context.Background(), newMockState(messages3))
	if err != nil {
		log.Printf("BeforeModel error: %v\n", err)
	} else {
		// Simulate model response with PII
		modelResponse := message.NewAIMessageFromText("Sure! Contact us at support@example.com or call 123-45-6789.")
		finalResponse, err := manager.ExecuteAfterModel(context.Background(), newMockState(messages3), modelResponse)
		if err != nil {
			log.Printf("AfterModel error: %v\n", err)
		} else if finalResponse != nil {
			parts := finalResponse.Parts()
			if len(parts) > 0 {
				if textPart, ok := parts[0].(message.TextPart); ok {
					fmt.Printf("✓ PII filtered response: %s\n", textPart.Text)
				}
			}
		}
	}

	fmt.Println()

	// Test 4: Cache hit (repeat first message)
	fmt.Println("Test 4: Cache hit test")
	result4, err := manager.ExecuteBeforeModel(context.Background(), newMockState(messages1))
	if err != nil {
		log.Printf("BeforeModel error: %v\n", err)
	} else if result4 != nil {
		fmt.Println("✓ Cache returned response - model call skipped")
	} else {
		fmt.Println("Cache miss (unexpected)")
	}

	fmt.Println("\n=== Demo Complete ===")
}
