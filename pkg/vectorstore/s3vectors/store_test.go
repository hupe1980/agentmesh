package s3vectors

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/types"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClient implements the Client interface for testing.
type mockClient struct {
	putVectorsFunc    func(ctx context.Context, params *s3vectors.PutVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.PutVectorsOutput, error)
	queryVectorsFunc  func(ctx context.Context, params *s3vectors.QueryVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.QueryVectorsOutput, error)
	deleteVectorsFunc func(ctx context.Context, params *s3vectors.DeleteVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteVectorsOutput, error)
	createIndexFunc   func(ctx context.Context, params *s3vectors.CreateIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.CreateIndexOutput, error)
	deleteIndexFunc   func(ctx context.Context, params *s3vectors.DeleteIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteIndexOutput, error)
	listIndexesFunc   func(ctx context.Context, params *s3vectors.ListIndexesInput, optFns ...func(*s3vectors.Options)) (*s3vectors.ListIndexesOutput, error)
}

func (m *mockClient) PutVectors(ctx context.Context, params *s3vectors.PutVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.PutVectorsOutput, error) {
	if m.putVectorsFunc != nil {
		return m.putVectorsFunc(ctx, params, optFns...)
	}
	return &s3vectors.PutVectorsOutput{}, nil
}

func (m *mockClient) QueryVectors(ctx context.Context, params *s3vectors.QueryVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.QueryVectorsOutput, error) {
	if m.queryVectorsFunc != nil {
		return m.queryVectorsFunc(ctx, params, optFns...)
	}
	return &s3vectors.QueryVectorsOutput{}, nil
}

func (m *mockClient) DeleteVectors(ctx context.Context, params *s3vectors.DeleteVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteVectorsOutput, error) {
	if m.deleteVectorsFunc != nil {
		return m.deleteVectorsFunc(ctx, params, optFns...)
	}
	return &s3vectors.DeleteVectorsOutput{}, nil
}

func (m *mockClient) CreateIndex(ctx context.Context, params *s3vectors.CreateIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.CreateIndexOutput, error) {
	if m.createIndexFunc != nil {
		return m.createIndexFunc(ctx, params, optFns...)
	}
	return &s3vectors.CreateIndexOutput{}, nil
}

func (m *mockClient) DeleteIndex(ctx context.Context, params *s3vectors.DeleteIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteIndexOutput, error) {
	if m.deleteIndexFunc != nil {
		return m.deleteIndexFunc(ctx, params, optFns...)
	}
	return &s3vectors.DeleteIndexOutput{}, nil
}

func (m *mockClient) ListIndexes(ctx context.Context, params *s3vectors.ListIndexesInput, optFns ...func(*s3vectors.Options)) (*s3vectors.ListIndexesOutput, error) {
	if m.listIndexesFunc != nil {
		return m.listIndexesFunc(ctx, params, optFns...)
	}
	return &s3vectors.ListIndexesOutput{}, nil
}

func TestNew(t *testing.T) {
	client := &mockClient{}
	store := New(client, "test-bucket", "test-index")

	assert.NotNil(t, store)
	assert.Equal(t, "test-bucket", store.vectorBucketName)
	assert.Equal(t, "test-index", store.indexName)
	assert.Equal(t, embedding.Cosine, store.opts.Metric)
}

func TestNew_WithOptions(t *testing.T) {
	client := &mockClient{}
	store := New(client, "test-bucket", "test-index",
		WithDimensions(128),
		WithMetric(embedding.Euclidean),
	)

	assert.NotNil(t, store)
	assert.Equal(t, 128, store.opts.Dimensions)
	assert.Equal(t, embedding.Euclidean, store.opts.Metric)
}

func TestStore_Add(t *testing.T) {
	var capturedInput *s3vectors.PutVectorsInput

	client := &mockClient{
		putVectorsFunc: func(ctx context.Context, params *s3vectors.PutVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.PutVectorsOutput, error) {
			capturedInput = params
			return &s3vectors.PutVectorsOutput{}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

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
			Embedding: []float32{0.5, 0.6, 0.7, 0.8},
		},
	}

	err := store.Add(context.Background(), docs)
	require.NoError(t, err)

	assert.Equal(t, "test-bucket", aws.ToString(capturedInput.VectorBucketName))
	assert.Equal(t, "test-index", aws.ToString(capturedInput.IndexName))
	assert.Len(t, capturedInput.Vectors, 2)
	assert.Equal(t, "doc1", aws.ToString(capturedInput.Vectors[0].Key))
	assert.Equal(t, "doc2", aws.ToString(capturedInput.Vectors[1].Key))
}

