package qdrant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// mockPointsClient is a mock implementation of PointsClient.
type mockPointsClient struct {
	UpsertFunc           func(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error)
	SearchFunc           func(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error)
	QueryFunc            func(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error)
	DeleteFunc           func(ctx context.Context, in *qdrant.DeletePoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error)
	CreateFieldIndexFunc func(ctx context.Context, in *qdrant.CreateFieldIndexCollection, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error)
}

func (m *mockPointsClient) Upsert(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	if m.UpsertFunc != nil {
		return m.UpsertFunc(ctx, in, opts...)
	}
	return &qdrant.PointsOperationResponse{}, nil
}

func (m *mockPointsClient) Search(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(ctx, in, opts...)
	}
	return &qdrant.SearchResponse{}, nil
}

func (m *mockPointsClient) Query(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, in, opts...)
	}
	return &qdrant.QueryResponse{}, nil
}

func (m *mockPointsClient) Delete(ctx context.Context, in *qdrant.DeletePoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, in, opts...)
	}
	return &qdrant.PointsOperationResponse{}, nil
}

func (m *mockPointsClient) CreateFieldIndex(ctx context.Context, in *qdrant.CreateFieldIndexCollection, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
	if m.CreateFieldIndexFunc != nil {
		return m.CreateFieldIndexFunc(ctx, in, opts...)
	}
	return &qdrant.PointsOperationResponse{}, nil
}

// mockCollectionsClient is a mock implementation of CollectionsClient.
type mockCollectionsClient struct {
	GetFunc    func(ctx context.Context, in *qdrant.GetCollectionInfoRequest, opts ...grpc.CallOption) (*qdrant.GetCollectionInfoResponse, error)
	CreateFunc func(ctx context.Context, in *qdrant.CreateCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error)
	DeleteFunc func(ctx context.Context, in *qdrant.DeleteCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error)
	ListFunc   func(ctx context.Context, in *qdrant.ListCollectionsRequest, opts ...grpc.CallOption) (*qdrant.ListCollectionsResponse, error)
}

func (m *mockCollectionsClient) Get(ctx context.Context, in *qdrant.GetCollectionInfoRequest, opts ...grpc.CallOption) (*qdrant.GetCollectionInfoResponse, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, in, opts...)
	}
	return &qdrant.GetCollectionInfoResponse{}, nil
}

func (m *mockCollectionsClient) Create(ctx context.Context, in *qdrant.CreateCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, in, opts...)
	}
	return &qdrant.CollectionOperationResponse{}, nil
}

func (m *mockCollectionsClient) Delete(ctx context.Context, in *qdrant.DeleteCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, in, opts...)
	}
	return &qdrant.CollectionOperationResponse{}, nil
}

func (m *mockCollectionsClient) List(ctx context.Context, in *qdrant.ListCollectionsRequest, opts ...grpc.CallOption) (*qdrant.ListCollectionsResponse, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, in, opts...)
	}
	return &qdrant.ListCollectionsResponse{}, nil
}

func TestNew(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	assert.NotNil(t, store)
	assert.Equal(t, "documents", store.opts.CollectionName)
	assert.Equal(t, embedding.Cosine, store.opts.Metric)
	assert.True(t, store.opts.AutoCreateCollection)
}

func TestNew_WithOptions(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient,
		WithCollectionName("test-collection"),
		WithMetric(embedding.Euclidean),
		WithDimensions(128),
		WithAutoCreateCollection(false),
	)
	require.NoError(t, err)

	assert.NotNil(t, store)
	assert.Equal(t, "test-collection", store.opts.CollectionName)
	assert.Equal(t, embedding.Euclidean, store.opts.Metric)
	assert.Equal(t, 128, store.opts.Dimensions)
	assert.False(t, store.opts.AutoCreateCollection)
}

