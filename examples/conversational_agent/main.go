// Package main demonstrates the Conversational agent pattern.
// This example shows how to wrap any agent with long-term memory
// for multi-turn conversations with context awareness.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/agent"
	embeddingai "github.com/hupe1980/agentmesh/pkg/embedding/openai"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/memory"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// WeatherArgs defines the JSON schema for the weather tool.
type WeatherArgs struct {
	Location string `json:"location" jsonschema:"description=The city to get weather for"`
}

func main() {
	ctx := context.Background()

	// Configure debug logger to see memory recall details
	logger := logging.NewSlogLogger(logging.LogLevelWarn, logging.LogFormatText, false)
	ctx = logging.WithLogger(ctx, logger)

	// Validate API key
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Create OpenAI model
	model := openai.NewModel()

	// Create embedder for semantic memory
	embedder := embeddingai.NewEmbedder()

	// Define a simple tool
	weatherTool, err := tool.NewFuncTool(
		"get_weather",
		"Get current weather for a location",
		func(ctx context.Context, args WeatherArgs) (string, error) {
			return fmt.Sprintf("Weather in %s: Sunny, 72°F", args.Location), nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create a ReAct agent as the base
	reactAgent, err := agent.NewReAct(model,
		agent.WithTools(weatherTool),
		agent.WithMaxIterations(5),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Create vector memory for semantic search
	mem := memory.NewVectorMemory(embedder)

	sessionID := "user-123-session"

	// Prefill memory with historical conversation (simulating past sessions)
	// This will be recalled via LONG-TERM memory (semantic search)
	fmt.Println("=== Prefilling memory with historical context ===")
	historicalMessages := []message.Message{
		message.NewHumanMessageFromText("What's my favorite programming language?"),
		message.NewAIMessageFromText("You mentioned that Go is your favorite programming language because of its simplicity and performance."),
		message.NewHumanMessageFromText("I also love Italian food, especially pasta."),
		message.NewAIMessageFromText("That's great! Italian cuisine is wonderful. Pasta is a classic choice."),
	}
	if err := mem.Store(ctx, sessionID, historicalMessages); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Stored 4 historical messages about programming and food preferences.")
	fmt.Println()

	// Wrap the ReAct agent with conversational memory
	// Uses dual-memory approach:
	// - Short-term: Last N messages for immediate context (recency-based)
	// - Long-term: Semantically similar messages from history (relevance-based)
	chatAgent, err := agent.NewConversational(
		reactAgent,
		mem,
		agent.WithShortTermMessages(10),   // Last 10 messages for immediate context
		agent.WithLongTermMessages(5),     // 5 semantically similar messages
		agent.WithMinSimilarityScore(0.3), // Lower threshold for long-term recall
		agent.WithFailOnStoreError(false), // Don't fail if memory store fails
	)
	if err != nil {
		log.Fatal(err)
	}

	// Turn 1: Introduce current context (will be stored for SHORT-TERM recall)
	fmt.Println("=== Turn 1: Setting up current context ===")
	fmt.Println("(This will be recalled via SHORT-TERM memory in Turn 2)")
	messages1 := []message.Message{
		message.NewHumanMessageFromText("My name is Alice and I live in San Francisco."),
	}

	for msg, err := range chatAgent.Run(ctx, messages1,
		graph.WithInitialValue(agent.SessionIDKey, sessionID),
	) {
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Agent: %s\n", message.Stringify(msg))
	}

	// Turn 2: Use tool - should recall Turn 1 via SHORT-TERM memory
	fmt.Println("\n=== Turn 2: Using tool (SHORT-TERM memory) ===")
	fmt.Println("(Agent recalls 'San Francisco' from Turn 1 via recent message history)")
	messages2 := []message.Message{
		message.NewHumanMessageFromText("What's the weather like here?"),
	}

	for msg, err := range chatAgent.Run(ctx, messages2,
		graph.WithInitialValue(agent.SessionIDKey, sessionID),
	) {
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Agent: %s\n", message.Stringify(msg))
	}

	// Turn 3: Ask about programming - should recall from LONG-TERM memory (prefilled history)
	fmt.Println("\n=== Turn 3: Recalling from LONG-TERM memory ===")
	fmt.Println("(Agent recalls programming preference from historical context via semantic search)")
	messages3 := []message.Message{
		message.NewHumanMessageFromText("What programming language do I like?"),
	}

	for msg, err := range chatAgent.Run(ctx, messages3,
		graph.WithInitialValue(agent.SessionIDKey, sessionID),
	) {
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Agent: %s\n", message.Stringify(msg))
	}

	// Turn 4: Ask about food - also from LONG-TERM memory
	fmt.Println("\n=== Turn 4: Another LONG-TERM memory recall ===")
	fmt.Println("(Agent recalls food preference from historical context)")
	messages4 := []message.Message{
		message.NewHumanMessageFromText("What kind of food do I enjoy?"),
	}

	for msg, err := range chatAgent.Run(ctx, messages4,
		graph.WithInitialValue(agent.SessionIDKey, sessionID),
	) {
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Agent: %s\n", message.Stringify(msg))
	}

	// Turn 5: Combine both - should use BOTH memory types
	fmt.Println("\n=== Turn 5: Combined recall (SHORT + LONG-TERM) ===")
	fmt.Println("(Agent uses both recent context AND historical semantic search)")
	messages5 := []message.Message{
		message.NewHumanMessageFromText("Tell me my name, where I live, what programming language I like, and what food I enjoy."),
	}

	for msg, err := range chatAgent.Run(ctx, messages5,
		graph.WithInitialValue(agent.SessionIDKey, sessionID),
	) {
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Agent: %s\n", message.Stringify(msg))
	}

	fmt.Println("\n=== Conversation complete! ===")
	fmt.Println("The agent used DUAL-MEMORY approach:")
	fmt.Println("  - SHORT-TERM: Recent messages (name, location, weather)")
	fmt.Println("  - LONG-TERM:  Semantic search (programming, food preferences)")
}
