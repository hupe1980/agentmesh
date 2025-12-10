package pinecone

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/pinecone-io/go-pinecone/pinecone"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

// mockClient implements the Client interface for testing.
type mockClient struct {
	createServerlessIndexFunc func(ctx context.Context, in *pinecone.CreateServerlessIndexRequest) (*pinecone.Index, error)
	deleteIndexFunc           func(ctx context.Context, name string) error
	listIndexesFunc           func(ctx context.Context) ([]*pinecone.Index, error)
}

func (m *mockClient) CreateServerlessIndex(ctx context.Context, in *pinecone.CreateServerlessIndexRequest) (*pinecone.Index, error) {
	if m.createServerlessIndexFunc != nil {
		return m.createServerlessIndexFunc(ctx, in)
	}
	return &pinecone.Index{Name: in.Name}, nil
}

func (m *mockClient) DeleteIndex(ctx context.Context, name string) error {
	if m.deleteIndexFunc != nil {
		return m.deleteIndexFunc(ctx, name)
	}
	return nil
}

func (m *mockClient) ListIndexes(ctx context.Context) ([]*pinecone.Index, error) {
	if m.listIndexesFunc != nil {
		return m.listIndexesFunc(ctx)
	}
	return []*pinecone.Index{}, nil
}

// mockIndexConnection implements the IndexConnection interface for testing.
type mockIndexConnection struct {
	upsertVectorsFunc       func(ctx context.Context, in []*pinecone.Vector) (uint32, error)
	queryByVectorValuesFunc func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error)
	deleteVectorsByIdFunc   func(ctx context.Context, ids []string) error
	closeFunc               func() error
}

func (m *mockIndexConnection) UpsertVectors(ctx context.Context, in []*pinecone.Vector) (uint32, error) {
	if m.upsertVectorsFunc != nil {
		return m.upsertVectorsFunc(ctx, in)
	}
	return uint32(len(in)), nil
}

func (m *mockIndexConnection) QueryByVectorValues(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
	if m.queryByVectorValuesFunc != nil {
		return m.queryByVectorValuesFunc(ctx, in)
	}
	return &pinecone.QueryVectorsResponse{}, nil
}

func (m *mockIndexConnection) DeleteVectorsById(ctx context.Context, ids []string) error {
	if m.deleteVectorsByIdFunc != nil {
		return m.deleteVectorsByIdFunc(ctx, ids)
	}
	return nil
}

func (m *mockIndexConnection) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestNew(t *testing.T) {
	client := &mockClient{}
	idx := &mockIndexConnection{}
	store := New(client, idx, "test-index")

	assert.NotNil(t, store)
	assert.Equal(t, "test-index", store.indexName)
	assert.Equal(t, embedding.Cosine, store.opts.Metric)
	assert.Equal(t, "aws", store.opts.Cloud)
	assert.Equal(t, "us-east-1", store.opts.Region)
}

func TestNew_WithOptions(t *testing.T) {
	client := &mockClient{}
	idx := &mockIndexConnection{}
	store := New(client, idx, "test-index",
		WithMetric(embedding.Euclidean),
		WithCloud("gcp"),
		WithRegion("us-central1"),
	)

	assert.Equal(t, embedding.Euclidean, store.opts.Metric)
	assert.Equal(t, "gcp", store.opts.Cloud)
	assert.Equal(t, "us-central1", store.opts.Region)
}