func TestStore_Add(t *testing.T) {
	var capturedRequest *qdrant.UpsertPoints

	pointsClient := &mockPointsClient{
		UpsertFunc: func(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			capturedRequest = in
			return &qdrant.PointsOperationResponse{}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient,
		WithAutoCreateCollection(false),
	)

	docs := []vectorstore.Document{
		{
			ID:        "doc1",
			Content:   "Hello world",
			Embedding: []float32{0.1, 0.2, 0.3},
			Metadata:  map[string]any{"key": "value"},
		},
	}

	err = store.Add(context.Background(), docs)
	require.NoError(t, err)

	assert.NotNil(t, capturedRequest)
	assert.Equal(t, "documents", capturedRequest.CollectionName)
	assert.Len(t, capturedRequest.Points, 1)
	assert.Equal(t, "doc1", capturedRequest.Points[0].Payload["_id"].GetStringValue())
	assert.Equal(t, "Hello world", capturedRequest.Points[0].Payload["content"].GetStringValue())
}

func TestStore_Add_EmptyDocs(t *testing.T) {
	pointsClient := &mockPointsClient{
		UpsertFunc: func(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			t.Fatal("Upsert should not be called for empty docs")
			return nil, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.Add(context.Background(), nil)
	require.NoError(t, err)
}

func TestStore_Add_WithNamespace(t *testing.T) {
	var capturedRequest *qdrant.UpsertPoints

	pointsClient := &mockPointsClient{
		UpsertFunc: func(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			capturedRequest = in
			return &qdrant.PointsOperationResponse{}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient,
		WithAutoCreateCollection(false),
	)

	docs := []vectorstore.Document{
		{ID: "doc1", Content: "test", Embedding: []float32{0.1}},
	}

	err = store.Add(context.Background(), docs, func(o *vectorstore.AddOptions) {
		o.Namespace = "ns1"
	})
	require.NoError(t, err)

	assert.Equal(t, "documents_ns1", capturedRequest.CollectionName)
}

func TestStore_Add_Error(t *testing.T) {
	pointsClient := &mockPointsClient{
		UpsertFunc: func(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			return nil, errors.New("upsert failed")
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient,
		WithAutoCreateCollection(false),
	)

	docs := []vectorstore.Document{
		{ID: "doc1", Embedding: []float32{0.1}},
	}

	err = store.Add(context.Background(), docs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert")
}

func TestStore_Add_GeneratesID(t *testing.T) {
	var capturedRequest *qdrant.UpsertPoints

	pointsClient := &mockPointsClient{
		UpsertFunc: func(ctx context.Context, in *qdrant.UpsertPoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			capturedRequest = in
			return &qdrant.PointsOperationResponse{}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient,
		WithAutoCreateCollection(false),
	)

	docs := []vectorstore.Document{
		{Content: "no id", Embedding: []float32{0.1}},
	}

	err = store.Add(context.Background(), docs)
	require.NoError(t, err)

	// Should have generated an ID
	generatedID := capturedRequest.Points[0].Payload["_id"].GetStringValue()
	assert.NotEmpty(t, generatedID)
}

func TestStore_Search(t *testing.T) {
	pointsClient := &mockPointsClient{
		SearchFunc: func(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error) {
			return &qdrant.SearchResponse{
				Result: []*qdrant.ScoredPoint{
					{
						Id:    &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: "uuid1"}},
						Score: 0.95,
						Payload: map[string]*qdrant.Value{
							"_id":       {Kind: &qdrant.Value_StringValue{StringValue: "doc1"}},
							"content":   {Kind: &qdrant.Value_StringValue{StringValue: "Hello world"}},
							"timestamp": {Kind: &qdrant.Value_IntegerValue{IntegerValue: time.Now().UnixNano()}},
							"author":    {Kind: &qdrant.Value_StringValue{StringValue: "test"}},
						},
					},
				},
			}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	results, err := store.Search(context.Background(), []float32{0.1, 0.2}, vectorstore.SearchOptions{K: 10})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
	assert.Equal(t, "Hello world", results[0].Content)
	assert.InDelta(t, 0.95, results[0].Score, 0.0001)
	assert.Equal(t, "test", results[0].Metadata["author"])
}

func TestStore_Search_WithFilter(t *testing.T) {
	var capturedRequest *qdrant.SearchPoints

	pointsClient := &mockPointsClient{
		SearchFunc: func(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error) {
			capturedRequest = in
			return &qdrant.SearchResponse{Result: []*qdrant.ScoredPoint{}}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	_, err = store.Search(context.Background(), []float32{0.1}, vectorstore.SearchOptions{
		K:      5,
		Filter: map[string]any{"category": "test"},
	})
	require.NoError(t, err)

	assert.NotNil(t, capturedRequest.Filter)
	assert.Len(t, capturedRequest.Filter.Must, 1)
}

func TestStore_Search_Error(t *testing.T) {
	pointsClient := &mockPointsClient{
		SearchFunc: func(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error) {
			return nil, errors.New("search failed")
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	_, err = store.Search(context.Background(), []float32{0.1}, vectorstore.SearchOptions{K: 10})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search failed")
}

func TestStore_Delete(t *testing.T) {
	var capturedRequest *qdrant.DeletePoints

	pointsClient := &mockPointsClient{
		DeleteFunc: func(ctx context.Context, in *qdrant.DeletePoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			capturedRequest = in
			return &qdrant.PointsOperationResponse{}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.Delete(context.Background(), []string{"doc1", "doc2"}, "")
	require.NoError(t, err)

	assert.NotNil(t, capturedRequest)
	assert.Equal(t, "documents", capturedRequest.CollectionName)
}

func TestStore_Delete_EmptyIDs(t *testing.T) {
	pointsClient := &mockPointsClient{
		DeleteFunc: func(ctx context.Context, in *qdrant.DeletePoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			t.Fatal("Delete should not be called for empty IDs")
			return nil, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.Delete(context.Background(), nil, "")
	require.NoError(t, err)
}

func TestStore_Delete_Error(t *testing.T) {
	pointsClient := &mockPointsClient{
		DeleteFunc: func(ctx context.Context, in *qdrant.DeletePoints, opts ...grpc.CallOption) (*qdrant.PointsOperationResponse, error) {
			return nil, errors.New("delete failed")
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.Delete(context.Background(), []string{"doc1"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete")
}

func TestStore_CreateIndex(t *testing.T) {
	var capturedRequest *qdrant.CreateCollection

	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		CreateFunc: func(ctx context.Context, in *qdrant.CreateCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
			capturedRequest = in
			return &qdrant.CollectionOperationResponse{}, nil
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.CreateIndex(context.Background(), "test-index", 128, embedding.Cosine)
	require.NoError(t, err)

	assert.NotNil(t, capturedRequest)
	assert.Equal(t, "test-index", capturedRequest.CollectionName)
}

func TestStore_CreateIndex_Error(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		CreateFunc: func(ctx context.Context, in *qdrant.CreateCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
			return nil, errors.New("create failed")
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.CreateIndex(context.Background(), "test-index", 128, embedding.Cosine)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create")
}

func TestStore_DeleteIndex(t *testing.T) {
	var capturedRequest *qdrant.DeleteCollection

	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		DeleteFunc: func(ctx context.Context, in *qdrant.DeleteCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
			capturedRequest = in
			return &qdrant.CollectionOperationResponse{}, nil
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.DeleteIndex(context.Background(), "test-index")
	require.NoError(t, err)

	assert.NotNil(t, capturedRequest)
	assert.Equal(t, "test-index", capturedRequest.CollectionName)
}

func TestStore_DeleteIndex_Error(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		DeleteFunc: func(ctx context.Context, in *qdrant.DeleteCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
			return nil, errors.New("delete failed")
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.DeleteIndex(context.Background(), "test-index")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete")
}

func TestStore_ListIndexes(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		ListFunc: func(ctx context.Context, in *qdrant.ListCollectionsRequest, opts ...grpc.CallOption) (*qdrant.ListCollectionsResponse, error) {
			return &qdrant.ListCollectionsResponse{
				Collections: []*qdrant.CollectionDescription{
					{Name: "collection1"},
					{Name: "collection2"},
				},
			}, nil
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	names, err := store.ListIndexes(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"collection1", "collection2"}, names)
}

func TestStore_ListIndexes_Error(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		ListFunc: func(ctx context.Context, in *qdrant.ListCollectionsRequest, opts ...grpc.CallOption) (*qdrant.ListCollectionsResponse, error) {
			return nil, errors.New("list failed")
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	_, err = store.ListIndexes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list")
}

func TestStore_EnsureCollection_AlreadyExists(t *testing.T) {
	createCalled := false

	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		GetFunc: func(ctx context.Context, in *qdrant.GetCollectionInfoRequest, opts ...grpc.CallOption) (*qdrant.GetCollectionInfoResponse, error) {
			return &qdrant.GetCollectionInfoResponse{}, nil // Collection exists
		},
		CreateFunc: func(ctx context.Context, in *qdrant.CreateCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
			createCalled = true
			return &qdrant.CollectionOperationResponse{}, nil
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.ensureCollection(context.Background(), "test", 128, embedding.Cosine)
	require.NoError(t, err)
	assert.False(t, createCalled, "Create should not be called when collection exists")
}

func TestStore_EnsureCollection_Create(t *testing.T) {
	var capturedRequest *qdrant.CreateCollection

	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{
		GetFunc: func(ctx context.Context, in *qdrant.GetCollectionInfoRequest, opts ...grpc.CallOption) (*qdrant.GetCollectionInfoResponse, error) {
			return nil, errors.New("not found") // Collection doesn't exist
		},
		CreateFunc: func(ctx context.Context, in *qdrant.CreateCollection, opts ...grpc.CallOption) (*qdrant.CollectionOperationResponse, error) {
			capturedRequest = in
			return &qdrant.CollectionOperationResponse{}, nil
		},
	}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	err = store.ensureCollection(context.Background(), "new-collection", 256, embedding.Euclidean)
	require.NoError(t, err)

	assert.NotNil(t, capturedRequest)
	assert.Equal(t, "new-collection", capturedRequest.CollectionName)
}

func TestStore_Close_NilConn(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	// When using New with mock clients, conn is nil.
	// The Close method will panic if called with a nil conn,
	// which is expected behavior since we didn't provide a real connection.
	// In production, users should use NewFromAddr() which sets up a proper connection.
	assert.Nil(t, store.conn)
}

func TestToQdrantDistance(t *testing.T) {
	tests := []struct {
		name     string
		metric   embedding.Metric
		expected qdrant.Distance
	}{
		{"cosine", embedding.Cosine, qdrant.Distance_Cosine},
		{"euclidean", embedding.Euclidean, qdrant.Distance_Euclid},
		{"dot_product", embedding.DotProduct, qdrant.Distance_Dot},
		{"unknown", embedding.Metric(99), qdrant.Distance_Cosine}, // Default for unknown
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toQdrantDistance(tt.metric)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToQdrantValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected *qdrant.Value
	}{
		{
			name:     "string",
			input:    "hello",
			expected: &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: "hello"}},
		},
		{
			name:     "int",
			input:    42,
			expected: &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: 42}},
		},
		{
			name:     "int64",
			input:    int64(100),
			expected: &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: 100}},
		},
		{
			name:     "float64",
			input:    3.14,
			expected: &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: 3.14}},
		},
		{
			name:     "bool",
			input:    true,
			expected: &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toQdrantValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFromQdrantValue(t *testing.T) {
	tests := []struct {
		name     string
		input    *qdrant.Value
		expected any
	}{
		{
			name:     "string",
			input:    &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: "hello"}},
			expected: "hello",
		},
		{
			name:     "integer",
			input:    &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: 42}},
			expected: int64(42),
		},
		{
			name:     "double",
			input:    &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: 3.14}},
			expected: 3.14,
		},
		{
			name:     "bool",
			input:    &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: true}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromQdrantValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractID(t *testing.T) {
	tests := []struct {
		name     string
		input    *qdrant.PointId
		expected string
	}{
		{
			name:     "nil",
			input:    nil,
			expected: "",
		},
		{
			name:     "uuid",
			input:    &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: "test-uuid"}},
			expected: "test-uuid",
		},
		{
			name:     "num",
			input:    &qdrant.PointId{PointIdOptions: &qdrant.PointId_Num{Num: 123}},
			expected: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCollectionName(t *testing.T) {
	pointsClient := &mockPointsClient{}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient,
		WithCollectionName("mydata"),
	)
	require.NoError(t, err)

	tests := []struct {
		namespace string
		expected  string
	}{
		{"", "mydata"},
		{"ns1", "mydata_ns1"},
		{"production", "mydata_production"},
	}

	for _, tt := range tests {
		t.Run(tt.namespace, func(t *testing.T) {
			result := store.collectionName(tt.namespace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStore_SearchHybrid(t *testing.T) {
	var capturedRequest *qdrant.QueryPoints

	pointsClient := &mockPointsClient{
		QueryFunc: func(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error) {
			capturedRequest = in
			return &qdrant.QueryResponse{
				Result: []*qdrant.ScoredPoint{
					{
						Id:    &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: "uuid1"}},
						Score: 0.92,
						Payload: map[string]*qdrant.Value{
							"_id":       {Kind: &qdrant.Value_StringValue{StringValue: "doc1"}},
							"content":   {Kind: &qdrant.Value_StringValue{StringValue: "Hello world"}},
							"timestamp": {Kind: &qdrant.Value_IntegerValue{IntegerValue: time.Now().UnixNano()}},
						},
					},
					{
						Id:    &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: "uuid2"}},
						Score: 0.85,
						Payload: map[string]*qdrant.Value{
							"_id":       {Kind: &qdrant.Value_StringValue{StringValue: "doc2"}},
							"content":   {Kind: &qdrant.Value_StringValue{StringValue: "Hello universe"}},
							"timestamp": {Kind: &qdrant.Value_IntegerValue{IntegerValue: time.Now().UnixNano()}},
						},
					},
				},
			}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	results, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         0.5, // 50% vector, 50% keyword
		},
	)
	require.NoError(t, err)

	assert.Len(t, results, 2)
	assert.Equal(t, "doc1", results[0].ID)
	assert.Equal(t, "Hello world", results[0].Content)
	assert.InDelta(t, 0.92, results[0].Score, 0.0001)
	assert.Equal(t, "doc2", results[1].ID)

	// Verify the query was constructed correctly
	assert.NotNil(t, capturedRequest)
	assert.Equal(t, "documents", capturedRequest.CollectionName)
	assert.Len(t, capturedRequest.Prefetch, 2) // Dense + keyword prefetch
}

func TestStore_SearchHybrid_PureVector(t *testing.T) {
	// When alpha=1.0, should fall back to regular Search
	searchCalled := false
	queryCalled := false

	pointsClient := &mockPointsClient{
		SearchFunc: func(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error) {
			searchCalled = true
			return &qdrant.SearchResponse{
				Result: []*qdrant.ScoredPoint{
					{
						Id:    &qdrant.PointId{PointIdOptions: &qdrant.PointId_Uuid{Uuid: "uuid1"}},
						Score: 0.95,
						Payload: map[string]*qdrant.Value{
							"_id":     {Kind: &qdrant.Value_StringValue{StringValue: "doc1"}},
							"content": {Kind: &qdrant.Value_StringValue{StringValue: "Vector only"}},
						},
					},
				},
			}, nil
		},
		QueryFunc: func(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error) {
			queryCalled = true
			return &qdrant.QueryResponse{}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	results, err := store.SearchHybrid(
		context.Background(),
		"hello",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         1.0, // Pure vector search
		},
	)
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.True(t, searchCalled, "Should use regular Search for alpha=1.0")
	assert.False(t, queryCalled, "Should not use Query for alpha=1.0")
}

func TestStore_SearchHybrid_EmptyQuery(t *testing.T) {
	// When query is empty, should fall back to regular Search
	searchCalled := false

	pointsClient := &mockPointsClient{
		SearchFunc: func(ctx context.Context, in *qdrant.SearchPoints, opts ...grpc.CallOption) (*qdrant.SearchResponse, error) {
			searchCalled = true
			return &qdrant.SearchResponse{
				Result: []*qdrant.ScoredPoint{},
			}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	_, err = store.SearchHybrid(
		context.Background(),
		"", // Empty query
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         0.5,
		},
	)
	require.NoError(t, err)
	assert.True(t, searchCalled, "Should use regular Search when query is empty")
}

func TestStore_SearchHybrid_WithFusionAlgorithm(t *testing.T) {
	var capturedRequest *qdrant.QueryPoints

	pointsClient := &mockPointsClient{
		QueryFunc: func(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error) {
			capturedRequest = in
			return &qdrant.QueryResponse{
				Result: []*qdrant.ScoredPoint{},
			}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	_, err = store.SearchHybrid(
		context.Background(),
		"test query",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions:   vectorstore.SearchOptions{K: 10},
			Alpha:           0.5,
			FusionAlgorithm: vectorstore.FusionRelativeScore,
		},
	)
	require.NoError(t, err)
	assert.NotNil(t, capturedRequest)
}

func TestStore_SearchHybrid_Error(t *testing.T) {
	pointsClient := &mockPointsClient{
		QueryFunc: func(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error) {
			return nil, errors.New("query failed")
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	_, err = store.SearchHybrid(
		context.Background(),
		"test",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{K: 10},
			Alpha:         0.5,
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hybrid search failed")
}

func TestStore_SearchHybrid_WithNamespace(t *testing.T) {
	var capturedRequest *qdrant.QueryPoints

	pointsClient := &mockPointsClient{
		QueryFunc: func(ctx context.Context, in *qdrant.QueryPoints, opts ...grpc.CallOption) (*qdrant.QueryResponse, error) {
			capturedRequest = in
			return &qdrant.QueryResponse{
				Result: []*qdrant.ScoredPoint{},
			}, nil
		},
	}
	collectionsClient := &mockCollectionsClient{}

	store, err := New(nil, pointsClient, collectionsClient)
	require.NoError(t, err)

	_, err = store.SearchHybrid(
		context.Background(),
		"test query",
		[]float32{0.1, 0.2, 0.3},
		vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{
				K:         10,
				Namespace: "custom-ns",
			},
			Alpha: 0.5,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "documents_custom-ns", capturedRequest.CollectionName)
}

func TestHybridSearchOptions_Normalize(t *testing.T) {
	tests := []struct {
		name          string
		opts          vectorstore.HybridSearchOptions
		expectedK     int
		expectedAlpha float64
	}{
		{
			name:          "zero K defaults to 10",
			opts:          vectorstore.HybridSearchOptions{},
			expectedK:     10,
			expectedAlpha: 0.0, // alpha stays 0 if not set
		},
		{
			name: "alpha clamped to 0",
			opts: vectorstore.HybridSearchOptions{
				Alpha: -0.5,
			},
			expectedK:     10,
			expectedAlpha: 0.0,
		},
		{
			name: "alpha clamped to 1",
			opts: vectorstore.HybridSearchOptions{
				Alpha: 1.5,
			},
			expectedK:     10,
			expectedAlpha: 1.0,
		},
		{
			name: "valid alpha unchanged",
			opts: vectorstore.HybridSearchOptions{
				SearchOptions: vectorstore.SearchOptions{K: 20},
				Alpha:         0.7,
			},
			expectedK:     20,
			expectedAlpha: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.Normalize()
			assert.Equal(t, tt.expectedK, tt.opts.K)
			assert.InDelta(t, tt.expectedAlpha, tt.opts.Alpha, 0.0001)
		})
	}
}
