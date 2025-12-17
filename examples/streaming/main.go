// Package main demonstrates real-time streaming execution in AgentMesh.
//
// This example shows how to:
//   - Stream LLM responses in real-time using the iterator pattern
//   - Receive partial AI messages as they're generated
//   - Handle both partial and complete messages cleanly
//
// The key insight: partial model responses are streamed via the Run() iterator,
// so you don't need an event bus for basic streaming use cases.
//
// Prerequisites:
//
//	export OPENAI_API_KEY="sk-..."
//
// Run: go run main.go
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
	"github.com/logrusorgru/aurora/v3"
)

func main() {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		log.Fatalf("OPENAI_API_KEY not set")
	}

	// Create OpenAI model
	openaiModel := openai.NewModel()

	// Create a simple agent graph
	g := graph.New()

	// Single model node with streaming enabled
	modelFn, err := agent.NewModelNodeFunc(openaiModel,
		agent.WithModelInstructions("You are a helpful assistant. Provide detailed, thoughtful responses."),
		agent.WithModelStreaming(true), // Enable streaming for real-time output
	)
	if err != nil {
		log.Fatalf("Failed to create model node: %v", err)
	}

	g.Node("model", modelFn, graph.END)
	g.Start("model")

	compiled, err := g.Build()
	if err != nil {
		log.Fatalf("Failed to build graph: %v", err)
	}

	// Prepare input
	messages := []message.Message{
		message.NewHumanMessageFromText("Explain how streaming works in AI applications in 3-4 sentences."),
	}

	fmt.Println("🚀 Streaming AI Response:")
	fmt.Println(strings.Repeat("─", 60))

	// Stream execution - partial messages appear as they're generated!
	ctx := context.Background()
	chunkCount := 0

	for msg, err := range compiled.Run(ctx, messages) {
		if err != nil {
			log.Fatalf("Execution error: %v", err)
		}

		// Handle streaming chunks vs final messages
		switch m := msg.(type) {
		case *message.AIMessageChunk:
			// Streaming partial output - print immediately
			fmt.Print(aurora.Green(m.String()))
			chunkCount++
		case *message.AIMessage:
			// Final complete message (already in state)
			// Skip printing to avoid duplication
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("✅ Received %d chunks\n", chunkCount)
}
