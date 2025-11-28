// Package main demonstrates the Model Router feature for intelligent model selection.
// This example shows:
//   - Cost-based routing: simple queries to cheap models, complex to expensive
//   - Capability-based routing: vision requests to vision-capable models
//   - Fallback routing: circuit breaker pattern for resilience
//   - Composite routing: chaining multiple routing strategies
//
// Run: OPENAI_API_KEY=sk-... go run main.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
)

func main() {
	// Validate API key is set
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	// Example 1: Cost-Based Router
	fmt.Println("=== Cost-Based Router Demo ===")
	costBasedDemo(ctx)

	// Example 2: Composite Router with Conditional Logic
	fmt.Println("\n=== Conditional Router Demo ===")
	conditionalDemo(ctx)
}

func costBasedDemo(ctx context.Context) {
	// Create cheap and expensive models
	cheapModel := openai.NewModel(openai.WithModel("gpt-4o-mini"))
	expensiveModel := openai.NewModel(openai.WithModel("gpt-4o"))

	// Create cost-based router with 30% complexity threshold
	router := model.NewCostBasedRouter(cheapModel, expensiveModel,
		model.WithComplexityThreshold(0.3),
	)

	// Wrap router as a Model for transparent usage
	routedModel := model.NewRoutedModel(router,
		model.WithRouteCallback(func(ctx context.Context, req *model.Request, selected model.Model) {
			if selected == cheapModel {
				fmt.Println("→ Routed to: gpt-4o-mini (cheap)")
			} else {
				fmt.Println("→ Routed to: gpt-4o (expensive)")
			}
		}),
	)

	// Test with simple query (should route to cheap)
	fmt.Println("Query: What is 2+2?")
	simpleReq := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("What is 2+2?"),
		},
	}
	resp, err := model.Last(routedModel.Generate(ctx, simpleReq))
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("Response: %s\n\n", message.Stringify(resp.Message))
	}

	// Test with complex query (should route to expensive)
	fmt.Println("Query: Analyze microservices vs monolithic architecture...")
	complexReq := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Analyze the trade-offs between microservices and monolithic architecture. Compare deployment complexity, scalability, and team organization impacts."),
		},
	}
	resp, err = model.Last(routedModel.Generate(ctx, complexReq))
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		text := message.Stringify(resp.Message)
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		fmt.Printf("Response: %s\n", text)
	}
}

func conditionalDemo(ctx context.Context) {
	// Create models
	cheapModel := openai.NewModel(openai.WithModel("gpt-4o-mini"))
	expensiveModel := openai.NewModel(openai.WithModel("gpt-4o"))

	// Use conditional routing - expensive for "important" queries
	conditionalRouter := model.NewConditionalRouter(
		func(ctx context.Context, req *model.Request) bool {
			text := message.Stringify(req.Messages[0])
			return strings.Contains(strings.ToLower(text), "important")
		},
		model.NewStaticRouter(expensiveModel),
		model.NewStaticRouter(cheapModel),
	)

	routedModel := model.NewRoutedModel(conditionalRouter,
		model.WithRouteCallback(func(ctx context.Context, req *model.Request, selected model.Model) {
			if selected == expensiveModel {
				fmt.Println("→ Important request, using premium model")
			} else {
				fmt.Println("→ Normal request, using standard model")
			}
		}),
	)

	// Test normal query
	fmt.Println("Query: Hello")
	normalReq := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Hello"),
		},
	}
	resp, err := model.Last(routedModel.Generate(ctx, normalReq))
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("Response: %s\n\n", message.Stringify(resp.Message))
	}

	// Test important query
	fmt.Println("Query: This is important - what is AI?")
	importantReq := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("This is important - what is AI in one sentence?"),
		},
	}
	resp, err = model.Last(routedModel.Generate(ctx, importantReq))
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		fmt.Printf("Response: %s\n", message.Stringify(resp.Message))
	}
}
