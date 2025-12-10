package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/schema"
)

// MovieReview represents a structured movie review with rating and analysis
type MovieReview struct {
	Title      string   `json:"title" jsonschema:"required,description=Movie title"`
	Rating     int      `json:"rating" jsonschema:"required,minimum=1,maximum=5,description=Rating from 1 to 5 stars"`
	Summary    string   `json:"summary" jsonschema:"required,description=Brief review summary"`
	Pros       []string `json:"pros" jsonschema:"description=Positive aspects of the movie"`
	Cons       []string `json:"cons" jsonschema:"description=Negative aspects of the movie"`
	Recommend  bool     `json:"recommend" jsonschema:"required,description=Would you recommend this movie"`
	Confidence float64  `json:"confidence" jsonschema:"required,minimum=0,maximum=1,description=Confidence in this analysis (0-1)"`
}

// ProductAnalysis represents structured product analysis with categorization
type ProductAnalysis struct {
	ProductName string   `json:"product_name" jsonschema:"required,description=Name of the product"`
	Category    string   `json:"category" jsonschema:"required,enum=electronics|clothing|food|home|other,description=Product category"`
	Price       float64  `json:"price" jsonschema:"required,minimum=0,description=Price in USD"`
	Features    []string `json:"features" jsonschema:"description=Key product features"`
	Rating      float64  `json:"rating" jsonschema:"required,minimum=0,maximum=5,description=Overall rating out of 5"`
	InStock     bool     `json:"in_stock" jsonschema:"required,description=Whether product is in stock"`
}

func main() {
	ctx := context.Background()

	// Initialize OpenAI model
	mdl := openai.NewModel(
		openai.WithModel("gpt-4o"),
		openai.WithTemperature(0.7),
	)

	fmt.Println("🎬 Structured Output Example - AgentMesh")
	fmt.Println("=========================================")
	fmt.Println()

	// Example 1: Direct model usage with OutputSchema
	fmt.Println("📋 Example 1: Direct Model Usage")
	fmt.Println("----------------------------------")
	if err := directModelExample(ctx, mdl); err != nil {
		log.Printf("Example 1 failed: %v", err)
	}
	fmt.Println()

	// Example 2: Using model.LastStructured helper
	fmt.Println("🔍 Example 2: LastStructured Helper")
	fmt.Println("------------------------------------")
	if err := lastStructuredExample(ctx, mdl); err != nil {
		log.Printf("Example 2 failed: %v", err)
	}
	fmt.Println()

	// Example 3: Using ReActAgent with structured output
	fmt.Println("🤖 Example 3: ReActAgent with Structured Output")
	fmt.Println("------------------------------------------------")
	if err := reactAgentExample(ctx, mdl); err != nil {
		log.Printf("Example 3 failed: %v", err)
	}
	fmt.Println()

	fmt.Println("✅ All examples completed!")
}

// directModelExample demonstrates using OutputSchema directly with model.Request
func directModelExample(ctx context.Context, mdl model.Model) error {
	// Create OutputSchema from struct
	outputSchema, err := schema.NewOutputSchema("movie_review", MovieReview{},
		schema.WithStrict(true),
		schema.WithDescription("Structured movie review with rating and analysis"),
	)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	fmt.Printf("Schema Name: %s\n", outputSchema.Name)
	fmt.Printf("Strict Mode: %v\n", outputSchema.Strict)
	fmt.Printf("Description: %s\n\n", outputSchema.Description)

	// Create request with OutputSchema
	req := &model.Request{
		Messages: []message.Message{
			message.NewSystemMessageFromText("You are a professional movie critic. Analyze the movie and provide a structured review."),
			message.NewHumanMessageFromText("Review the movie 'Inception' (2010) directed by Christopher Nolan. A science fiction thriller about dream infiltration."),
		},
		OutputSchema: &outputSchema,
	}

	// Generate response with structured output
	fmt.Println("Generating structured movie review...")
	resp, err := model.Last(mdl.Generate(ctx, req))
	if err != nil {
		return fmt.Errorf("generation failed: %w", err)
	}

	// Parse the structured output
	var review MovieReview
	if err := json.Unmarshal([]byte(resp.Message.String()), &review); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Display the structured output
	fmt.Println("\n📊 Structured Review:")
	fmt.Printf("  Title: %s\n", review.Title)
	fmt.Printf("  Rating: %d/5 ⭐\n", review.Rating)
	fmt.Printf("  Summary: %s\n", review.Summary)
	fmt.Printf("  Pros: %v\n", review.Pros)
	fmt.Printf("  Cons: %v\n", review.Cons)
	fmt.Printf("  Recommend: %v\n", review.Recommend)
	fmt.Printf("  Confidence: %.2f\n", review.Confidence)

	return nil
}