func TestStore_Add_EmptyDocs(t *testing.T) {
	client := &mockClient{
		putVectorsFunc: func(ctx context.Context, params *s3vectors.PutVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.PutVectorsOutput, error) {
			t.Fatal("PutVectors should not be called for empty docs")
			return nil, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.Add(context.Background(), []vectorstore.Document{})
	require.NoError(t, err)
}

func TestStore_Add_WithNamespace(t *testing.T) {
	var capturedInput *s3vectors.PutVectorsInput

	client := &mockClient{
		putVectorsFunc: func(ctx context.Context, params *s3vectors.PutVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.PutVectorsOutput, error) {
			capturedInput = params
			return &s3vectors.PutVectorsOutput{}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	docs := []vectorstore.Document{
		{ID: "doc1", Content: "Hello", Embedding: []float32{0.1, 0.2}},
	}

	err := store.Add(context.Background(), docs, func(o *vectorstore.AddOptions) {
		o.Namespace = "my-namespace"
	})
	require.NoError(t, err)

	assert.NotNil(t, capturedInput.Vectors[0].Metadata)
}

func TestStore_Add_Error(t *testing.T) {
	client := &mockClient{
		putVectorsFunc: func(ctx context.Context, params *s3vectors.PutVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.PutVectorsOutput, error) {
			return nil, errors.New("put failed")
		},
	}

	store := New(client, "test-bucket", "test-index")

	docs := []vectorstore.Document{
		{ID: "doc1", Content: "Hello", Embedding: []float32{0.1, 0.2}},
	}

	err := store.Add(context.Background(), docs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to put vectors")
}

func TestStore_Search(t *testing.T) {
	distance := float32(0.1)

	client := &mockClient{
		queryVectorsFunc: func(ctx context.Context, params *s3vectors.QueryVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.QueryVectorsOutput, error) {
			// Verify query params
			assert.Equal(t, "test-bucket", aws.ToString(params.VectorBucketName))
			assert.Equal(t, "test-index", aws.ToString(params.IndexName))
			assert.Equal(t, int32(5), aws.ToInt32(params.TopK))
			assert.True(t, params.ReturnMetadata)
			assert.True(t, params.ReturnDistance)

			return &s3vectors.QueryVectorsOutput{
				Vectors: []types.QueryOutputVector{
					{
						Key:      aws.String("doc1"),
						Distance: &distance,
						// Note: document.NewLazyDocument doesn't support UnmarshalSmithyDocument properly in tests
						// So we test without metadata here; metadata parsing is tested via integration tests
					},
				},
			}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	results, err := store.Search(context.Background(), []float32{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{K: 5})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
	assert.InDelta(t, 0.9, results[0].Score, 0.001) // 1.0 - 0.1 = 0.9
}

func TestStore_Search_WithMinScore(t *testing.T) {
	lowDistance := float32(0.1)
	highDistance := float32(0.5)

	client := &mockClient{
		queryVectorsFunc: func(ctx context.Context, params *s3vectors.QueryVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.QueryVectorsOutput, error) {
			return &s3vectors.QueryVectorsOutput{
				Vectors: []types.QueryOutputVector{
					{Key: aws.String("doc1"), Distance: &lowDistance},
					{Key: aws.String("doc2"), Distance: &highDistance},
				},
			}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	results, err := store.Search(context.Background(), []float32{0.1, 0.2}, vectorstore.SearchOptions{
		K:        10,
		MinScore: 0.8,
	})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
}

func TestStore_Search_WithNamespace(t *testing.T) {
	var capturedInput *s3vectors.QueryVectorsInput

	client := &mockClient{
		queryVectorsFunc: func(ctx context.Context, params *s3vectors.QueryVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.QueryVectorsOutput, error) {
			capturedInput = params
			return &s3vectors.QueryVectorsOutput{}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	_, err := store.Search(context.Background(), []float32{0.1, 0.2}, vectorstore.SearchOptions{
		K:         5,
		Namespace: "my-namespace",
	})
	require.NoError(t, err)

	assert.NotNil(t, capturedInput.Filter)
}

func TestStore_Search_Error(t *testing.T) {
	client := &mockClient{
		queryVectorsFunc: func(ctx context.Context, params *s3vectors.QueryVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.QueryVectorsOutput, error) {
			return nil, errors.New("query failed")
		},
	}

	store := New(client, "test-bucket", "test-index")

	_, err := store.Search(context.Background(), []float32{0.1, 0.2}, vectorstore.SearchOptions{K: 5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "query failed")
}

func TestStore_Delete(t *testing.T) {
	var capturedInput *s3vectors.DeleteVectorsInput

	client := &mockClient{
		deleteVectorsFunc: func(ctx context.Context, params *s3vectors.DeleteVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteVectorsOutput, error) {
			capturedInput = params
			return &s3vectors.DeleteVectorsOutput{}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.Delete(context.Background(), []string{"doc1", "doc2"}, "")
	require.NoError(t, err)

	assert.Equal(t, "test-bucket", aws.ToString(capturedInput.VectorBucketName))
	assert.Equal(t, "test-index", aws.ToString(capturedInput.IndexName))
	assert.Equal(t, []string{"doc1", "doc2"}, capturedInput.Keys)
}

func TestStore_Delete_EmptyIDs(t *testing.T) {
	client := &mockClient{
		deleteVectorsFunc: func(ctx context.Context, params *s3vectors.DeleteVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteVectorsOutput, error) {
			t.Fatal("DeleteVectors should not be called for empty IDs")
			return nil, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.Delete(context.Background(), []string{}, "")
	require.NoError(t, err)
}

func TestStore_Delete_Error(t *testing.T) {
	client := &mockClient{
		deleteVectorsFunc: func(ctx context.Context, params *s3vectors.DeleteVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteVectorsOutput, error) {
			return nil, errors.New("delete failed")
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.Delete(context.Background(), []string{"doc1"}, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete vectors")
}

func TestStore_CreateIndex(t *testing.T) {
	var capturedInput *s3vectors.CreateIndexInput

	client := &mockClient{
		createIndexFunc: func(ctx context.Context, params *s3vectors.CreateIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.CreateIndexOutput, error) {
			capturedInput = params
			return &s3vectors.CreateIndexOutput{}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.CreateIndex(context.Background(), "new-index", 128, embedding.Cosine)
	require.NoError(t, err)

	assert.Equal(t, "test-bucket", aws.ToString(capturedInput.VectorBucketName))
	assert.Equal(t, "new-index", aws.ToString(capturedInput.IndexName))
	assert.Equal(t, int32(128), aws.ToInt32(capturedInput.Dimension))
	assert.Equal(t, types.DistanceMetricCosine, capturedInput.DistanceMetric)
}

func TestStore_CreateIndex_Euclidean(t *testing.T) {
	var capturedInput *s3vectors.CreateIndexInput

	client := &mockClient{
		createIndexFunc: func(ctx context.Context, params *s3vectors.CreateIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.CreateIndexOutput, error) {
			capturedInput = params
			return &s3vectors.CreateIndexOutput{}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.CreateIndex(context.Background(), "new-index", 64, embedding.Euclidean)
	require.NoError(t, err)

	assert.Equal(t, types.DistanceMetricEuclidean, capturedInput.DistanceMetric)
}

func TestStore_CreateIndex_Error(t *testing.T) {
	client := &mockClient{
		createIndexFunc: func(ctx context.Context, params *s3vectors.CreateIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.CreateIndexOutput, error) {
			return nil, errors.New("create failed")
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.CreateIndex(context.Background(), "new-index", 128, embedding.Cosine)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create index")
}

func TestStore_DeleteIndex(t *testing.T) {
	var capturedInput *s3vectors.DeleteIndexInput

	client := &mockClient{
		deleteIndexFunc: func(ctx context.Context, params *s3vectors.DeleteIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteIndexOutput, error) {
			capturedInput = params
			return &s3vectors.DeleteIndexOutput{}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.DeleteIndex(context.Background(), "old-index")
	require.NoError(t, err)

	assert.Equal(t, "test-bucket", aws.ToString(capturedInput.VectorBucketName))
	assert.Equal(t, "old-index", aws.ToString(capturedInput.IndexName))
}

func TestStore_DeleteIndex_Error(t *testing.T) {
	client := &mockClient{
		deleteIndexFunc: func(ctx context.Context, params *s3vectors.DeleteIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteIndexOutput, error) {
			return nil, errors.New("delete failed")
		},
	}

	store := New(client, "test-bucket", "test-index")

	err := store.DeleteIndex(context.Background(), "old-index")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete index")
}

func TestStore_ListIndexes(t *testing.T) {
	client := &mockClient{
		listIndexesFunc: func(ctx context.Context, params *s3vectors.ListIndexesInput, optFns ...func(*s3vectors.Options)) (*s3vectors.ListIndexesOutput, error) {
			return &s3vectors.ListIndexesOutput{
				Indexes: []types.IndexSummary{
					{IndexName: aws.String("index1")},
					{IndexName: aws.String("index2")},
					{IndexName: aws.String("index3")},
				},
			}, nil
		},
	}

	store := New(client, "test-bucket", "test-index")

	indexes, err := store.ListIndexes(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"index1", "index2", "index3"}, indexes)
}

func TestStore_ListIndexes_Error(t *testing.T) {
	client := &mockClient{
		listIndexesFunc: func(ctx context.Context, params *s3vectors.ListIndexesInput, optFns ...func(*s3vectors.Options)) (*s3vectors.ListIndexesOutput, error) {
			return nil, errors.New("list failed")
		},
	}

	store := New(client, "test-bucket", "test-index")

	_, err := store.ListIndexes(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list indexes")
}

func TestStore_Close(t *testing.T) {
	client := &mockClient{}
	store := New(client, "test-bucket", "test-index")

	err := store.Close()
	assert.NoError(t, err)
}

func TestToS3VectorsMetric(t *testing.T) {
	tests := []struct {
		input    embedding.Metric
		expected types.DistanceMetric
	}{
		{embedding.Cosine, types.DistanceMetricCosine},
		{embedding.Euclidean, types.DistanceMetricEuclidean},
		{embedding.DotProduct, types.DistanceMetricCosine},
	}

	for _, tt := range tests {
		result := toS3VectorsMetric(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}
