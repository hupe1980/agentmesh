package integration

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/hupe1980/agentmesh/pkg/vectorstore/weaviate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcweaviate "github.com/testcontainers/testcontainers-go/modules/weaviate"
)

// setupWeaviateContainer creates a Weaviate container for testing.
func setupWeaviateContainer(t *testing.T, ctx context.Context) (*weaviate.Store, func()) {
	t.Helper()

	container, err := tcweaviate.Run(ctx, "semitechnologies/weaviate:1.24.0")
	require.NoError(t, err)

	scheme, host, err := container.HttpHostAddress(ctx)
	require.NoError(t, err)

	store, err := weaviate.New(
		weaviate.WithHost(host),
		weaviate.WithScheme(scheme),
		weaviate.WithClassName("TestDocuments"),
		weaviate.WithDimensions(4),
		weaviate.WithMetric(embedding.Cosine),
		weaviate.WithAutoCreateClass(true),
	)
	require.NoError(t, err)

	cleanup := func() {
		_ = store.Close()
		_ = tc.TerminateContainer(container)
	}

	return store, cleanup
}

func TestWeaviateVectorStore_BasicOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, cleanup := setupWeaviateContainer(t, ctx)
	defer cleanup()

	// Test Add
	docs := []vectorstore.Document{
		{
			ID:        "doc1",
			Content:   "Hello world",
			Embedding: []float64{0.1, 0.2, 0.3, 0.4},
			Metadata:  map[string]any{"category": "greeting"},
		},
		{
			ID:        "doc2",
			Content:   "Goodbye world",
			Embedding: []float64{0.4, 0.3, 0.2, 0.1},
			Metadata:  map[string]any{"category": "farewell"},
		},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(1 * time.Second)

	// Test Search
	results, err := store.Search(ctx, []float64{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{K: 5})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
	assert.Equal(t, "doc1", results[0].ID)

	// Test Delete
	err = store.Delete(ctx, []string{"doc1"}, "")
	require.NoError(t, err)
}

func TestWeaviateVectorStore_Namespaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, cleanup := setupWeaviateContainer(t, ctx)
	defer cleanup()

	// Add documents to different namespaces
	docs1 := []vectorstore.Document{
		{ID: "ns1-doc1", Content: "Namespace 1 doc", Embedding: []float64{0.1, 0.2, 0.3, 0.4}},
	}
	docs2 := []vectorstore.Document{
		{ID: "ns2-doc1", Content: "Namespace 2 doc", Embedding: []float64{0.5, 0.6, 0.7, 0.8}},
	}

	err := store.Add(ctx, docs1, func(o *vectorstore.AddOptions) { o.Namespace = "namespace1" })
	require.NoError(t, err)

	err = store.Add(ctx, docs2, func(o *vectorstore.AddOptions) { o.Namespace = "namespace2" })
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(1 * time.Second)

	// Search in namespace1
	results, err := store.Search(ctx, []float64{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{
		K:         5,
		Namespace: "namespace1",
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "ns1-doc1", results[0].ID)

	// Search in namespace2
	results, err = store.Search(ctx, []float64{0.5, 0.6, 0.7, 0.8}, vectorstore.SearchOptions{
		K:         5,
		Namespace: "namespace2",
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "ns2-doc1", results[0].ID)
}

func TestWeaviateVectorStore_IndexOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, cleanup := setupWeaviateContainer(t, ctx)
	defer cleanup()

	// Create a new index (class)
	err := store.CreateIndex(ctx, "NewClass", 4, embedding.Cosine)
	require.NoError(t, err)

	// List indexes
	indexes, err := store.ListIndexes(ctx)
	require.NoError(t, err)
	assert.Contains(t, indexes, "NewClass")

	// Delete the new index
	err = store.DeleteIndex(ctx, "NewClass")
	require.NoError(t, err)

	// Verify deletion
	indexes, err = store.ListIndexes(ctx)
	require.NoError(t, err)
	assert.NotContains(t, indexes, "NewClass")
}

func TestWeaviateVectorStore_Search(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, cleanup := setupWeaviateContainer(t, ctx)
	defer cleanup()

	// Add documents
	docs := []vectorstore.Document{
		{ID: "d1", Content: "First document", Embedding: []float64{1.0, 0.0, 0.0, 0.0}},
		{ID: "d2", Content: "Second document", Embedding: []float64{0.9, 0.1, 0.0, 0.0}},
		{ID: "d3", Content: "Third document", Embedding: []float64{0.0, 1.0, 0.0, 0.0}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(1 * time.Second)

	// Search with K limit
	results, err := store.Search(ctx, []float64{1.0, 0.0, 0.0, 0.0}, vectorstore.SearchOptions{
		K: 2,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Search with MinScore
	results, err = store.Search(ctx, []float64{1.0, 0.0, 0.0, 0.0}, vectorstore.SearchOptions{
		K:        5,
		MinScore: 0.9,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)
	for _, r := range results {
		assert.GreaterOrEqual(t, r.Score, 0.9)
	}
}

func TestWeaviateVectorStore_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, cleanup := setupWeaviateContainer(t, ctx)
	defer cleanup()

	// Add documents
	docs := []vectorstore.Document{
		{ID: "del1", Content: "Delete me 1", Embedding: []float64{0.1, 0.2, 0.3, 0.4}},
		{ID: "del2", Content: "Delete me 2", Embedding: []float64{0.2, 0.3, 0.4, 0.5}},
		{ID: "del3", Content: "Keep me", Embedding: []float64{0.3, 0.4, 0.5, 0.6}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(1 * time.Second)

	// Delete some documents
	err = store.Delete(ctx, []string{"del1", "del2"}, "")
	require.NoError(t, err)

	// Wait for deletion
	time.Sleep(500 * time.Millisecond)

	// Verify remaining documents
	results, err := store.Search(ctx, []float64{0.3, 0.4, 0.5, 0.6}, vectorstore.SearchOptions{K: 10})
	require.NoError(t, err)

	// Should only find del3/Keep me
	found := false
	for _, r := range results {
		if r.ID == "del3" {
			found = true
		}
		assert.NotEqual(t, "del1", r.ID)
		assert.NotEqual(t, "del2", r.ID)
	}
	assert.True(t, found, "Should find 'del3' document")
}