// lastStructuredExample demonstrates using the LastStructured helper
func lastStructuredExample(ctx context.Context, mdl model.Model) error {
	// Create schema
	outputSchema, err := schema.NewOutputSchema("quick_analysis", ProductAnalysis{},
		schema.WithStrict(true),
	)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Create request
	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("Analyze this product: 'iPhone 15 Pro - A high-end smartphone with titanium design, A17 Pro chip, and advanced camera system. Price: $999. Currently in stock with 4.5 star rating.'"),
		},
		OutputSchema: &outputSchema,
	}

	fmt.Println("Using model.LastStructured helper...")

	// Use the LastStructured helper to directly get typed result
	analysis, err := model.LastStructured[ProductAnalysis](mdl.Generate(ctx, req))
	if err != nil {
		return fmt.Errorf("failed to get structured output: %w", err)
	}

	// Display the result
	fmt.Println("\n�� Product Analysis (via LastStructured):")
	fmt.Printf("  Product: %s\n", analysis.ProductName)
	fmt.Printf("  Category: %s\n", analysis.Category)
	fmt.Printf("  Price: $%.2f\n", analysis.Price)
	fmt.Printf("  Features: %v\n", analysis.Features)
	fmt.Printf("  Rating: %.1f/5 ⭐\n", analysis.Rating)
	fmt.Printf("  In Stock: %v\n", analysis.InStock)

	return nil
}

// AgentResponse represents structured agent response with reasoning
type AgentResponse struct {
	Reasoning  string  `json:"reasoning" jsonschema:"required,description=Step-by-step reasoning process"`
	Answer     string  `json:"answer" jsonschema:"required,description=Final answer"`
	Confidence float64 `json:"confidence" jsonschema:"required,minimum=0,maximum=1,description=Confidence in the answer"`
}

// reactAgentExample demonstrates using ReActAgent with OutputSchema
func reactAgentExample(ctx context.Context, mdl model.Model) error {
	// Create schema for agent responses
	outputSchema, err := schema.NewOutputSchema("agent_response", AgentResponse{},
		schema.WithStrict(true),
		schema.WithDescription("ReAct agent structured response with reasoning"),
	)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	fmt.Printf("Creating ReActAgent with schema: %s\n", outputSchema.Name)

	// Create ReActAgent with OutputSchema
	reactAgent, err := agent.NewReAct(mdl,
		agent.WithOutputSchema(&outputSchema),
		agent.WithSystemPrompt("You are a helpful assistant. Think step-by-step and provide structured responses with your reasoning."),
	)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	fmt.Println("\nAsking agent: What is the capital of France and why is it significant?")

	// Run the agent with a question
	input := []message.Message{
		message.NewHumanMessageFromText("What is the capital of France and why is it significant?"),
	}

	// Get the response using graph.LastStructured helper
	response, err := graph.LastStructured[AgentResponse](reactAgent.Run(ctx, input))
	if err != nil {
		return fmt.Errorf("agent execution failed: %w", err)
	}

	// Display the structured response
	fmt.Println("\n📊 Agent Response:")
	fmt.Printf("  Reasoning: %s\n", response.Reasoning)
	fmt.Printf("  Answer: %s\n", response.Answer)
	fmt.Printf("  Confidence: %.2f\n", response.Confidence)

	return nil
}
