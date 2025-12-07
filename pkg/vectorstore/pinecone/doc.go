// Package pinecone provides a Pinecone-backed VectorStore implementation.
//
// # Overview
//
// This package implements the vectorstore.VectorStore and vectorstore.Indexer
// interfaces using Pinecone as the backend. Pinecone is a managed vector database
// that provides high-performance similarity search at scale.
//
// # Features
//
//   - Serverless and pod-based deployment support
//   - Namespace isolation for multi-tenancy
//   - Metadata filtering with flexible query syntax
//   - Automatic index management
//
// # Usage
//
//	import "github.com/hupe1980/agentmesh/pkg/vectorstore/pinecone"
//
//	// Create a Pinecone store
//	store, err := pinecone.New(ctx,
//	    pinecone.WithAPIKey("your-api-key"),
//	    pinecone.WithIndexName("my-index"),
//	    pinecone.WithDimensions(1536),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
//	// Add documents
//	docs := []vectorstore.Document{
//	    {ID: "1", Content: "Hello", Embedding: embedding},
//	}
//	store.Add(ctx, docs)
//
//	// Search
//	results, _ := store.Search(ctx, queryEmbedding, vectorstore.SearchOptions{K: 5})
//
// # Testing
//
// For integration tests, set PINECONE_API_KEY environment variable:
//
//	PINECONE_API_KEY=xxx go test ./pkg/vectorstore/pinecone/...
package pinecone
