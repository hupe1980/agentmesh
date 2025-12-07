// Package pgvector provides a PostgreSQL pgvector-backed VectorStore implementation.
//
// pgvector is a PostgreSQL extension for vector similarity search. This
// implementation supports all core VectorStore operations including
// namespaced storage, metadata filtering, and index management.
//
// # Setup
//
// Start PostgreSQL with pgvector using Docker:
//
//	docker run -p 5432:5432 -e POSTGRES_PASSWORD=password pgvector/pgvector:pg17
//
// # Basic Usage
//
//	store, err := pgvector.New(ctx, "postgres://user:pass@localhost:5432/db",
//	    pgvector.WithTableName("documents"),
//	    pgvector.WithDimensions(1536),
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
// The pgvector store implements the Indexer interface:
//
//	// Create a table with vector column
//	err := store.CreateIndex(ctx, "products", 768, embedding.Cosine)
//
//	// List tables
//	tables, err := store.ListIndexes(ctx)
//
//	// Delete a table
//	err := store.DeleteIndex(ctx, "products")
//
// # Testing
//
// For testing, use testcontainers to spin up a PostgreSQL instance:
//
//	import "github.com/testcontainers/testcontainers-go/modules/postgres"
//
//	container, _ := postgres.Run(ctx, "pgvector/pgvector:pg17",
//	    postgres.WithDatabase("testdb"),
//	    postgres.WithUsername("test"),
//	    postgres.WithPassword("test"),
//	)
//	connStr, _ := container.ConnectionString(ctx)
//	store, _ := New(ctx, connStr)
package pgvector
