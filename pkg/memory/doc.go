// Package memory provides long-term memory storage for multi-session conversations.
// It supports both simple key-value storage and semantic vector search for message retrieval.
//
// The memory system enables agents to maintain context across multiple interactions by:
//   - Storing messages with session/user identifiers
//   - Semantic search using vector embeddings
//   - Filtering and ranking by relevance
//   - Time-based and metadata-based queries
//   - Pluggable VectorStore backends for persistence
//
// # Basic Usage
//
//	// Create a vector memory with default in-memory store
//	embedder := embedding.NewSimpleEmbedder(128)
//	mem := memory.NewVectorMemory(embedder)
//
//	// Store conversation messages
//	err := mem.Store(ctx, "session-123", messages)
//
//	// Recall relevant messages by semantic similarity
//	recalled, err := mem.Recall(ctx, "session-123", memory.RecallFilter{
//	   Query: "What did we discuss about pricing?",
//	   K:     5,  // Top 5 most relevant messages
//	})
//
// # Custom VectorStore Backend
//
// VectorMemory supports pluggable backends via the vectorstore package:
//
//	// Use a custom VectorStore (e.g., Pinecone, Qdrant, etc.)
//	store := pinecone.New(...)  // or any VectorStore implementation
//	mem := memory.NewVectorMemory(embedder, memory.WithStore(store))
//
// The package provides two main implementations:
//   - VectorMemory: Semantic search using embeddings with VectorStore backend
//   - SimpleMemory: Basic FIFO storage without semantic search
package memory
