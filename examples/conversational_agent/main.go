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

	// Wrap the ReAct agent with conversational memory
	chatAgent, err := agent.NewConversational(
		reactAgent,
		mem,
		agent.WithMaxRecallMessages(10),   // Recall up to 10 relevant messages
		agent.WithMinSimilarityScore(0.3), // Only recall if similarity > 0.3
		agent.WithFailOnStoreError(false), // Don't fail if memory store fails
	)
	if err != nil {
		log.Fatal(err)
	}

	sessionID := "user-123-session"

	// First turn: introduce context
	fmt.Println("=== Turn 1: Setting up context ===")
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

	// Second turn: use weather tool
	fmt.Println("\n=== Turn 2: Using tool ===")
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

	// Third turn: recall from memory
	fmt.Println("\n=== Turn 3: Recalling from memory ===")
	messages3 := []message.Message{
		message.NewHumanMessageFromText("What is my name and where do I live?"),
	}

	for msg, err := range chatAgent.Run(ctx, messages3,
		graph.WithInitialValue(agent.SessionIDKey, sessionID),
	) {
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Agent: %s\n", message.Stringify(msg))
	}

	fmt.Println("\n=== Conversation complete! ===")
	fmt.Println("The agent remembered context from earlier turns using semantic memory.")
}
