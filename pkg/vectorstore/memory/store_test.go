package memory

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_Add(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "hello", Embedding: []float64{1.0, 0.0, 0.0}},
		{ID: "2", Content: "world", Embedding: []float64{0.0, 1.0, 0.0}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	stats, err := store.Stats(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), stats.DocumentCount)
}

func TestStore_AddGeneratesIDs(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{Content: "no id", Embedding: []float64{1.0, 0.0}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	// Search should find the document
	results, err := store.Search(ctx, []float64{1.0, 0.0}, vectorstore.SearchOptions{K: 1})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotEmpty(t, results[0].ID)
}

func TestStore_Search(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "hello", Embedding: []float64{1.0, 0.0, 0.0}},
		{ID: "2", Content: "world", Embedding: []float64{0.0, 1.0, 0.0}},
		{ID: "3", Content: "foo", Embedding: []float64{0.0, 0.0, 1.0}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	// Search for vector closest to doc 1
	results, err := store.Search(ctx, []float64{0.9, 0.1, 0.0}, vectorstore.SearchOptions{K: 2})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "1", results[0].ID)
	assert.Greater(t, results[0].Score, results[1].Score)
}

func TestStore_SearchWithMinScore(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "match", Embedding: embedding.Normalize([]float64{1.0, 0.0, 0.0})},
		{ID: "2", Content: "partial", Embedding: embedding.Normalize([]float64{0.7, 0.7, 0.0})},
		{ID: "3", Content: "no match", Embedding: embedding.Normalize([]float64{0.0, 0.0, 1.0})},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	query := embedding.Normalize([]float64{1.0, 0.0, 0.0})
	results, err := store.Search(ctx, query, vectorstore.SearchOptions{K: 10, MinScore: 0.9})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "1", results[0].ID)
}

func TestStore_SearchWithFilter(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "doc1", Embedding: []float64{1.0, 0.0}, Metadata: map[string]any{"type": "article"}},
		{ID: "2", Content: "doc2", Embedding: []float64{0.9, 0.1}, Metadata: map[string]any{"type": "blog"}},
		{ID: "3", Content: "doc3", Embedding: []float64{0.8, 0.2}, Metadata: map[string]any{"type": "article"}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	results, err := store.Search(ctx, []float64{1.0, 0.0}, vectorstore.SearchOptions{
		K:      10,
		Filter: vectorstore.Eq("type", "article"),
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, "article", r.Metadata["type"])
	}
}

func TestStore_SearchWithNamespace(t *testing.T) {
	ctx := context.Background()
	store := New()

	// Add to namespace "a"
	err := store.Add(ctx, []vectorstore.Document{
		{ID: "1", Content: "ns-a", Embedding: []float64{1.0, 0.0}},
	}, func(o *vectorstore.AddOptions) { o.Namespace = "a" })
	require.NoError(t, err)

	// Add to namespace "b"
	err = store.Add(ctx, []vectorstore.Document{
		{ID: "2", Content: "ns-b", Embedding: []float64{0.0, 1.0}},
	}, func(o *vectorstore.AddOptions) { o.Namespace = "b" })
	require.NoError(t, err)

	// Search in namespace "a"
	results, err := store.Search(ctx, []float64{1.0, 0.0}, vectorstore.SearchOptions{K: 10, Namespace: "a"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "1", results[0].ID)

	// Search in namespace "b"
	results, err = store.Search(ctx, []float64{1.0, 0.0}, vectorstore.SearchOptions{K: 10, Namespace: "b"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "2", results[0].ID)
}

func TestStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "keep", Embedding: []float64{1.0, 0.0}},
		{ID: "2", Content: "delete", Embedding: []float64{0.0, 1.0}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	err = store.Delete(ctx, []string{"2"}, "")
	require.NoError(t, err)

	stats, err := store.Stats(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.DocumentCount)
}

func TestStore_Upsert(t *testing.T) {
	ctx := context.Background()
	store := New()

	// Add initial document
	err := store.Add(ctx, []vectorstore.Document{
		{ID: "1", Content: "original", Embedding: []float64{1.0, 0.0}},
	})
	require.NoError(t, err)

	// Upsert (default behavior)
	err = store.Add(ctx, []vectorstore.Document{
		{ID: "1", Content: "updated", Embedding: []float64{0.0, 1.0}},
	})
	require.NoError(t, err)

	results, err := store.Search(ctx, []float64{0.0, 1.0}, vectorstore.SearchOptions{K: 1})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "updated", results[0].Content)
}

func TestStore_NoUpsert(t *testing.T) {
	ctx := context.Background()
	store := New()

	// Add initial document
	err := store.Add(ctx, []vectorstore.Document{
		{ID: "1", Content: "original", Embedding: []float64{1.0, 0.0}},
	})
	require.NoError(t, err)

	// Try to add with upsert disabled
	err = store.Add(ctx, []vectorstore.Document{
		{ID: "1", Content: "should not update", Embedding: []float64{0.0, 1.0}},
	}, func(o *vectorstore.AddOptions) { o.Upsert = false })
	require.NoError(t, err)

	results, err := store.Search(ctx, []float64{1.0, 0.0}, vectorstore.SearchOptions{K: 1})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "original", results[0].Content)
}

func TestStore_Clear(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "hello", Embedding: []float64{1.0, 0.0}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	store.Clear()

	stats, err := store.Stats(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.DocumentCount)
}

func TestStore_ExcludeEmbeddings(t *testing.T) {
	ctx := context.Background()
	store := New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "hello", Embedding: []float64{1.0, 0.0, 0.0}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	// By default, embeddings are excluded
	results, err := store.Search(ctx, []float64{1.0, 0.0, 0.0}, vectorstore.SearchOptions{K: 1})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Nil(t, results[0].Embedding)

	// Explicitly include embeddings
	results, err = store.Search(ctx, []float64{1.0, 0.0, 0.0}, vectorstore.SearchOptions{K: 1, IncludeEmbeddings: true})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotNil(t, results[0].Embedding)
}
