// Package qdrant provides a Qdrant-backed VectorStore implementation.
//
// Qdrant is a high-performance vector database designed for similarity search.
// This implementation supports all core VectorStore operations including
// namespaced storage, metadata filtering, and index management.
//
// # Setup
//
// Start Qdrant using Docker:
//
//	docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant
//
// # Basic Usage
//
//	store, err := qdrant.New("localhost:6334",
//	    qdrant.WithCollectionName("my-collection"),
//	    qdrant.WithDimensions(1536),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
//	// Add documents
//	err = store.Add(ctx, []vectorstore.Document{
//	    {ID: "1", Content: "Hello", Embedding: vec1},
//	    {ID: "2", Content: "World", Embedding: vec2},
//	})
//
//	// Search
//	results, err := store.Search(ctx, queryVec, vectorstore.SearchOptions{K: 5})
//
// # Index Management
//
// The Qdrant store implements the Indexer interface:
//
//	// Create a collection
//	err := store.CreateIndex(ctx, "products", 768, embedding.Cosine)
//
//	// List collections
//	collections, err := store.ListIndexes(ctx)
//
//	// Delete a collection
//	err := store.DeleteIndex(ctx, "products")
//
// # Testing
//
// For testing, use testcontainers to spin up a Qdrant instance:
//
//	import "github.com/testcontainers/testcontainers-go/modules/qdrant"
//
//	container, _ := qdrant.Run(ctx, "qdrant/qdrant:latest")
//	endpoint, _ := container.GRPCEndpoint(ctx)
//	store, _ := New(endpoint)
package qdrant
