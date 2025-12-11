// Package main demonstrates Retrieval-Augmented Generation (RAG) using AgentMesh.
// This example shows:
//   - Setting up an in-memory vector store with OpenAI embeddings
//   - Creating a RAG agent that retrieves relevant context before generating
//   - Streaming responses for real-time output
//   - Using the agent.NewRAG builder for clean configuration
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
	embeddingOpenAI "github.com/hupe1980/agentmesh/pkg/embedding/openai"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model/openai"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	vsmemory "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	// Validate API key is set
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is required")
	}

	ctx := context.Background()

	fmt.Println("=== AgentMesh Basic RAG Example ===")
	fmt.Println()

	// Step 1: Create embedder and vector store
	fmt.Println("Setting up vector store with OpenAI embeddings...")

	embedder := embeddingOpenAI.NewEmbedder()
	store := vsmemory.New()
	embeddingStore := vectorstore.NewEmbeddingStore(store, embedder)

	// Step 2: Add knowledge documents
	documents := []string{
		"AgentMesh is a Go framework for building AI agents using a graph-based execution model.",
		"The framework uses Pregel's Bulk Synchronous Parallel (BSP) model for deterministic execution.",
		"Checkpointing in AgentMesh enables time-travel debugging and state persistence.",
		"Tools in AgentMesh allow agents to interact with external systems like APIs and databases.",
		"The message system supports multi-modal content including text, images, and tool calls.",
		"Middleware provides cross-cutting concerns like logging, retries, and rate limiting.",
		"RAG (Retrieval-Augmented Generation) combines retrieval with LLM generation for grounded answers.",
		"AgentMesh supports streaming responses for real-time output of partial results.",
	}

	metadata := []vectorstore.Metadata{
		{"topic": "overview"},
		{"topic": "execution"},
		{"topic": "debugging"},
		{"topic": "tools"},
		{"topic": "messages"},
		{"topic": "middleware"},
		{"topic": "rag"},
		{"topic": "streaming"},
	}

	if err := embeddingStore.AddTexts(ctx, documents, metadata); err != nil {
		return fmt.Errorf("failed to add documents: %w", err)
	}

	fmt.Printf("Added %d documents to knowledge base\n\n", len(documents))

	// Step 3: Create retriever
	retriever := retrieval.NewVectorStoreRetriever(store, embedder,
		retrieval.WithK(3),        // Retrieve top 3 most relevant documents
		retrieval.WithMinScore(0), // Include all matches
	)

	// Step 4: Create RAG agent with streaming enabled
	ragAgent, err := agent.NewRAG(
		openai.NewModel(),
		retriever,
		agent.WithInstructions("You are a helpful assistant that answers questions about AgentMesh. Use the provided context to give accurate, concise answers."),
		agent.WithStreaming(true), // Enable streaming for real-time output
	)
	if err != nil {
		return fmt.Errorf("failed to create RAG agent: %w", err)
	}

	// Step 5: Ask a question
	question := "What execution model does AgentMesh use and why?"
	fmt.Printf("Question: %s\n\n", question)
	fmt.Print("Answer: ")

	// Run the agent with streaming
	// Partial messages are streamed via the iterator
	messages := []message.Message{
		message.NewHumanMessageFromText(question),
	}

	for msg, err := range ragAgent.Run(ctx, messages) {
		if err != nil {
			return fmt.Errorf("agent execution failed: %w", err)
		}

		// Print streaming chunks as they arrive
		// AIMessageChunk = streaming partial output (print immediately)
		// AIMessage = final complete message (already in state, skip printing to avoid duplication)
		if chunk, ok := msg.(*message.AIMessageChunk); ok {
			fmt.Print(chunk.String())
		}
	}

	fmt.Println() // New line after streaming completes
	fmt.Println()

	// To access retrieved documents, we need to use RunWithState
	// For simplicity, this example just shows the streaming output
	fmt.Println("✅ RAG agent completed successfully!")

	return nil
}
