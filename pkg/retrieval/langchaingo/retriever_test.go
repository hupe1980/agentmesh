package langchaingo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

type stubRetriever struct {
	docs      []schema.Document
	err       error
	lastQuery string
}

func (s *stubRetriever) GetRelevantDocuments(ctx context.Context, query string) ([]schema.Document, error) {
	s.lastQuery = query

	if s.err != nil {
		return nil, s.err
	}

	return s.docs, nil
}

type stubVectorStore struct {
	docs         []schema.Document
	err          error
	lastQuery    string
	lastNumDocs  int
	lastOptions  []vectorstores.Option
	addDocuments []schema.Document
}

func (s *stubVectorStore) AddDocuments(
	ctx context.Context,
	docs []schema.Document,
	options ...vectorstores.Option,
) ([]string, error) {
	s.addDocuments = append(s.addDocuments, docs...)
	return nil, nil
}

func (s *stubVectorStore) SimilaritySearch(
	ctx context.Context,
	query string,
	numDocuments int,
	options ...vectorstores.Option,
) ([]schema.Document, error) {
	s.lastQuery = query
	s.lastNumDocs = numDocuments
	s.lastOptions = options

	if s.err != nil {
		return nil, s.err
	}

	return s.docs, nil
}

func TestRetriever_RetrieveSuccess(t *testing.T) {
	stub := &stubRetriever{
		docs: []schema.Document{
			{
				PageContent: "doc1",
				Metadata:    map[string]any{"source": "a"},
				Score:       0.7,
			},
			{
				PageContent: "doc2",
				Metadata:    map[string]any{"source": "b"},
				Score:       0.3,
			},
		},
	}

	retr := NewRetriever(stub)

	docs, err := retr.Retrieve(context.Background(), "  query  ")
	require.NoError(t, err)
	require.Equal(t, "query", stub.lastQuery)
	require.Len(t, docs, 2)

	require.Equal(t, "doc1", docs[0].PageContent)
	require.Equal(t, map[string]any{"source": "a"}, docs[0].Metadata)
	require.InDelta(t, 0.7, docs[0].Score, 1e-6)

	require.Equal(t, "doc2", docs[1].PageContent)
	require.Equal(t, map[string]any{"source": "b"}, docs[1].Metadata)
	require.InDelta(t, 0.3, docs[1].Score, 1e-6)
}

func TestRetriever_RetrieveError(t *testing.T) {
	boom := errors.New("boom")
	stub := &stubRetriever{err: boom}
	retr := NewRetriever(stub)

	docs, err := retr.Retrieve(context.Background(), "query")
	require.ErrorIs(t, err, boom)
	require.Nil(t, docs)
}

func TestRetriever_RetrieveEmptyQuery(t *testing.T) {
	retr := NewRetriever(&stubRetriever{})

	docs, err := retr.Retrieve(context.Background(), "   ")
	require.EqualError(t, err, "empty langchaingo query string")
	require.Nil(t, docs)
}

func TestNewRetrieverFromVectorStore(t *testing.T) {
	store := &stubVectorStore{
		docs: []schema.Document{
			{
				PageContent: "doc",
				Metadata:    map[string]any{"source": "store"},
				Score:       0.9,
			},
		},
	}

	scoreThreshold := float32(0.42)

	retr := NewRetrieverFromVectorStore(
		store,
		func(o *Options) {
			o.NumDocuments = 5
			o.VectorStoreOptions = []vectorstores.Option{vectorstores.WithScoreThreshold(scoreThreshold)}
		},
	)

	docs, err := retr.Retrieve(context.Background(), " query ")
	require.NoError(t, err)

	require.Equal(t, "query", store.lastQuery)
	require.Equal(t, 5, store.lastNumDocs)
	require.Len(t, store.lastOptions, 1)

	var applied vectorstores.Options
	for _, opt := range store.lastOptions {
		opt(&applied)
	}
	require.Equal(t, scoreThreshold, applied.ScoreThreshold)

	require.Len(t, docs, 1)
	require.Equal(t, "doc", docs[0].PageContent)
	require.Equal(t, map[string]any{"source": "store"}, docs[0].Metadata)
	require.InDelta(t, 0.9, docs[0].Score, 1e-6)
}
