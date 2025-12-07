// Package vectorstore provides interfaces and implementations for vector storage backends.
//
// # Overview
//
// The vectorstore package defines the VectorStore interface for storing and searching
// documents by their vector embeddings. This enables semantic search, RAG (Retrieval-Augmented
// Generation), and similarity matching workflows.
//
// # Core Concepts
//
//   - Document: A storable item with content, embedding, and metadata
//   - VectorStore: Interface for Add/Search/Delete operations
//   - EmbeddingStore: Helper that auto-generates embeddings from text
//   - Filter: Metadata-based filtering for search queries
//
// # Available Implementations
//
//   - memory.Store: In-memory store for testing and development
//   - qdrant.Store: Qdrant vector database (gRPC-based)
//   - pgvector.Store: PostgreSQL with pgvector extension
//   - (future) pinecone.Store: Pinecone vector database
//
// # Basic Usage
//
//	// Create an in-memory store
//	store := memory.New()
//
//	// Add documents with embeddings
//	docs := []vectorstore.Document{
//	    {ID: "1", Content: "Hello world", Embedding: []float64{0.1, 0.2, ...}},
//	    {ID: "2", Content: "Goodbye world", Embedding: []float64{0.3, 0.4, ...}},
//	}
//	store.Add(ctx, docs)
//
//	// Search by embedding vector
//	results, _ := store.Search(ctx, queryEmbedding, vectorstore.SearchOptions{K: 5})
//
// # Using EmbeddingStore
//
// For convenience, EmbeddingStore auto-generates embeddings:
//
//	embedder := openai.NewEmbedder()
//	store := memory.New()
//	es := vectorstore.NewEmbeddingStore(store, embedder)
//
//	// Add texts (embeddings generated automatically)
//	es.AddTexts(ctx, []string{"doc1", "doc2"}, nil)
//
//	// Search by text query
//	results, _ := es.SearchText(ctx, "find similar docs", vectorstore.SearchOptions{K: 10})
//
// # Metadata Filtering
//
// Filter search results by metadata:
//
//	results, _ := store.Search(ctx, embedding, vectorstore.SearchOptions{
//	    K:      10,
//	    Filter: vectorstore.Eq("category", "science"),
//	})
//
// # Namespaces
//
// Partition data for multi-tenant scenarios:
//
//	store.Add(ctx, docs, func(o *vectorstore.AddOptions) {
//	    o.Namespace = "tenant-123"
//	})
//
//	results, _ := store.Search(ctx, embedding, vectorstore.SearchOptions{
//	    Namespace: "tenant-123",
//	})
package vectorstore
