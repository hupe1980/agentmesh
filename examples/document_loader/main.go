package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/pkg/loader"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
)

func main() {
	ctx := context.Background()

	// Create a mock embedder (replace with real embedder in production)
	embedder := testutil.NewMockEmbedder(64)

	// Create an in-memory vector store with embedding support
	store := vectorstore.NewEmbeddingStore(memory.New(), embedder)

	// Example 1: Load from a string
	fmt.Println("=== Loading from string ===")
	stringLoader := loader.NewStringLoader(
		"AgentMesh is a powerful framework for building AI agents. "+
			"It provides tools for memory, retrieval, and orchestration.",
		loader.WithStringSource("inline"),
	)

	docs, err := stringLoader.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Loaded %d document(s) from string\n", len(docs))

	// Example 2: Load from a reader
	fmt.Println("\n=== Loading from reader ===")
	readerContent := `This is content loaded from a reader.
It can be any io.Reader source like files, network, etc.
Very flexible for various use cases.`
	readerLoader := loader.NewReaderLoader(
		strings.NewReader(readerContent),
		loader.WithReaderSource("reader-example"),
	)

	docs, err = readerLoader.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Loaded %d document(s) from reader\n", len(docs))

	// Example 3: Text splitting
	fmt.Println("\n=== Text splitting ===")
	longContent := `# Introduction

AgentMesh is a framework for building AI agents in Go.

## Features

AgentMesh provides many powerful features:

1. Memory Management - Store and recall conversation history
2. Tool Integration - Connect to external APIs and services
3. Graph-Based Workflows - Build complex agent pipelines

## Getting Started

To get started with AgentMesh, first install it:

go get github.com/hupe1980/agentmesh

Then create your first agent!`

	splitter := loader.NewRecursiveCharacterSplitter(200, 20,
		loader.WithSeparators([]string{"\n\n", "\n", ". ", " "}),
	)

	doc := loader.Document{Content: longContent, Source: "readme"}
	chunks, err := splitter.Split(doc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Split into %d chunk(s)\n", len(chunks))
	for i, chunk := range chunks {
		preview := chunk.Content
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		fmt.Printf("  Chunk %d: %q\n", i+1, preview)
	}

	// Example 4: Full ingestion pipeline (using raw VectorStore, not EmbeddingStore)
	fmt.Println("\n=== Full ingestion pipeline ===")

	// Create a raw memory store for the pipeline
	rawStore := memory.New()

	// Create a loader that returns multiple documents
	multiDocLoader := loader.Func(func(_ context.Context) ([]loader.Document, error) {
		return []loader.Document{
			{Content: "Document 1: Introduction to machine learning concepts.", Source: "ml-intro"},
			{Content: "Document 2: Deep learning neural networks architecture.", Source: "deep-learning"},
			{Content: "Document 3: Natural language processing fundamentals.", Source: "nlp-basics"},
		}, nil
	})

	// Create a simple splitter (no actual splitting for short docs)
	simpleSplitter := loader.NewRecursiveCharacterSplitter(1000, 100)

	// Create the pipeline
	pipeline := loader.NewPipeline(multiDocLoader, simpleSplitter, rawStore,
		loader.WithPipelineBatchSize(10),
		loader.WithPipelineProgress(func(processed, total int) {
			fmt.Printf("  Progress: %d/%d\n", processed, total)
		}),
	)

	// Run the pipeline
	if err := pipeline.Run(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Pipeline completed successfully!")

	// Example 5: Document with metadata
	fmt.Println("\n=== Document with metadata ===")
	doc = loader.NewDocument("Technical documentation content").
		WithSource("docs/api.md").
		WithMetadata("category", "documentation").
		WithMetadata("version", "1.0")

	fmt.Printf("Document: source=%s, metadata=%v\n", doc.Source, doc.Metadata)

	// Example 6: Use EmbeddingStore for search
	fmt.Println("\n=== Adding documents via EmbeddingStore ===")
	texts := []string{
		"How to build AI agents",
		"Memory management best practices",
		"Graph-based workflow design",
	}
	if err := store.AddTexts(ctx, texts, nil); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Added %d texts to EmbeddingStore\n", len(texts))

	fmt.Println("\n=== Demo complete ===")
}