func TestStore_Add(t *testing.T) {
	var capturedVectors []*pinecone.Vector

	idx := &mockIndexConnection{
		upsertVectorsFunc: func(ctx context.Context, in []*pinecone.Vector) (uint32, error) {
			capturedVectors = in
			return uint32(len(in)), nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	docs := []vectorstore.Document{
		{
			ID:        "doc1",
			Content:   "Hello world",
			Embedding: []float32{0.1, 0.2, 0.3, 0.4},
			Metadata:  map[string]any{"category": "greeting"},
		},
		{
			ID:        "doc2",
			Content:   "Goodbye world",
			Embedding: []float32{0.4, 0.3, 0.2, 0.1},
		},
	}

	err := store.Add(context.Background(), docs)
	require.NoError(t, err)

	assert.Len(t, capturedVectors, 2)
	assert.Equal(t, "doc1", capturedVectors[0].Id)
	assert.Equal(t, "doc2", capturedVectors[1].Id)
	assert.Equal(t, []float32{0.1, 0.2, 0.3, 0.4}, capturedVectors[0].Values)
}

func TestStore_Add_EmptyDocs(t *testing.T) {
	idx := &mockIndexConnection{
		upsertVectorsFunc: func(ctx context.Context, in []*pinecone.Vector) (uint32, error) {
			t.Fatal("should not be called")
			return 0, nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	err := store.Add(context.Background(), []vectorstore.Document{})
	assert.NoError(t, err)

	err = store.Add(context.Background(), nil)
	assert.NoError(t, err)
}

func TestStore_Add_Error(t *testing.T) {
	idx := &mockIndexConnection{
		upsertVectorsFunc: func(ctx context.Context, in []*pinecone.Vector) (uint32, error) {
			return 0, errors.New("upsert failed")
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	docs := []vectorstore.Document{
		{ID: "doc1", Embedding: []float32{0.1, 0.2}},
	}

	err := store.Add(context.Background(), docs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upsert vectors")
}

func TestStore_Search(t *testing.T) {
	metadata, _ := structpb.NewStruct(map[string]any{
		"content":   "Hello world",
		"timestamp": float64(1234567890),
		"category":  "greeting",
	})

	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			return &pinecone.QueryVectorsResponse{
				Matches: []*pinecone.ScoredVector{
					{
						Vector: &pinecone.Vector{
							Id:       "doc1",
							Values:   []float32{0.1, 0.2, 0.3, 0.4},
							Metadata: metadata,
						},
						Score: 0.95,
					},
				},
			}, nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	results, err := store.Search(context.Background(), []float32{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{K: 5})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
	assert.Equal(t, "Hello world", results[0].Content)
	assert.InDelta(t, 0.95, results[0].Score, 0.001)
	assert.Equal(t, "greeting", results[0].Metadata["category"])
}

func TestStore_Search_WithMinScore(t *testing.T) {
	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			return &pinecone.QueryVectorsResponse{
				Matches: []*pinecone.ScoredVector{
					{Vector: &pinecone.Vector{Id: "doc1"}, Score: 0.95},
					{Vector: &pinecone.Vector{Id: "doc2"}, Score: 0.5},
					{Vector: &pinecone.Vector{Id: "doc3"}, Score: 0.3},
				},
			}, nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	results, err := store.Search(context.Background(), []float32{0.1, 0.2}, vectorstore.SearchOptions{
		K:        10,
		MinScore: 0.8,
	})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
}

func TestStore_Search_WithEmbeddings(t *testing.T) {
	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			assert.True(t, in.IncludeValues)
			return &pinecone.QueryVectorsResponse{
				Matches: []*pinecone.ScoredVector{
					{
						Vector: &pinecone.Vector{
							Id:     "doc1",
							Values: []float32{0.1, 0.2, 0.3, 0.4},
						},
						Score: 0.95,
					},
				},
			}, nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	results, err := store.Search(context.Background(), []float32{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{
		K:                 5,
		IncludeEmbeddings: true,
	})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Len(t, results[0].Embedding, 4)
	assert.InDelta(t, 0.1, results[0].Embedding[0], 0.001)
	assert.InDelta(t, 0.2, results[0].Embedding[1], 0.001)
	assert.InDelta(t, 0.3, results[0].Embedding[2], 0.001)
	assert.InDelta(t, 0.4, results[0].Embedding[3], 0.001)
}

func TestStore_Search_Error(t *testing.T) {
	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			return nil, errors.New("query failed")
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	_, err := store.Search(context.Background(), []float32{0.1, 0.2}, vectorstore.SearchOptions{K: 5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "search failed")
}

func TestStore_Delete(t *testing.T) {
	var capturedIDs []string

	idx := &mockIndexConnection{
		deleteVectorsByIdFunc: func(ctx context.Context, ids []string) error {
			capturedIDs = ids
			return nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	err := store.Delete(context.Background(), []string{"doc1", "doc2"}, "")
	require.NoError(t, err)

	assert.Equal(t, []string{"doc1", "doc2"}, capturedIDs)
}

func TestStore_Delete_EmptyIDs(t *testing.T) {
	idx := &mockIndexConnection{
		deleteVectorsByIdFunc: func(ctx context.Context, ids []string) error {
			t.Fatal("should not be called")
			return nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	err := store.Delete(context.Background(), []string{}, "")
	assert.NoError(t, err)

	err = store.Delete(context.Background(), nil, "")
	assert.NoError(t, err)
}

func TestStore_Delete_Error(t *testing.T) {
	idx := &mockIndexConnection{
		deleteVectorsByIdFunc: func(ctx context.Context, ids []string) error {
			return errors.New("delete failed")
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	err := store.Delete(context.Background(), []string{"doc1"}, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete vectors")
}

func TestStore_CreateIndex(t *testing.T) {
	var capturedRequest *pinecone.CreateServerlessIndexRequest

	client := &mockClient{
		createServerlessIndexFunc: func(ctx context.Context, in *pinecone.CreateServerlessIndexRequest) (*pinecone.Index, error) {
			capturedRequest = in
			return &pinecone.Index{Name: in.Name}, nil
		},
	}

	store := New(client, &mockIndexConnection{}, "test-index",
		WithCloud("aws"),
		WithRegion("us-east-1"),
	)

	err := store.CreateIndex(context.Background(), "new-index", 128, embedding.Cosine)
	require.NoError(t, err)

	assert.Equal(t, "new-index", capturedRequest.Name)
	assert.Equal(t, int32(128), capturedRequest.Dimension)
	assert.Equal(t, pinecone.Cosine, capturedRequest.Metric)
	assert.Equal(t, pinecone.Cloud("aws"), capturedRequest.Cloud)
	assert.Equal(t, "us-east-1", capturedRequest.Region)
}

func TestStore_CreateIndex_Euclidean(t *testing.T) {
	var capturedRequest *pinecone.CreateServerlessIndexRequest

	client := &mockClient{
		createServerlessIndexFunc: func(ctx context.Context, in *pinecone.CreateServerlessIndexRequest) (*pinecone.Index, error) {
			capturedRequest = in
			return &pinecone.Index{Name: in.Name}, nil
		},
	}

	store := New(client, &mockIndexConnection{}, "test-index")

	err := store.CreateIndex(context.Background(), "new-index", 256, embedding.Euclidean)
	require.NoError(t, err)

	assert.Equal(t, pinecone.Euclidean, capturedRequest.Metric)
}

func TestStore_CreateIndex_Error(t *testing.T) {
	client := &mockClient{
		createServerlessIndexFunc: func(ctx context.Context, in *pinecone.CreateServerlessIndexRequest) (*pinecone.Index, error) {
			return nil, errors.New("create failed")
		},
	}

	store := New(client, &mockIndexConnection{}, "test-index")

	err := store.CreateIndex(context.Background(), "new-index", 128, embedding.Cosine)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create index")
}

func TestStore_DeleteIndex(t *testing.T) {
	var capturedName string

	client := &mockClient{
		deleteIndexFunc: func(ctx context.Context, name string) error {
			capturedName = name
			return nil
		},
	}

	store := New(client, &mockIndexConnection{}, "test-index")

	err := store.DeleteIndex(context.Background(), "old-index")
	require.NoError(t, err)

	assert.Equal(t, "old-index", capturedName)
}

func TestStore_DeleteIndex_Error(t *testing.T) {
	client := &mockClient{
		deleteIndexFunc: func(ctx context.Context, name string) error {
			return errors.New("delete failed")
		},
	}

	store := New(client, &mockIndexConnection{}, "test-index")

	err := store.DeleteIndex(context.Background(), "old-index")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete index")
}

func TestStore_ListIndexes(t *testing.T) {
	client := &mockClient{
		listIndexesFunc: func(ctx context.Context) ([]*pinecone.Index, error) {
			return []*pinecone.Index{
				{Name: "index1"},
				{Name: "index2"},
				{Name: "index3"},
			}, nil
		},
	}

	store := New(client, &mockIndexConnection{}, "test-index")

	indexes, err := store.ListIndexes(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"index1", "index2", "index3"}, indexes)
}

func TestStore_ListIndexes_Error(t *testing.T) {
	client := &mockClient{
		listIndexesFunc: func(ctx context.Context) ([]*pinecone.Index, error) {
			return nil, errors.New("list failed")
		},
	}

	store := New(client, &mockIndexConnection{}, "test-index")

	_, err := store.ListIndexes(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list indexes")
}

func TestStore_Close(t *testing.T) {
	closed := false

	idx := &mockIndexConnection{
		closeFunc: func() error {
			closed = true
			return nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")

	err := store.Close()
	assert.NoError(t, err)
	assert.True(t, closed)
}

func TestStore_Close_NilIndex(t *testing.T) {
	store := &Store{
		client: &mockClient{},
		idx:    nil,
	}

	err := store.Close()
	assert.NoError(t, err)
}

func TestToPineconeMetric(t *testing.T) {
	tests := []struct {
		input    embedding.Metric
		expected pinecone.IndexMetric
	}{
		{embedding.Cosine, pinecone.Cosine},
		{embedding.Euclidean, pinecone.Euclidean},
		{embedding.DotProduct, pinecone.Dotproduct},
		{embedding.Metric(99), pinecone.Cosine}, // unknown metric defaults to cosine
	}

	for _, tt := range tests {
		result := toPineconeMetric(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

// mockSparseEncoder implements SparseEncoder for testing.
type mockSparseEncoder struct {
	encodeFunc func(text string) ([]uint32, []float32, error)
}

func (m *mockSparseEncoder) Encode(text string) ([]uint32, []float32, error) {
	if m.encodeFunc != nil {
		return m.encodeFunc(text)
	}
	// Default: simple mock encoding
	return []uint32{0, 1, 2}, []float32{0.5, 0.3, 0.2}, nil
}

func TestStore_SearchHybrid(t *testing.T) {
	var capturedRequest *pinecone.QueryByVectorValuesRequest

	metadata, _ := structpb.NewStruct(map[string]any{
		"content":   "Hello world",
		"timestamp": float64(1234567890),
	})

	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			capturedRequest = in
			return &pinecone.QueryVectorsResponse{
				Matches: []*pinecone.ScoredVector{
					{
						Vector: &pinecone.Vector{
							Id:       "doc1",
							Values:   []float32{0.1, 0.2, 0.3},
							Metadata: metadata,
						},
						Score: 0.92,
					},
				},
			}, nil
		},
	}

	sparseEncoder := &mockSparseEncoder{}
	store := New(&mockClient{}, idx, "test-index",
		WithSparseEncoder(sparseEncoder),
	)

	results, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         0.5,
		},
	)
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
	assert.Equal(t, "Hello world", results[0].Content)
	assert.InDelta(t, 0.92, results[0].Score, 0.0001)

	// Verify sparse values were included
	assert.NotNil(t, capturedRequest.SparseValues)
	assert.Equal(t, []uint32{0, 1, 2}, capturedRequest.SparseValues.Indices)
	assert.Equal(t, []float32{0.5, 0.3, 0.2}, capturedRequest.SparseValues.Values)
}

func TestStore_SearchHybrid_NoSparseEncoder(t *testing.T) {
	// Without sparse encoder, should fall back to regular Search
	searchCalled := false

	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			searchCalled = true
			// Verify no sparse values when no encoder
			assert.Nil(t, in.SparseValues)
			return &pinecone.QueryVectorsResponse{Matches: []*pinecone.ScoredVector{}}, nil
		},
	}

	store := New(&mockClient{}, idx, "test-index")
	// No sparse encoder configured

	_, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         0.5,
		},
	)
	require.NoError(t, err)
	assert.True(t, searchCalled)
}

func TestStore_SearchHybrid_PureVector(t *testing.T) {
	// When alpha=1.0, should use regular Search (no sparse values)
	var capturedRequest *pinecone.QueryByVectorValuesRequest

	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			capturedRequest = in
			return &pinecone.QueryVectorsResponse{Matches: []*pinecone.ScoredVector{}}, nil
		},
	}

	sparseEncoder := &mockSparseEncoder{}
	store := New(&mockClient{}, idx, "test-index",
		WithSparseEncoder(sparseEncoder),
	)

	_, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         1.0, // Pure vector
		},
	)
	require.NoError(t, err)

	// Should fall back to regular search (no sparse values)
	assert.Nil(t, capturedRequest.SparseValues)
}

func TestStore_SearchHybrid_EncoderError(t *testing.T) {
	idx := &mockIndexConnection{}

	sparseEncoder := &mockSparseEncoder{
		encodeFunc: func(text string) ([]uint32, []float32, error) {
			return nil, nil, errors.New("encoding failed")
		},
	}
	store := New(&mockClient{}, idx, "test-index",
		WithSparseEncoder(sparseEncoder),
	)

	_, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         0.5,
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to encode sparse vector")
}

func TestStore_SearchHybrid_QueryError(t *testing.T) {
	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			return nil, errors.New("query failed")
		},
	}

	sparseEncoder := &mockSparseEncoder{}
	store := New(&mockClient{}, idx, "test-index",
		WithSparseEncoder(sparseEncoder),
	)

	_, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         0.5,
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hybrid search failed")
}

func TestStore_SearchHybrid_WithFilter(t *testing.T) {
	var capturedRequest *pinecone.QueryByVectorValuesRequest

	idx := &mockIndexConnection{
		queryByVectorValuesFunc: func(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error) {
			capturedRequest = in
			return &pinecone.QueryVectorsResponse{Matches: []*pinecone.ScoredVector{}}, nil
		},
	}

	sparseEncoder := &mockSparseEncoder{}
	store := New(&mockClient{}, idx, "test-index",
		WithSparseEncoder(sparseEncoder),
	)

	_, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{
				K:      10,
				Filter: map[string]any{"category": "test"},
			},
			Alpha: 0.5,
		},
	)
	require.NoError(t, err)
	assert.NotNil(t, capturedRequest.MetadataFilter)
}

func TestWithSparseEncoder(t *testing.T) {
	encoder := &mockSparseEncoder{}
	opts := &Options{}

	WithSparseEncoder(encoder)(opts)

	assert.Equal(t, encoder, opts.SparseEncoder)
}
