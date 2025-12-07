package retrieval

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRerankerFunc(t *testing.T) {
	ctx := context.Background()

	reranker := RerankerFunc(func(ctx context.Context, query string, docs []Document) ([]Document, error) {
		// Reverse the order
		result := make([]Document, len(docs))
		for i, doc := range docs {
			result[len(docs)-1-i] = doc
		}
		return result, nil
	})

	docs := []Document{
		{PageContent: "first", Score: 0.9},
		{PageContent: "second", Score: 0.8},
	}

	reranked, err := reranker.Rerank(ctx, "query", docs)
	require.NoError(t, err)
	assert.Equal(t, "second", reranked[0].PageContent)
	assert.Equal(t, "first", reranked[1].PageContent)
}

func TestScoreReranker(t *testing.T) {
	ctx := context.Background()

	// Scorer that prefers shorter content
	scorer := func(ctx context.Context, query string, doc Document) (float64, error) {
		return 1.0 / float64(len(doc.PageContent)+1), nil
	}

	reranker := NewScoreReranker(scorer)

	docs := []Document{
		{PageContent: "this is a very long document"},
		{PageContent: "short"},
		{PageContent: "medium length"},
	}

	reranked, err := reranker.Rerank(ctx, "query", docs)
	require.NoError(t, err)
	require.Len(t, reranked, 3)

	// Shortest should be first
	assert.Equal(t, "short", reranked[0].PageContent)
}

func TestBoostReranker(t *testing.T) {
	ctx := context.Background()

	reranker := NewBoostReranker("priority", map[any]float64{
		"high":   2.0,
		"medium": 1.0,
		"low":    0.5,
	}, 1.0)

	docs := []Document{
		{PageContent: "doc1", Score: 0.8, Metadata: map[string]any{"priority": "low"}},
		{PageContent: "doc2", Score: 0.7, Metadata: map[string]any{"priority": "high"}},
		{PageContent: "doc3", Score: 0.9, Metadata: map[string]any{"priority": "medium"}},
	}

	reranked, err := reranker.Rerank(ctx, "query", docs)
	require.NoError(t, err)

	// doc2 with high boost should be first (0.7 * 2.0 = 1.4)
	assert.Equal(t, "doc2", reranked[0].PageContent)
	assert.InDelta(t, 1.4, reranked[0].Score, 0.001)
}

func TestRerankedRetriever(t *testing.T) {
	ctx := context.Background()

	// Mock retriever
	baseRetriever := RetrieverFunc(func(ctx context.Context, query string) ([]Document, error) {
		return []Document{
			{PageContent: "doc1", Score: 0.9},
			{PageContent: "doc2", Score: 0.8},
			{PageContent: "doc3", Score: 0.7},
		}, nil
	})

	// Reranker that reverses order
	reranker := RerankerFunc(func(ctx context.Context, query string, docs []Document) ([]Document, error) {
		result := make([]Document, len(docs))
		for i, doc := range docs {
			result[len(docs)-1-i] = doc
		}
		return result, nil
	})

	retriever := NewRerankedRetriever(baseRetriever, reranker, 2)

	results, err := retriever.Retrieve(ctx, "query")
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "doc3", results[0].PageContent)
	assert.Equal(t, "doc2", results[1].PageContent)
}

func TestChainedReranker(t *testing.T) {
	ctx := context.Background()

	// First reranker: reverse
	reverse := RerankerFunc(func(ctx context.Context, query string, docs []Document) ([]Document, error) {
		result := make([]Document, len(docs))
		for i, doc := range docs {
			result[len(docs)-1-i] = doc
		}
		return result, nil
	})

	// Second reranker: take first two
	first2 := RerankerFunc(func(ctx context.Context, query string, docs []Document) ([]Document, error) {
		if len(docs) > 2 {
			docs = docs[:2]
		}
		return docs, nil
	})

	chained := NewChainedReranker(reverse, first2)

	docs := []Document{
		{PageContent: "A"},
		{PageContent: "B"},
		{PageContent: "C"},
	}

	result, err := chained.Rerank(ctx, "query", docs)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "C", result[0].PageContent)
	assert.Equal(t, "B", result[1].PageContent)
}
