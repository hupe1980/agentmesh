// Package weaviate provides a Weaviate-backed VectorStore implementation.
//
// # Overview
//
// This package implements the vectorstore.VectorStore and vectorstore.Indexer
// interfaces using Weaviate as the backend. Weaviate is an open-source vector
// database that supports hybrid search (vector + keyword) and GraphQL queries.
//
// # Features
//
//   - Hybrid search combining vector and keyword search
//   - GraphQL-based querying
//   - Multi-tenancy support
//   - Automatic schema management
//
// # Usage
//
//	import "github.com/hupe1980/agentmesh/pkg/vectorstore/weaviate"
//
//	store, err := weaviate.New(
//	    weaviate.WithHost("localhost:8080"),
//	    weaviate.WithClassName("Documents"),
//	    weaviate.WithDimensions(1536),
//	)
//	defer store.Close()
//
// # Testing with Docker
//
//	docker run -d -p 8080:8080 semitechnologies/weaviate:latest
package weaviate
