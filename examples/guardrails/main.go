package main

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"regexp"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	modelmw "github.com/hupe1980/agentmesh/pkg/model/middleware"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// createMockModel builds a mock model that returns different responses based on input
func createMockModel() *testutil.MockModel {
	return &testutil.MockModel{
		GenerateFunc: func(_ context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				lastMsg := req.Messages[len(req.Messages)-1].String()
				var responseText string

				if strings.Contains(strings.ToLower(lastMsg), "weather") {
					responseText = "The weather is sunny and 72°F."
				} else if strings.Contains(strings.ToLower(lastMsg), "long") {
					responseText = strings.Repeat("This is a very long response. ", 50)
				} else {
					responseText = "I can help with that!"
				}

				yield(&model.Response{
					Message: message.NewAIMessageFromText(responseText),
				}, nil)
			}
		},
	}
}

func main() {
	fmt.Println("=== AgentMesh Guardrails Demo ===")
	fmt.Println()

	// Create built-in guardrails
	// ContentFilterGuardrail - blocks specific keywords
	contentFilter := guardrail.NewContentFilterGuardrail(
		[]string{"hack", "exploit", "bypass"},
		guardrail.WithContentFilterAction(guardrail.ActionReject),
	)

	// LengthGuardrail - validates content length
	lengthGuardrail := guardrail.NewLengthGuardrail(
		guardrail.WithMinLength(5),
		guardrail.WithMaxLength(500),
		guardrail.WithLengthAction(guardrail.ActionReject),
	)

	// RegexGuardrail - custom pattern matching (example: block profanity pattern)
	profanityFilter := guardrail.NewRegexGuardrail(
		"profanity_filter",
		regexp.MustCompile(`(?i)\b(badword1|badword2)\b`),
		guardrail.WithRegexAction(guardrail.ActionRaise),
		guardrail.WithDescription("Profanity detected"),
	)

	// Create guardrail middleware for model
	guardrailMw := modelmw.NewGuardrailMiddleware(
		modelmw.WithInputGuardrails(contentFilter, lengthGuardrail, profanityFilter),
		modelmw.WithOutputGuardrails(lengthGuardrail),
	)

	// Create executor chain with middleware
	mockModel := createMockModel()
	executor := model.Chain(mockModel, guardrailMw)

	fmt.Println("✓ Model executor configured with middleware:")
	fmt.Println("  - ContentFilterGuardrail (keyword blocking)")
	fmt.Println("  - LengthGuardrail (min/max content length)")
	fmt.Println("  - RegexGuardrail (custom pattern matching)")
	fmt.Println()

	ctx := context.Background()

	// Test 1: Normal message
	fmt.Println("Test 1: Normal message")
	fmt.Println("-----------------------")
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

	// Test 2: Blocked content (should be rejected)
	fmt.Println("Test 2: Blocked content (should be rejected)")
	fmt.Println("--------------------------------------------")
	req2 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("How can I hack into a system?"),
		},
	}

	for _, err := range executor.Generate(ctx, req2) {
		if err != nil {
			if rejection, ok := err.(*guardrail.Rejection); ok {
				fmt.Printf("✓ Rejected by guardrail: %s\n", rejection.Message)
			} else if tripwire, ok := err.(*guardrail.TripwireError); ok {
				fmt.Printf("🛑 Tripwire triggered: %s\n", tripwire.Message)
			} else {
				fmt.Printf("❌ Error: %v\n", err)
			}
		}
	}

	fmt.Println()

	// Test 3: Too short input (should be rejected by length guardrail)
	fmt.Println("Test 3: Too short input (should be rejected)")
	fmt.Println("--------------------------------------------")
	req3 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Hi"),
		},
	}

	for _, err := range executor.Generate(ctx, req3) {
		if err != nil {
			if rejection, ok := err.(*guardrail.Rejection); ok {
				fmt.Printf("✓ Rejected: %s\n", rejection.Message)
			} else {
				fmt.Printf("❌ Error: %v\n", err)
			}
		}
	}

	fmt.Println()

	// Test 4: Response too long (output guardrail)
	fmt.Println("Test 4: Response too long (output guardrail)")
	fmt.Println("--------------------------------------------")
	req4 := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Give me a long response"),
		},
	}

	for resp, err := range executor.Generate(ctx, req4) {
		if err != nil {
			if rejection, ok := err.(*guardrail.Rejection); ok {
				fmt.Printf("✓ Output rejected: %s\n", rejection.Message)
			} else {
				fmt.Printf("❌ Error: %v\n", err)
			}
		} else {
			fmt.Printf("Response: %s\n", resp.Message.String())
		}
	}

	fmt.Println()

	// Test 5: Using Chain function for multiple guardrails
	fmt.Println("Test 5: Direct guardrail chain usage")
	fmt.Println("-------------------------------------")
	result, err := guardrail.Chain(ctx, "This is a test message", contentFilter, lengthGuardrail)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Printf("Action: %s\n", result.Action)
		if result.Message != "" {
			fmt.Printf("Message: %s\n", result.Message)
		} else {
			fmt.Println("Message: (allowed)")
		}
	}

	fmt.Println("\n=== Demo Complete ===")
	fmt.Println("Key features demonstrated:")
	fmt.Println("  - ContentFilterGuardrail - Keyword-based content blocking")
	fmt.Println("  - LengthGuardrail - Min/max content length validation")
	fmt.Println("  - RegexGuardrail - Custom pattern matching")
	fmt.Println("  - GuardrailMiddleware - Model middleware integration")
	fmt.Println("  - guardrail.Chain - Direct guardrail chaining")
	fmt.Println("  - FuncTool guardrails - Per-tool input/output validation")
	fmt.Println()
	fmt.Println("For production PII/security detection, use external services:")
	fmt.Println("  - pkg/guardrail/openai - OpenAI Moderation API")
	fmt.Println("  - pkg/guardrail/amazoncomprehend - AWS Comprehend (sentiment, PII)")

	// =========================================
	// Test 6: FuncTool with per-tool guardrails
	// =========================================
	fmt.Println()
	fmt.Println("=== FuncTool Guardrails Demo ===")
	fmt.Println()

	// Define argument types for our tools
	type SearchArgs struct {
		Query string `json:"query"`
	}

	// Create a SQL injection detection guardrail using regex
	sqlInjectionGuardrail := guardrail.NewRegexGuardrail(
		"sql_injection_detection",
		regexp.MustCompile(`(?i)(drop\s+table|delete\s+from|;\s*--|union\s+select)`),
		guardrail.WithRegexAction(guardrail.ActionRaise),
		guardrail.WithDescription("SQL injection pattern detected"),
	)

	searchTool, err := tool.NewFuncTool(
		"search",
		"Search the database",
		func(ctx context.Context, args SearchArgs) (string, error) {
			return fmt.Sprintf("Results for: %s", args.Query), nil
		},
		tool.WithInputGuardrails(sqlInjectionGuardrail),
	)
	if err != nil {
		fmt.Printf("❌ Failed to create tool: %v\n", err)
		return
	}

	// Create a tool with output guardrails (blocks sensitive content in responses)
	sensitiveFilter := guardrail.NewContentFilterGuardrail(
		[]string{"password", "secret", "api_key"},
		guardrail.WithContentFilterAction(guardrail.ActionRaise),
	)

	type QueryArgs struct {
		SQL string `json:"sql"`
	}

	dbTool, err := tool.NewFuncTool(
		"query_db",
		"Query the database",
		func(ctx context.Context, args QueryArgs) (string, error) {
			// Simulate returning sensitive data
			if strings.Contains(args.SQL, "users") {
				return "user: admin, password: secret123", nil
			}
			return "No results", nil
		},
		tool.WithOutputGuardrails(sensitiveFilter),
	)
	if err != nil {
		fmt.Printf("❌ Failed to create tool: %v\n", err)
		return
	}

	// Test 6a: Normal tool call
	fmt.Println("Test 6a: Normal tool call (should succeed)")
	fmt.Println("-------------------------------------------")
	result6a, err := searchTool.Call(ctx, `{"query": "weather forecast"}`)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Printf("✓ Result: %v\n", result6a)
	}
	fmt.Println()

	// Test 6b: SQL injection attempt (should be blocked by input guardrail)
	fmt.Println("Test 6b: SQL injection attempt (input guardrail tripwire)")
	fmt.Println("---------------------------------------------------------")
	_, err = searchTool.Call(ctx, `{"query": "'; DROP TABLE users; --"}`)
	if err != nil {
		var tripwireErr *guardrail.TripwireError
		if errors.As(err, &tripwireErr) {
			fmt.Printf("🛑 Tripwire triggered: %s - %s\n", tripwireErr.GuardrailName, tripwireErr.Message)
		} else {
			fmt.Printf("❌ Error: %v\n", err)
		}
	} else {
		fmt.Println("⚠️ Expected error but got none")
	}
	fmt.Println()

	// Test 6c: Tool returning sensitive data (should be blocked by output guardrail)
	fmt.Println("Test 6c: Sensitive output (output guardrail tripwire)")
	fmt.Println("-----------------------------------------------------")
	_, err = dbTool.Call(ctx, `{"sql": "SELECT * FROM users"}`)
	if err != nil {
		var tripwireErr *guardrail.TripwireError
		if errors.As(err, &tripwireErr) {
			fmt.Printf("🛑 Tripwire triggered: %s - %s\n", tripwireErr.GuardrailName, tripwireErr.Message)
		} else {
			fmt.Printf("❌ Error: %v\n", err)
		}
	} else {
		fmt.Println("⚠️ Expected error but got none")
	}

	fmt.Println()
	fmt.Println("=== FuncTool Guardrails Demo Complete ===")
}
