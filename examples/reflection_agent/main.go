package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
)

// This example demonstrates the ReAct agent with reflection capabilities.
// The agent will:
// 1. Generate an initial answer
// 2. Critique its own answer through reflection
// 3. Refine the answer based on the critique
// 4. Repeat until max reflections reached or quality threshold met

func main() {
	// Validate API key
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	fmt.Println("=== ReAct Agent with Reflection Example ===")
	fmt.Println()

	// Create model for the base agent
	mdl := openai.NewModel()

	// Create a ReAct agent (could be any agent - RAG, Supervisor, etc.)
	reactAgent, err := agent.NewReAct(mdl,
		agent.WithSystemPrompt("You are a helpful assistant that provides clear, accurate answers."),
	)
	if err != nil {
		log.Fatalf("failed to create base agent: %v", err)
	}

	// Wrap the agent with reflection capabilities
	// This works with ANY agent type!
	wrappedAgent, err := agent.NewReflection(
		reactAgent,
		mdl,                                  // Can use same or different model for reflection
		agent.WithReflectionMaxIterations(2), // Max 2 reflection iterations
	)
	if err != nil {
		log.Fatalf("failed to create reflection agent: %v", err)
	}

	// Test question that benefits from refinement
	question := "Explain what recursion is in programming and provide a simple example."

	fmt.Printf("Question: %s\n\n", question)

	input := []message.Message{
		message.NewHumanMessageFromText(question),
	}

	fmt.Println("Agent processing with reflection...")
	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println()

	iteration := 0
	for evt, err := range wrappedAgent.Run(ctx, input) {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}

		if evt == nil {
			continue
		}

		switch m := evt.(type) {
		case *message.AIMessage:
			iteration++
			fmt.Printf("--- Iteration %d ---\n", iteration)
			fmt.Printf("%s\n\n", message.Stringify(m))

		case *message.SystemMessage:
			// This is the reflection feedback
			content := message.Stringify(m)
			if strings.Contains(content, "Reflection on your previous answer") {
				fmt.Println("🔍 Reflection Critique:")
				fmt.Println(strings.Repeat("-", 60))

				// Extract and display the critique (between "Reflection on..." and "Please provide...")
				lines := strings.Split(content, "\n")
				inCritique := false
				for _, line := range lines {
					if strings.Contains(line, "Reflection on your previous answer:") {
						inCritique = true
						continue
					}
					if strings.Contains(line, "Please provide an improved answer") {
						break
					}
					if inCritique && strings.TrimSpace(line) != "" {
						fmt.Println(line)
					}
				}

				fmt.Println(strings.Repeat("-", 60))
				fmt.Println("   (Agent is now generating improved answer...)")
				fmt.Println()
			}
		}
	}

	fmt.Println("=" + strings.Repeat("=", 60))
	fmt.Println()

	// Get final result
	lastMsg, err := graph.Last(wrappedAgent.Run(ctx, input))
	if err != nil {
		log.Fatalf("failed to get final result: %v", err)
	}

	fmt.Println("Final Answer:")
	fmt.Println(message.Stringify(lastMsg))
	fmt.Println()

	fmt.Println("✅ Reflection example completed!")
}
