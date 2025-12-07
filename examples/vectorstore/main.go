package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/embedding"
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
	ctx := context.Background()

	fmt.Println("=== AgentMesh VectorStore Example ===")
	fmt.Println()

	// Create embedder (using SimpleEmbedder for demo - use openai.NewEmbedder() in production)
	embedder := embedding.NewSimpleEmbedder(384)

	// Create in-memory vector store
	store := vsmemory.New()

	// Create EmbeddingStore for automatic embedding generation
	es := vectorstore.NewEmbeddingStore(store, embedder)

	// Add documents with metadata
	fmt.Println("Adding documents to vector store...")

	texts := []string{
		"AgentMesh uses Pregel BSP for graph execution",
		"Checkpointing enables time-travel debugging",
		"Tools allow agents to call external APIs",
		"The message system supports multi-modal content",
		"Middleware provides cross-cutting concerns",
	}

	metadata := []vectorstore.Metadata{
		{"category": "core", "topic": "execution"},
		{"category": "core", "topic": "debugging"},
		{"category": "features", "topic": "tools"},
		{"category": "features", "topic": "messages"},
		{"category": "features", "topic": "middleware"},
	}

	if err := es.AddTexts(ctx, texts, metadata); err != nil {
		return fmt.Errorf("failed to add texts: %w", err)
	}

	fmt.Printf("Added %d documents\n\n", len(texts))

	// Semantic search
	query := "graph execution model"
	fmt.Printf("Searching for: '%s'\n", query)

	results, err := es.SearchText(ctx, query, vectorstore.SearchOptions{
		K:        3,
		MinScore: 0.0,
	})
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	fmt.Println("Results:")
	for i, doc := range results {
		fmt.Printf("  %d. Score: %.3f - %s\n", i+1, doc.Score, doc.Content)
	}
	fmt.Println()

	// Search with metadata filter
	fmt.Println("Filtering by category='core':")

	queryVec, err := embedder.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to embed query: %w", err)
	}

	filteredResults, err := store.Search(ctx, queryVec, vectorstore.SearchOptions{
		K:      3,
		Filter: vectorstore.Eq("category", "core"),
	})
	if err != nil {
		return fmt.Errorf("filtered search failed: %w", err)
	}

	for i, doc := range filteredResults {
		fmt.Printf("  %d. Score: %.3f - %s\n", i+1, doc.Score, doc.Content)
	}
	fmt.Println()

	// Use VectorStoreRetriever
	fmt.Println("Using VectorStoreRetriever...")

	retriever := retrieval.NewVectorStoreRetriever(store, embedder,
		retrieval.WithK(3),
		retrieval.WithMinScore(0.0),
	)

	docs, err := retriever.Retrieve(ctx, "how does the system work")
	if err != nil {
		return fmt.Errorf("retrieval failed: %w", err)
	}

	fmt.Printf("Retrieved %d documents for RAG context\n", len(docs))
	for i, doc := range docs {
		fmt.Printf("  %d. Score: %.3f - %s\n", i+1, doc.Score, doc.PageContent)
	}

	return nil
}
