package retrieval

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	vsmemory "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVectorStoreRetriever(t *testing.T) {
	ctx := context.Background()

	// Create embedder and store
	embedder := embedding.NewSimpleEmbedder(64)
	store := vsmemory.New()

	// Add some documents
	texts := []string{
		"The quick brown fox jumps over the lazy dog",
		"Machine learning is a subset of artificial intelligence",
		"Go is a statically typed programming language",
	}

	embeddings, err := embedder.EmbedBatch(ctx, texts)
	require.NoError(t, err)

	docs := make([]vectorstore.Document, len(texts))
	for i, text := range texts {
		docs[i] = vectorstore.Document{
			ID:        string(rune('a' + i)),
			Content:   text,
			Embedding: embeddings[i],
			Metadata:  map[string]any{"index": i},
		}
	}

	err = store.Add(ctx, docs)
	require.NoError(t, err)

	// Create retriever - use K=3 to get all documents and verify ML doc is included
	retriever := NewVectorStoreRetriever(store, embedder, WithK(3))

	// Retrieve
	results, err := retriever.Retrieve(ctx, "artificial intelligence and machine learning")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Should find the ML document somewhere in the results
	found := false
	for _, r := range results {
		if r.PageContent == texts[1] {
			found = true
			break
		}
	}
	assert.True(t, found, "Should find the machine learning document")
}

func TestVectorStoreRetrieverWithFilter(t *testing.T) {
	ctx := context.Background()

	embedder := embedding.NewSimpleEmbedder(64)
	store := vsmemory.New()

	docs := []vectorstore.Document{
		{ID: "1", Content: "doc about cats", Embedding: mustEmbed(embedder, "cats"), Metadata: map[string]any{"category": "animals"}},
		{ID: "2", Content: "doc about dogs", Embedding: mustEmbed(embedder, "dogs"), Metadata: map[string]any{"category": "animals"}},
		{ID: "3", Content: "doc about cars", Embedding: mustEmbed(embedder, "cars"), Metadata: map[string]any{"category": "vehicles"}},
	}

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	retriever := NewVectorStoreRetriever(store, embedder,
		WithK(10),
		WithFilter(vectorstore.Eq("category", "animals")),
	)

	results, err := retriever.Retrieve(ctx, "pets")
	require.NoError(t, err)
	require.Len(t, results, 2)

	for _, r := range results {
		assert.Equal(t, "animals", r.Metadata["category"])
	}
}

func mustEmbed(e embedding.Embedder, text string) embedding.Vector {
	vec, err := e.Embed(context.Background(), text)
	if err != nil {
		panic(err)
	}
	return vec
}
