package retrieval

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTextSearcher implements vectorstore.TextSearcher for testing.
type mockTextSearcher struct {
	searchHybridFunc func(ctx context.Context, query string, embedding embedding.Vector, opts vectorstore.HybridSearchOptions) ([]vectorstore.Document, error)
}

func (m *mockTextSearcher) Add(ctx context.Context, docs []vectorstore.Document, opts ...func(*vectorstore.AddOptions)) error {
	return nil
}

func (m *mockTextSearcher) Search(ctx context.Context, emb embedding.Vector, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	return nil, nil
}

func (m *mockTextSearcher) SearchHybrid(ctx context.Context, query string, emb embedding.Vector, opts vectorstore.HybridSearchOptions) ([]vectorstore.Document, error) {
	if m.searchHybridFunc != nil {
		return m.searchHybridFunc(ctx, query, emb, opts)
	}
	return nil, nil
}

func (m *mockTextSearcher) Delete(ctx context.Context, ids []string, namespace string) error {
	return nil
}

func (m *mockTextSearcher) Close() error {
	return nil
}

// mockHybridEmbedder is a simple mock embedder for testing.
type mockHybridEmbedder struct {
	embedFunc func(ctx context.Context, text string) (embedding.Vector, error)
}

func (m *mockHybridEmbedder) Embed(ctx context.Context, text string) (embedding.Vector, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, text)
	}
	return []float64{0.1, 0.2, 0.3}, nil
}

func (m *mockHybridEmbedder) EmbedBatch(ctx context.Context, texts []string) ([]embedding.Vector, error) {
	results := make([]embedding.Vector, len(texts))
	for i, text := range texts {
		vec, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

func (m *mockHybridEmbedder) Dimensions() int {
	return 3
}

func TestNewHybridVectorStoreRetriever(t *testing.T) {
	store := &mockTextSearcher{}
	embedder := &mockHybridEmbedder{}

	retriever := NewHybridVectorStoreRetriever(store, embedder)

	assert.NotNil(t, retriever)

	hvr := retriever.(*HybridVectorStoreRetriever)
	assert.Equal(t, 10, hvr.searchOpts.K)
	assert.Equal(t, 0.5, hvr.searchOpts.Alpha)
	assert.Equal(t, vectorstore.FusionRRF, hvr.searchOpts.FusionAlgorithm)
}

func TestNewHybridVectorStoreRetriever_WithOptions(t *testing.T) {
	store := &mockTextSearcher{}
	embedder := &mockHybridEmbedder{}

	retriever := NewHybridVectorStoreRetriever(store, embedder,
		WithHybridK(20),
		WithHybridMinScore(0.7),
		WithHybridNamespace("test-ns"),
		WithAlpha(0.8),
		WithFusionAlgorithm(vectorstore.FusionRelativeScore),
		WithHybridFilter(vectorstore.Filter{"category": "test"}),
	)

	hvr := retriever.(*HybridVectorStoreRetriever)
	assert.Equal(t, 20, hvr.searchOpts.K)
	assert.Equal(t, 0.7, hvr.searchOpts.MinScore)
	assert.Equal(t, "test-ns", hvr.searchOpts.Namespace)
	assert.Equal(t, 0.8, hvr.searchOpts.Alpha)
	assert.Equal(t, vectorstore.FusionRelativeScore, hvr.searchOpts.FusionAlgorithm)
	assert.Equal(t, "test", hvr.searchOpts.Filter["category"])
}

func TestHybridVectorStoreRetriever_Retrieve(t *testing.T) {
	var capturedQuery string
	var capturedEmbedding embedding.Vector
	var capturedOpts vectorstore.HybridSearchOptions

	store := &mockTextSearcher{
		searchHybridFunc: func(ctx context.Context, query string, emb embedding.Vector, opts vectorstore.HybridSearchOptions) ([]vectorstore.Document, error) {
			capturedQuery = query
			capturedEmbedding = emb
			capturedOpts = opts
			return []vectorstore.Document{
				{ID: "doc1", Content: "Hello world", Score: 0.95, Metadata: map[string]any{"key": "value"}},
				{ID: "doc2", Content: "Hello universe", Score: 0.85},
			}, nil
		},
	}

	embedder := &mockHybridEmbedder{
		embedFunc: func(ctx context.Context, text string) (embedding.Vector, error) {
			return []float64{0.1, 0.2, 0.3}, nil
		},
	}

	retriever := NewHybridVectorStoreRetriever(store, embedder,
		WithHybridK(5),
		WithAlpha(0.7),
	)

	results, err := retriever.Retrieve(context.Background(), "test query")
	require.NoError(t, err)

	assert.Equal(t, "test query", capturedQuery)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, capturedEmbedding)
	assert.Equal(t, 5, capturedOpts.K)
	assert.Equal(t, 0.7, capturedOpts.Alpha)

	assert.Len(t, results, 2)
	assert.Equal(t, "Hello world", results[0].PageContent)
	assert.Equal(t, 0.95, results[0].Score)
	assert.Equal(t, "value", results[0].Metadata["key"])
	assert.Equal(t, "Hello universe", results[1].PageContent)
}

func TestHybridVectorStoreRetriever_Retrieve_EmbedError(t *testing.T) {
	store := &mockTextSearcher{}
	embedder := &mockHybridEmbedder{
		embedFunc: func(ctx context.Context, text string) (embedding.Vector, error) {
			return nil, errors.New("embed failed")
		},
	}

	retriever := NewHybridVectorStoreRetriever(store, embedder)

	_, err := retriever.Retrieve(context.Background(), "test query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embed failed")
}

func TestHybridVectorStoreRetriever_Retrieve_SearchError(t *testing.T) {
	store := &mockTextSearcher{
		searchHybridFunc: func(ctx context.Context, query string, emb embedding.Vector, opts vectorstore.HybridSearchOptions) ([]vectorstore.Document, error) {
			return nil, errors.New("search failed")
		},
	}
	embedder := &mockHybridEmbedder{
		embedFunc: func(ctx context.Context, text string) (embedding.Vector, error) {
			return []float64{0.1, 0.2, 0.3}, nil
		},
	}

	retriever := NewHybridVectorStoreRetriever(store, embedder)

	_, err := retriever.Retrieve(context.Background(), "test query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search failed")
}

func TestHybridVectorStoreRetriever_Retrieve_EmptyResults(t *testing.T) {
	store := &mockTextSearcher{
		searchHybridFunc: func(ctx context.Context, query string, emb embedding.Vector, opts vectorstore.HybridSearchOptions) ([]vectorstore.Document, error) {
			return []vectorstore.Document{}, nil
		},
	}
	embedder := &mockHybridEmbedder{
		embedFunc: func(ctx context.Context, text string) (embedding.Vector, error) {
			return []float64{0.1, 0.2, 0.3}, nil
		},
	}

	retriever := NewHybridVectorStoreRetriever(store, embedder)

	results, err := retriever.Retrieve(context.Background(), "test query")
	require.NoError(t, err)
	assert.Empty(t, results)
}
