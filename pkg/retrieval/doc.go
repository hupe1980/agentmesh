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
//   - VectorStoreRetriever: Adapts any VectorStore to the Retriever interface
//   - MergerRetriever: Combines results from multiple retrievers in parallel
//   - RerankedRetriever: Wraps a retriever with reranking for improved relevance
//   - LangChainRetriever: Adapter for langchaingo retrievers (pkg/retrieval/langchaingo)
//   - AmazonKendraRetriever: AWS Kendra search (pkg/retrieval/amazonkendra)
//   - AmazonBedrockRetriever: AWS Bedrock agents (pkg/retrieval/amazonbedrock)
//
// # Basic Usage with VectorStore
//
//	// Create embedder and vector store
//	embedder := openai.NewEmbedder()
//	store := memory.New()
//	es := vectorstore.NewEmbeddingStore(store, embedder)
//
//	// Add documents
//	es.AddTexts(ctx, []string{"doc1", "doc2", "doc3"}, nil)
//
//	// Create retriever from vector store
//	retriever := retrieval.NewVectorStoreRetriever(store, embedder,
//	    retrieval.WithK(5),
//	    retrieval.WithMinScore(0.7),
//	)
//
//	docs, _ := retriever.Retrieve(ctx, "What is AgentMesh?")
//	for _, doc := range docs {
//	    fmt.Printf("Score: %.3f Content: %s\n", doc.Score, doc.PageContent)
//	}
//
// # Reranking
//
// Improve relevance with reranking:
//
//	// Boost documents by priority field
//	reranker := retrieval.NewBoostReranker("priority", map[any]float64{
//	    "high": 2.0, "medium": 1.0, "low": 0.5,
//	}, 1.0)
//
//	retriever := retrieval.NewRerankedRetriever(baseRetriever, reranker, 10)
//
// # Merging Multiple Sources
//
// MergerRetriever combines results from multiple retrieval backends:
//
//	retrievers := []retrieval.Retriever{
//	    vectorStoreRetriever,
//	    kendraRetriever,
//	    bedrockRetriever,
//	}
//
//	merger := retrieval.NewMergerRetriever(
//	    retrievers,
//	    retrieval.WithMergerMaxParallel(3),
//	    retrieval.WithMergerStopOnFirstError(false),
//	)
//
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
