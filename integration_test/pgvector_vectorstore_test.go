package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	vspgvector "github.com/hupe1980/agentmesh/pkg/vectorstore/pgvector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupPostgresContainer starts a PostgreSQL container with pgvector for testing.
func setupPostgresContainer(t *testing.T, ctx context.Context) (string, func()) {
	t.Helper()

	container, err := postgres.Run(ctx, "pgvector/pgvector:pg17",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err)

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate container: %v", err)
		}
	}

	return connStr, cleanup
}

func TestPgvectorVectorStore_BasicOperations(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	connStr, cleanup := setupPostgresContainer(t, ctx)
	defer cleanup()

	embedder := embedding.NewSimpleEmbedder(4)

	store, err := vspgvector.New(ctx, connStr,
		vspgvector.WithTableName("test_docs"),
		vspgvector.WithDimensions(4),
		vspgvector.WithAutoCreateTable(true),
	)
	require.NoError(t, err)
	defer store.Close()

	docs := []vectorstore.Document{
		{
			ID:       "doc1",
			Content:  "The quick brown fox jumps over the lazy dog",
			Metadata: map[string]any{"category": "animals", "priority": "1"},
		},
		{
			ID:       "doc2",
			Content:  "A journey of a thousand miles begins with a single step",
			Metadata: map[string]any{"category": "wisdom", "priority": "2"},
		},
		{
			ID:       "doc3",
			Content:  "To be or not to be, that is the question",
			Metadata: map[string]any{"category": "literature", "priority": "3"},
		},
	}

	for i := range docs {
		emb, err := embedder.Embed(ctx, docs[i].Content)
		require.NoError(t, err)
		docs[i].Embedding = emb
	}

	err = store.Add(ctx, docs)
	require.NoError(t, err)

	queryEmb, err := embedder.Embed(ctx, "fox and dog")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K: 2,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)

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

func TestPgvectorVectorStore_Namespaces(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	connStr, cleanup := setupPostgresContainer(t, ctx)
	defer cleanup()

	embedder := embedding.NewSimpleEmbedder(4)

	store, err := vspgvector.New(ctx, connStr,
		vspgvector.WithTableName("namespace_test"),
		vspgvector.WithDimensions(4),
		vspgvector.WithAutoCreateTable(true),
	)
	require.NoError(t, err)
	defer store.Close()

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

	queryEmb, err := embedder.Embed(ctx, "document")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K:         10,
		Namespace: "ns_a",
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	results, err = store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K:         10,
		Namespace: "ns_b",
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestPgvectorVectorStore_IndexOperations(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	connStr, cleanup := setupPostgresContainer(t, ctx)
	defer cleanup()

	store, err := vspgvector.New(ctx, connStr,
		vspgvector.WithDimensions(4),
		vspgvector.WithAutoCreateTable(false),
	)
	require.NoError(t, err)
	defer store.Close()

	err = store.CreateIndex(ctx, "index1", 4, embedding.Cosine)
	require.NoError(t, err)

	err = store.CreateIndex(ctx, "index2", 4, embedding.Cosine)
	require.NoError(t, err)

	indexes, err := store.ListIndexes(ctx)
	require.NoError(t, err)
	assert.Contains(t, indexes, "index1")
	assert.Contains(t, indexes, "index2")

	err = store.DeleteIndex(ctx, "index1")
	require.NoError(t, err)

	indexes, err = store.ListIndexes(ctx)
	require.NoError(t, err)
	assert.NotContains(t, indexes, "index1")
	assert.Contains(t, indexes, "index2")
}

func TestPgvectorVectorStore_Delete(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	connStr, cleanup := setupPostgresContainer(t, ctx)
	defer cleanup()

	embedder := embedding.NewSimpleEmbedder(4)

	store, err := vspgvector.New(ctx, connStr,
		vspgvector.WithTableName("delete_test"),
		vspgvector.WithDimensions(4),
		vspgvector.WithAutoCreateTable(true),
	)
	require.NoError(t, err)
	defer store.Close()

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

	err = store.Delete(ctx, []string{"del1"}, "")
	require.NoError(t, err)

	queryEmb, err := embedder.Embed(ctx, "document")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K: 10,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "del2", results[0].ID)
}

func TestPgvectorVectorStore_Search(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	connStr, cleanup := setupPostgresContainer(t, ctx)
	defer cleanup()

	embedder := embedding.NewSimpleEmbedder(4)

	store, err := vspgvector.New(ctx, connStr,
		vspgvector.WithTableName("search_test"),
		vspgvector.WithDimensions(4),
		vspgvector.WithAutoCreateTable(true),
	)
	require.NoError(t, err)
	defer store.Close()

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

	queryEmb, err := embedder.Embed(ctx, "test")
	require.NoError(t, err)

	results, err := store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K: 1,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)

	results, err = store.Search(ctx, queryEmb, vectorstore.SearchOptions{
		K:                 1,
		IncludeEmbeddings: true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.NotNil(t, results[0].Embedding)
	assert.Len(t, results[0].Embedding, 4)
}
