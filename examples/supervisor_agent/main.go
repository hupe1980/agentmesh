// Package main demonstrates supervisor-based agent coordination using the Supervisor template.
// This simplified example shows:
//   - Using agent.NewSupervisor() for easy multi-agent setup
//   - Creating specialized worker agents with system prompts
//   - Automatic routing to appropriate specialists
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
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
)

func main() {
	// Validate API key
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	// Test queries
	queries := []string{
		"What is the derivative of x^2 + 3x + 5?",
		"Who was the first president of the United States and when did he serve?",
		"Write a Python function to calculate fibonacci numbers",
		"Integrate x^3 + 2x from 0 to 5",
	}

	// Execute queries
	for i, query := range queries {
		fmt.Println()
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("Query %d: %s\n", i+1, query)
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println()

		// Create fresh supervisor for each query to avoid state accumulation
		// In production, you might want to use checkpointing or state management instead
		supervisor, err := createSupervisor()
		if err != nil {
			log.Printf("Error creating supervisor: %v", err)
			continue
		}

		// Execute and collect messages
		messages, err := graph.CollectMessages(supervisor.Run(ctx, []message.Message{
			message.NewHumanMessageFromText(query),
		}))
		if err != nil {
			log.Printf("Error executing query: %v", err)
			continue
		}

		// Display conversation transcript
		displayTranscript(query, messages)
	}
}

// createSupervisor creates a supervisor agent with specialized workers
func createSupervisor() (graph.MessageRunnable, error) {
	model := openai.NewModel()

	// Create specialized worker agents
	mathAgent, err := createMathAgent()
	if err != nil {
		return nil, fmt.Errorf("failed creating math agent: %w", err)
	}

	historyAgent, err := createHistoryAgent()
	if err != nil {
		return nil, fmt.Errorf("failed creating history agent: %w", err)
	}

	codeAgent, err := createCodeAgent()
	if err != nil {
		return nil, fmt.Errorf("failed creating code agent: %w", err)
	}

	// Create supervisor using the template with functional options
	return agent.NewSupervisorAgent(
		model,
		agent.WithWorker("math", "Expert in mathematics: algebra, calculus (derivatives, integrals), and general calculations.", mathAgent),
		agent.WithWorker("history", "Expert in historical facts, events, timelines, and context.", historyAgent),
		agent.WithWorker("code", "Expert in programming and software development.", codeAgent),
		agent.WithSupervisorSystemPrompt(`You are a supervisor that routes questions to specialist agents.
Analyze the user's question and delegate to the appropriate specialist.
Use handoff_to_math for mathematical problems.
Use handoff_to_history for historical questions.
Use handoff_to_code for programming tasks.
Always provide the full task context when delegating.`),
		agent.WithSupervisorMaxIterations(10),
		agent.WithWorkerContext(false), // Fresh context for each task
		agent.WithWorkerRetries(2),
	)
}

// createMathAgent creates a specialized agent for mathematical problem solving
func createMathAgent() (graph.MessageRunnable, error) {
	model := openai.NewModel()

	return agent.NewReActAgent(
		model,
		agent.WithSystemPrompt(`You are a math expert.
- Solve with clear, concise steps and provide a boxed final answer.
- For calculus, apply the power rule/product rule/chain rule as appropriate.
- Keep the explanation short (3-6 lines) unless complexity requires more.
- Always show your work.`),
		agent.WithMaxIterations(5),
	)
}

// createHistoryAgent creates a specialized agent for historical questions
func createHistoryAgent() (graph.MessageRunnable, error) {
	model := openai.NewModel()

	return agent.NewReActAgent(
		model,
		agent.WithSystemPrompt(`You are a history expert.
- Provide concise, factual answers with dates and key names when available.
- Avoid speculation; indicate uncertainty if sources conflict.
- Include relevant historical context when helpful.
- Cite time periods clearly (e.g., "1789-1797").`),
		agent.WithMaxIterations(5),
	)
}

// createCodeAgent creates a specialized agent for programming tasks
func createCodeAgent() (graph.MessageRunnable, error) {
	model := openai.NewModel()

	return agent.NewReActAgent(
		model,
		agent.WithSystemPrompt(`You are a programming expert.
- Write clean, well-documented code with explanations.
- Include docstrings and comments where appropriate.
- Follow language best practices and conventions.
- Provide usage examples when relevant.
- Keep code concise but readable.`),
		agent.WithMaxIterations(5),
	)
}

// displayTranscript shows the conversation flow including tool calls
func displayTranscript(query string, messages []message.Message) {
	// Print the user query at the top only
	fmt.Printf("👤 User:\n   %s\n", query)

	for i, msg := range messages {
		switch m := msg.(type) {
		case *message.AIMessage:
			// Check if this is supervisor routing or final response
			if len(m.ToolCalls) > 0 {
				fmt.Printf("\n🎯 Supervisor Routing:\n")
				for _, call := range m.ToolCalls {
					fmt.Printf("   → Delegating to: %s\n", call.Name)
					if args, ok := call.Arguments["task"].(string); ok {
						fmt.Printf("   → Task: %s\n", args)
					}
				}
			} else {
				// Final response
				fmt.Printf("\n🤖 Response:\n")
				for _, part := range m.Parts() {
					if text, ok := part.(message.TextPart); ok {
						// Format the response nicely
						lines := strings.SplitSeq(text.Text, "\n")
						for line := range lines {
							if line != "" {
								fmt.Printf("   %s\n", line)
							}
						}
					}
				}
			}

		case *message.ToolMessage:
			// Show specialist response
			fmt.Printf("\n✨ Specialist Result:\n")
			for _, part := range m.Parts() {
				if text, ok := part.(message.TextPart); ok {
					// Indent specialist response
					lines := strings.SplitSeq(text.Text, "\n")
					for line := range lines {
						if line != "" {
							fmt.Printf("   %s\n", line)
						}
					}
				}
			}

		case *message.SystemMessage:
			// Skip system messages in output
			continue
		case *message.HumanMessage:
			// Skip repeated user messages in the transcript
			continue
		}

		// Add spacing between messages
		if i < len(messages)-1 {
			fmt.Println()
		}
	}
	fmt.Println()
}
