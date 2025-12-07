package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	vsqdrant "github.com/hupe1980/agentmesh/pkg/vectorstore/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/qdrant"
)

// setupQdrantContainer starts a Qdrant container for testing.
func setupQdrantContainer(t *testing.T, ctx context.Context) (string, func()) {
	t.Helper()

	container, err := qdrant.Run(ctx, "qdrant/qdrant:latest")
	require.NoError(t, err)

	grpcEndpoint, err := container.GRPCEndpoint(ctx)
	require.NoError(t, err)

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return grpcEndpoint, cleanup
}

func TestQdrantVectorStore_BasicOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	endpoint, cleanup := setupQdrantContainer(t, ctx)
	defer cleanup()

	// Create embedder
	embedder := embedding.NewSimpleEmbedder(4)

	// Create store with auto-create enabled
	store, err := vsqdrant.New(
		endpoint,
		vsqdrant.WithCollectionName("test_collection"),
		vsqdrant.WithDimensions(4),
		vsqdrant.WithAutoCreateCollection(true),
	)
	require.NoError(t, err)
	defer store.Close()

	// Embed documents before adding
	docs := []vectorstore.Document{
		{
			ID:       "doc1",
			Content:  "The quick brown fox jumps over the lazy dog",
			Metadata: map[string]any{"category": "animals", "priority": 1},
		},
		{
			ID:       "doc2",
			Content:  "A journey of a thousand miles begins with a single step",
			Metadata: map[string]any{"category": "wisdom", "priority": 2},
		},
		{
			ID:       "doc3",
			Content:  "To be or not to be, that is the question",
			Metadata: map[string]any{"category": "literature", "priority": 3},
		},
	}

	// Embed each document
	for i := range docs {
		emb, err := embedder.Embed(ctx, docs[i].Content)
		require.NoError(t, err)
		docs[i].Embedding = emb
	}

	err = store.Add(ctx, docs)
	require.NoError(t, err)

	// Search for similar documents
	queryEmb, err := embedder.Embed(ctx, "fox and dog")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K: 2,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Search with metadata filter
	queryEmb2, err := embedder.Embed(ctx, "wisdom")
	require.NoError(t, err)

	results, err = store.Search(ctx, queryEmb2, vectorstore.SearchOptions{
		K: 10,
		Filter: map[string]any{
			"category": "wisdom",
		},
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "doc2", results[0].ID)
}

func TestQdrantVectorStore_Namespaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	endpoint, cleanup := setupQdrantContainer(t, ctx)
	defer cleanup()

	embedder := embedding.NewSimpleEmbedder(4)

	store, err := vsqdrant.New(
		endpoint,
		vsqdrant.WithCollectionName("namespace_test"),
		vsqdrant.WithDimensions(4),
		vsqdrant.WithAutoCreateCollection(true),
	)
	require.NoError(t, err)
	defer store.Close()

	// Add documents to namespace A
	docsA := []vectorstore.Document{
		{ID: "a1", Content: "Document in namespace A"},
		{ID: "a2", Content: "Another document in namespace A"},
	}
	for i := range docsA {
		emb, err := embedder.Embed(ctx, docsA[i].Content)
		require.NoError(t, err)
		docsA[i].Embedding = emb
	}
	err = store.Add(ctx, docsA, func(o *vectorstore.AddOptions) {
		o.Namespace = "ns_a"
	})
	require.NoError(t, err)

	// Add documents to namespace B
	docsB := []vectorstore.Document{
		{ID: "b1", Content: "Document in namespace B"},
	}
	for i := range docsB {
		emb, err := embedder.Embed(ctx, docsB[i].Content)
		require.NoError(t, err)
		docsB[i].Embedding = emb
	}
	err = store.Add(ctx, docsB, func(o *vectorstore.AddOptions) {
		o.Namespace = "ns_b"
	})
	require.NoError(t, err)

	// Search in namespace A
	queryEmb, err := embedder.Embed(ctx, "document")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K:         10,
		Namespace: "ns_a",
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Search in namespace B
	results, err = store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K:         10,
		Namespace: "ns_b",
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Search without namespace returns all (in default collection)
	results, err = store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K: 10,
	})
	require.NoError(t, err)
	// This returns 0 because we stored them in namespaced collections
	// The default collection has no documents
	assert.Len(t, results, 0)
}

