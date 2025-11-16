// Package retrieval provides functionality for document retrieval and result merging in RAG workflows.
//
// # Overview
//
// The retrieval package defines the Retriever interface for fetching relevant documents
// based on queries, along with utilities for combining results from multiple retrieval
// sources. This is essential for Retrieval-Augmented Generation (RAG) patterns in agent systems.
//
// # Available Implementations
//
//   - MergerRetriever: Combines results from multiple retrievers in parallel
//   - LangChainRetriever: Adapter for langchaingo retrievers (pkg/retrieval/langchaingo)
//   - AmazonKendraRetriever: AWS Kendra search (pkg/retrieval/amazonkendra)
//   - AmazonBedrockRetriever: AWS Bedrock agents (pkg/retrieval/amazonbedrock)
//
// # Basic Usage
//
//	// Single retriever
//	retriever := langchaingo.NewRetriever(vectorStore, langchaingo.WithTopK(5))
//	docs, _ := retriever.Retrieve(ctx, "What is AgentMesh?")
//
//	for _, doc := range docs {
//	    fmt.Printf("Score: %.3f\n", doc.Score)
//	    fmt.Printf("Content: %s\n", doc.PageContent)
//	    fmt.Printf("Metadata: %v\n", doc.Metadata)
//	}
//
// # Merging Multiple Sources
//
// MergerRetriever combines results from multiple retrieval backends:
//
//	retrievers := []retrieval.Retriever{
//	    langchainRetriever,
//	    kendraRetriever,
//	    bedrockRetriever,
//	}
//
//	merger := retrieval.NewMergerRetriever(
//	    retrievers,
//	    retrieval.WithMergerMaxParallel(3),       // Run 3 at a time
//	    retrieval.WithMergerStopOnFirstError(false), // Continue on errors
//	)
//
//	// Fetches from all retrievers in parallel
//	allDocs, _ := merger.Retrieve(ctx, "query")
//
// # RAG Integration
//
// Retrievers are typically used with RAG agents to provide context:
//
//	// 1. Create retriever
//	retriever := langchaingo.NewRetriever(vectorStore)
//
//	// 2. Create RAG agent with retriever
//	ragAgent := agent.NewRAG(model, retriever,
//	    agent.WithTopK(5),
//	    agent.WithSystemPrompt("You are a helpful assistant..."),
//	)
//
//	// 3. Agent automatically retrieves context for queries
//	response, _ := graph.Last(ragAgent.Run(ctx, []message.Message{
//	    message.NewUserMessage("What is AgentMesh?"),
//	}))
//
// # Document Structure
//
// Retrieved documents contain three fields:
//
//   - PageContent: The actual text content
//
//   - Score: Relevance score (higher = more relevant)
//
//   - Metadata: Additional context (source, timestamp, etc.)
//
//     doc := retrieval.Document{
//     PageContent: "AgentMesh is a multi-agent orchestration framework",
//     Score:       0.92,
//     Metadata: map[string]any{
//     "source": "docs/overview.md",
//     "section": "Introduction",
//     },
//     }
//
// # Parallel Retrieval
//
// MergerRetriever controls concurrency via options:
//
//	// Sequential (one at a time)
//	merger := retrieval.NewMergerRetriever(retrievers,
//	    retrieval.WithMergerMaxParallel(0),
//	)
//
//	// Bounded parallelism (4 concurrent)
//	merger := retrieval.NewMergerRetriever(retrievers,
//	    retrieval.WithMergerMaxParallel(4),
//	)
//
// # Error Handling
//
// Control error propagation behavior:
//
//	// Stop on first error (default)
//	merger := retrieval.NewMergerRetriever(retrievers,
//	    retrieval.WithMergerStopOnFirstError(true),
//	)
//
//	// Collect all results, aggregate errors
//	merger := retrieval.NewMergerRetriever(retrievers,
//	    retrieval.WithMergerStopOnFirstError(false),
//	)
//
// # Thread Safety
//
// Retriever implementations must be safe for concurrent use. The Retrieve method
// may be called concurrently from multiple goroutines.
//
// # See Also
//
//   - pkg/embedding: Text-to-vector conversion for semantic search
//   - pkg/agent: RAG agent patterns
//   - examples/basic_agent: RAG workflow examples
package retrieval
