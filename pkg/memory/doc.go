// Package memory provides long-term memory storage for multi-session conversations.
// It supports both simple key-value storage and semantic vector search for message retrieval.
//
// The memory system enables agents to maintain context across multiple interactions by:
//   - Storing messages with session/user identifiers
//   - Semantic search using vector embeddings
//   - Filtering and ranking by relevance
//   - Time-based and metadata-based queries
//
// Example usage:
//
// // Create an in-memory vector store
// embedder := NewSimpleEmbedder(128)
// memory := NewVectorMemory(embedder)
//
// // Store conversation messages
// err := memory.Store(ctx, "session-123", messages)
//
// // Recall relevant messages by semantic similarity
//
//	recalled, err := memory.Recall(ctx, "session-123", RecallFilter{
//	   Query: "What did we discuss about pricing?",
//	   K:     5,  // Top 5 most relevant messages
//	})
//
// The package provides two main implementations:
//   - VectorMemory: Semantic search using embeddings
//   - SimpleMemory: Basic FIFO storage without semantic search
package memory