func TestQdrantVectorStore_IndexOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	endpoint, cleanup := setupQdrantContainer(t, ctx)
	defer cleanup()

	store, err := vsqdrant.New(
		endpoint,
		vsqdrant.WithDimensions(4),
		vsqdrant.WithAutoCreateCollection(false), // Don't auto-create
	)
	require.NoError(t, err)
	defer store.Close()

	// Create multiple indexes
	err = store.CreateIndex(ctx, "index1", 4, embedding.Cosine)
	require.NoError(t, err)

	err = store.CreateIndex(ctx, "index2", 4, embedding.Cosine)
	require.NoError(t, err)

	// List indexes
	indexes, err := store.ListIndexes(ctx)
	require.NoError(t, err)
	assert.Contains(t, indexes, "index1")
	assert.Contains(t, indexes, "index2")

	// Delete an index
	err = store.DeleteIndex(ctx, "index1")
	require.NoError(t, err)

	indexes, err = store.ListIndexes(ctx)
	require.NoError(t, err)
	assert.NotContains(t, indexes, "index1")
	assert.Contains(t, indexes, "index2")
}

func TestQdrantVectorStore_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	endpoint, cleanup := setupQdrantContainer(t, ctx)
	defer cleanup()

	embedder := embedding.NewSimpleEmbedder(4)

	store, err := vsqdrant.New(
		endpoint,
		vsqdrant.WithCollectionName("delete_test"),
		vsqdrant.WithDimensions(4),
		vsqdrant.WithAutoCreateCollection(true),
	)
	require.NoError(t, err)
	defer store.Close()

	// Add documents
	docs := []vectorstore.Document{
		{ID: "del1", Content: "Document to delete"},
		{ID: "del2", Content: "Document to keep"},
	}
	for i := range docs {
		emb, err := embedder.Embed(ctx, docs[i].Content)
		require.NoError(t, err)
		docs[i].Embedding = emb
	}
	err = store.Add(ctx, docs)
	require.NoError(t, err)

	// Delete one document
	err = store.Delete(ctx, []string{"del1"}, "")
	require.NoError(t, err)

	// Search should only return the remaining document
	queryEmb, err := embedder.Embed(ctx, "document")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K: 10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "del2", results[0].ID)
}

func TestQdrantVectorStore_Search(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	endpoint, cleanup := setupQdrantContainer(t, ctx)
	defer cleanup()

	embedder := embedding.NewSimpleEmbedder(4)

	store, err := vsqdrant.New(
		endpoint,
		vsqdrant.WithCollectionName("search_test"),
		vsqdrant.WithDimensions(4),
		vsqdrant.WithAutoCreateCollection(true),
	)
	require.NoError(t, err)
	defer store.Close()

	// Add documents
	docs := []vectorstore.Document{
		{ID: "s1", Content: "apple banana cherry"},
		{ID: "s2", Content: "dog cat mouse"},
		{ID: "s3", Content: "red green blue"},
	}
	for i := range docs {
		emb, err := embedder.Embed(ctx, docs[i].Content)
		require.NoError(t, err)
		docs[i].Embedding = emb
	}
	err = store.Add(ctx, docs)
	require.NoError(t, err)

	// Test K parameter
	queryEmb, err := embedder.Embed(ctx, "test")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K: 1,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)

	// Test with embeddings included
	results, err = store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K:                 1,
		IncludeEmbeddings: true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotNil(t, results[0].Embedding)
	assert.Len(t, results[0].Embedding, 4)
}
