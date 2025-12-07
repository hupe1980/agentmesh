package cache

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	memorystore "github.com/hupe1980/agentmesh/pkg/vectorstore/memory"
	"github.com/stretchr/testify/require"
)

func TestMemory_GetSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := embedding.NewSimpleEmbedder(64)
	cache := NewMemory(embedder, WithSimilarityThreshold(0.8))

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("What is 2+2?"),
		},
	}
	resp := &model.Response{
		Message: message.NewAIMessageFromText("4"),
	}

	// Initially no cache hit
	got, err := cache.Get(ctx, req)
	require.NoError(t, err)
	require.Nil(t, got)

	// Set the cache
	err = cache.Set(ctx, req, resp)
	require.NoError(t, err)

	// Now should get a hit
	got, err = cache.Get(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Metadata["cache_hit"].(bool))
}

func TestMemory_TTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := embedding.NewSimpleEmbedder(64)
	cache := NewMemory(embedder, WithTTL(50*time.Millisecond))

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("test"),
		},
	}
	resp := &model.Response{
		Message: message.NewAIMessageFromText("result"),
	}

	err := cache.Set(ctx, req, resp)
	require.NoError(t, err)

	// Immediate get should hit
	got, err := cache.Get(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Should now miss due to TTL
	got, err = cache.Get(ctx, req)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestMemory_Clear(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := embedding.NewSimpleEmbedder(64)
	cache := NewMemory(embedder)

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("test"),
		},
	}
	resp := &model.Response{
		Message: message.NewAIMessageFromText("result"),
	}

	err := cache.Set(ctx, req, resp)
	require.NoError(t, err)

	// Clear cache
	err = cache.Clear(ctx)
	require.NoError(t, err)

	// Should now miss
	got, err := cache.Get(ctx, req)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestMemory_LRUEviction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	embedder := embedding.NewSimpleEmbedder(64)
	cache := NewMemory(embedder, WithMaxSize(2))

	// Add 3 entries, first should be evicted
	for i := 0; i < 3; i++ {
		req := &model.Request{
			Messages: []message.Message{
				message.NewHumanMessageFromText(string(rune('A' + i))),
			},
		}
		resp := &model.Response{
			Message: message.NewAIMessageFromText(string(rune('a' + i))),
		}
		err := cache.Set(ctx, req, resp)
		require.NoError(t, err)
	}

	stats := cache.Stats()
	require.Equal(t, 2, stats.Size)
	require.Equal(t, int64(1), stats.Evictions)
}

func TestVectorStore_GetSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memorystore.New()
	defer store.Close()

	embedder := embedding.NewSimpleEmbedder(64)
	cache := NewVectorStore(store, embedder, WithSimilarityThreshold(0.8))

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("What is 2+2?"),
		},
	}
	resp := &model.Response{
		Message: message.NewAIMessageFromText("4"),
	}

	// Initially no cache hit
	got, err := cache.Get(ctx, req)
	require.NoError(t, err)
	require.Nil(t, got)

	// Set the cache
	err = cache.Set(ctx, req, resp)
	require.NoError(t, err)

	// Now should get a hit
	got, err = cache.Get(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.True(t, got.Metadata["cache_hit"].(bool))
}

func TestVectorStore_TTL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memorystore.New()
	defer store.Close()

	embedder := embedding.NewSimpleEmbedder(64)
	cache := NewVectorStore(store, embedder, WithTTL(50*time.Millisecond))

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("test"),
		},
	}
	resp := &model.Response{
		Message: message.NewAIMessageFromText("result"),
	}

	err := cache.Set(ctx, req, resp)
	require.NoError(t, err)

	// Immediate get should hit
	got, err := cache.Get(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Should now miss due to TTL
	got, err = cache.Get(ctx, req)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestVectorStore_Clear(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memorystore.New()
	defer store.Close()

	embedder := embedding.NewSimpleEmbedder(64)
	cache := NewVectorStore(store, embedder)

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("test"),
		},
	}
	resp := &model.Response{
		Message: message.NewAIMessageFromText("result"),
	}

	err := cache.Set(ctx, req, resp)
	require.NoError(t, err)

	// Clear cache
	err = cache.Clear(ctx)
	require.NoError(t, err)

	// Should now miss
	got, err := cache.Get(ctx, req)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestVectorStore_Namespace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := memorystore.New()
	defer store.Close()

	embedder := embedding.NewSimpleEmbedder(64)
	cache1 := NewVectorStore(store, embedder, WithNamespace("ns1"))
	cache2 := NewVectorStore(store, embedder, WithNamespace("ns2"))

	req := &model.Request{
		Messages: []message.Message{
			message.NewHumanMessageFromText("test"),
		},
	}
	resp := &model.Response{
		Message: message.NewAIMessageFromText("result"),
	}

	// Set in cache1
	err := cache1.Set(ctx, req, resp)
	require.NoError(t, err)

	// Should hit in cache1
	got, err := cache1.Get(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, got)

	// Should miss in cache2 (different namespace)
	got, err = cache2.Get(ctx, req)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()

	// Identical vectors
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	require.InDelta(t, 1.0, embedding.CosineSimilarity(a, b), 0.001)

	// Orthogonal vectors
	c := []float64{1, 0, 0}
	d := []float64{0, 1, 0}
	require.InDelta(t, 0.0, embedding.CosineSimilarity(c, d), 0.001)

	// Opposite vectors
	e := []float64{1, 0, 0}
	f := []float64{-1, 0, 0}
	require.InDelta(t, -1.0, embedding.CosineSimilarity(e, f), 0.001)
}

func TestFindMostSimilar(t *testing.T) {
	t.Parallel()

	query := []float64{1, 0, 0}
	entries := []*Entry{
		{Embedding: []float64{1, 0, 0}},     // Perfect match
		{Embedding: []float64{0.9, 0.1, 0}}, // Close match
		{Embedding: []float64{0, 1, 0}},     // Orthogonal
	}

	// With high threshold, only perfect match
	best, score := FindMostSimilar(query, entries, 0.99)
	require.NotNil(t, best)
	require.InDelta(t, 1.0, score, 0.001)

	// With lower threshold, should find close match too
	best, score = FindMostSimilar(query, entries, 0.9)
	require.NotNil(t, best)
	require.GreaterOrEqual(t, score, 0.9)

	// With threshold above all scores
	best, _ = FindMostSimilar([]float64{0, 0, 1}, entries, 0.99)
	require.Nil(t, best)
}
