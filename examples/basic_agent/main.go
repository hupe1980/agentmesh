// Package main demonstrates the fundamentals of building a ReAct (Reasoning + Acting) agent
// with tool integration. This example shows:
//   - Creating function-based tools with automatic JSON schema generation
//   - Building a ReAct agent that reasons about when to use tools
//   - Executing the agent with conversational messages
//   - Processing the complete message transcript including tool calls
//
// Run: OPENAI_API_KEY=sk-... go run main.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/tool"
)

// weatherArgs defines the JSON schema for the weather tool's parameters.
// AgentMesh automatically generates JSON schema from this struct for the LLM.
type weatherArgs struct {
	Location string `json:"location" jsonschema:"description=The city or location to get weather for"`
}

// mockWeatherLookup simulates a weather API call.
// In production, this would query a real weather service.
func mockWeatherLookup(_ context.Context, args weatherArgs) (map[string]any, error) {
	location := strings.TrimSpace(args.Location)
	if location == "" {
		return map[string]any{"error": "missing location"}, nil
	}

	// Return mock weather data
	return map[string]any{
		"location":      location,
		"conditions":    "Partly cloudy",
		"temperature_c": 21.5,
	}, nil
}

func main() {
	// Validate API key is set
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY environment variable is required")
	}

	// Create a tool from a Go function
	// NewFuncTool automatically:
	//   - Generates JSON schema from weatherArgs struct
	//   - Handles argument parsing and validation
	//   - Manages error handling
	weatherTool, err := tool.NewFuncTool(
		"get_weather",
		"Lookup the current weather for a given city",
		mockWeatherLookup,
	)
	if err != nil {
		log.Fatalf("failed to create weather tool: %v", err)
	}

	// Build a ReAct agent that:
	//   - Uses GPT to reason about tool usage
	//   - Automatically calls tools when needed
	//   - Iterates until reaching a final answer
	compiled, err := agent.NewReActAgent(
		openai.NewModel(),
		agent.WithTools(weatherTool),
	)
	if err != nil {
		log.Fatalf("failed to create agent: %v", err)
	}

	// Prepare conversation messages
	system := message.NewSystemMessageFromText(
		"You are a helpful assistant. Call the weather tool when a user asks about weather conditions.",
	)
	human := message.NewHumanMessageFromText("What's the weather like in Berlin right now?")

	// Execute the agent
	// The agent will:
	//   1. Analyze the question
	//   2. Decide to call get_weather tool
	//   3. Process the tool result
	//   4. Generate a natural language response

	// Display the complete conversation transcript
	fmt.Println("=== Agent Transcript ===")
	fmt.Println()
	i := 0
	for evt, err := range compiled.Run(context.Background(), []message.Message{system, human}) {
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}

		// Each event is now a Message directly
		if evt == nil {
			continue
		}

		fmt.Printf("[%d] %s\n", i+1, evt.Type())

		switch m := evt.(type) {
		case *message.AIMessage:
			// Display AI's reasoning and responses
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Printf("    💭 %s\n", text.Text)
				}
			}
			// Show tool calls made by the AI
			if len(m.ToolCalls) > 0 {
				fmt.Printf("    🔧 Tool calls: %v\n", m.ToolCalls)
			}

		case *message.ToolMessage:
			// Display tool execution results
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Printf("    ⚙️  Tool result: %s\n", text.Text)
				}
			}

		default:
			// Display other message types (system, human)
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					fmt.Printf("    📝 %s\n", text.Text)
				}
			}
		}
		fmt.Println()
		i++
	}
}
