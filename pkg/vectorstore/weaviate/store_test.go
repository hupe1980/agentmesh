package weaviate

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"
)

// mockClient implements the Client interface for testing.
type mockClient struct {
	classExistsFunc  func(ctx context.Context, className string) (bool, error)
	createClassFunc  func(ctx context.Context, class *models.Class) error
	deleteClassFunc  func(ctx context.Context, className string) error
	getSchemaFunc    func(ctx context.Context) (*models.Schema, error)
	batchObjectsFunc func(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error)
	deleteObjectFunc func(ctx context.Context, className, id string) error
	graphQLQueryFunc func(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error)
}

func (m *mockClient) ClassExists(ctx context.Context, className string) (bool, error) {
	if m.classExistsFunc != nil {
		return m.classExistsFunc(ctx, className)
	}
	return false, nil
}

func (m *mockClient) CreateClass(ctx context.Context, class *models.Class) error {
	if m.createClassFunc != nil {
		return m.createClassFunc(ctx, class)
	}
	return nil
}

func (m *mockClient) DeleteClass(ctx context.Context, className string) error {
	if m.deleteClassFunc != nil {
		return m.deleteClassFunc(ctx, className)
	}
	return nil
}

func (m *mockClient) GetSchema(ctx context.Context) (*models.Schema, error) {
	if m.getSchemaFunc != nil {
		return m.getSchemaFunc(ctx)
	}
	return &models.Schema{}, nil
}

func (m *mockClient) BatchObjects(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error) {
	if m.batchObjectsFunc != nil {
		return m.batchObjectsFunc(ctx, objects)
	}
	resp := make([]models.ObjectsGetResponse, len(objects))
	for i := range objects {
		resp[i] = models.ObjectsGetResponse{}
	}
	return resp, nil
}

func (m *mockClient) DeleteObject(ctx context.Context, className, id string) error {
	if m.deleteObjectFunc != nil {
		return m.deleteObjectFunc(ctx, className, id)
	}
	return nil
}

func (m *mockClient) GraphQLQuery(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error) {
	if m.graphQLQueryFunc != nil {
		return m.graphQLQueryFunc(ctx, className, fields, nearVector, limit, where)
	}
	return &models.GraphQLResponse{}, nil
}

func TestNewFromClient(t *testing.T) {
	client := &mockClient{}
	store := NewFromClient(client)

	assert.NotNil(t, store)
	assert.Equal(t, "Document", store.opts.ClassName)
	assert.Equal(t, embedding.Cosine, store.opts.Metric)
}

func TestNew_WithOptions(t *testing.T) {
	client := &mockClient{}
	store := NewFromClient(client,
		WithClassName("TestClass"),
		WithMetric(embedding.Euclidean),
	)

	assert.Equal(t, "TestClass", store.opts.ClassName)
	assert.Equal(t, embedding.Euclidean, store.opts.Metric)
}

func TestStore_EnsureClass_AlreadyExists(t *testing.T) {
	client := &mockClient{
		classExistsFunc: func(ctx context.Context, className string) (bool, error) {
			assert.Equal(t, "TestClass", className)
			return true, nil
		},
		createClassFunc: func(ctx context.Context, class *models.Class) error {
			t.Fatal("should not be called when class exists")
			return nil
		},
	}

	store := NewFromClient(client, WithClassName("TestClass"))
	err := store.EnsureClass(context.Background(), "TestClass", embedding.Cosine)
	assert.NoError(t, err)
}

func TestStore_EnsureClass_Create(t *testing.T) {
	var capturedClass *models.Class

	client := &mockClient{
		classExistsFunc: func(ctx context.Context, className string) (bool, error) {
			return false, nil
		},
		createClassFunc: func(ctx context.Context, class *models.Class) error {
			capturedClass = class
			return nil
		},
	}

	store := NewFromClient(client)
	err := store.EnsureClass(context.Background(), "TestClass", embedding.Euclidean)
	require.NoError(t, err)

	assert.Equal(t, "TestClass", capturedClass.Class)
	assert.Equal(t, "l2-squared", capturedClass.VectorIndexConfig.(map[string]any)["distance"])
}

func TestStore_EnsureClass_Error(t *testing.T) {
	client := &mockClient{
		classExistsFunc: func(ctx context.Context, className string) (bool, error) {
			return false, errors.New("connection failed")
		},
	}

	store := NewFromClient(client)
	err := store.EnsureClass(context.Background(), "TestClass", embedding.Cosine)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check class existence")
}

func TestStore_Add(t *testing.T) {
	var capturedObjects []*models.Object

	client := &mockClient{
		batchObjectsFunc: func(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error) {
			capturedObjects = objects
			resp := make([]models.ObjectsGetResponse, len(objects))
			return resp, nil
		},
	}

	store := NewFromClient(client, WithClassName("TestClass"))

	docs := []vectorstore.Document{
		{
			ID:        "doc1",
			Content:   "Hello world",
			Embedding: []float64{0.1, 0.2, 0.3, 0.4},
			Metadata:  map[string]any{"category": "greeting"},
		},
		{
			ID:        "doc2",
			Content:   "Goodbye world",
			Embedding: []float64{0.4, 0.3, 0.2, 0.1},
		},
	}

	err := store.Add(context.Background(), docs)
	require.NoError(t, err)

	assert.Len(t, capturedObjects, 2)
	assert.Equal(t, "TestClass", capturedObjects[0].Class)
	assert.Equal(t, "doc1", capturedObjects[0].Properties.(map[string]any)["docID"])
	assert.Equal(t, "Hello world", capturedObjects[0].Properties.(map[string]any)["content"])
}

func TestStore_Add_EmptyDocs(t *testing.T) {
	client := &mockClient{
		batchObjectsFunc: func(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error) {
			t.Fatal("should not be called")
			return nil, nil
		},
	}

	store := NewFromClient(client)

	err := store.Add(context.Background(), []vectorstore.Document{})
	assert.NoError(t, err)

	err = store.Add(context.Background(), nil)
	assert.NoError(t, err)
}

func TestStore_Add_WithNamespace(t *testing.T) {
	var capturedObjects []*models.Object

	client := &mockClient{
		batchObjectsFunc: func(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error) {
			capturedObjects = objects
			resp := make([]models.ObjectsGetResponse, len(objects))
			return resp, nil
		},
	}

	store := NewFromClient(client)

	docs := []vectorstore.Document{
		{ID: "doc1", Content: "Test", Embedding: []float64{0.1, 0.2}},
	}

	err := store.Add(context.Background(), docs, func(o *vectorstore.AddOptions) {
		o.Namespace = "test-namespace"
	})
	require.NoError(t, err)

	assert.Equal(t, "test-namespace", capturedObjects[0].Properties.(map[string]any)["namespace"])
}

func TestStore_Add_Error(t *testing.T) {
	client := &mockClient{
		batchObjectsFunc: func(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error) {
			return nil, errors.New("batch failed")
		},
	}

	store := NewFromClient(client)

	docs := []vectorstore.Document{
		{ID: "doc1", Embedding: []float64{0.1, 0.2}},
	}

	err := store.Add(context.Background(), docs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "batch insert failed")
}

func TestStore_Add_BatchError(t *testing.T) {
	errMsg := "validation error"
	client := &mockClient{
		batchObjectsFunc: func(ctx context.Context, objects []*models.Object) ([]models.ObjectsGetResponse, error) {
			return []models.ObjectsGetResponse{
				{
					Result: &models.ObjectsGetResponseAO2Result{
						Errors: &models.ErrorResponse{
							Error: []*models.ErrorResponseErrorItems0{
								{Message: errMsg},
							},
						},
					},
				},
			}, nil
		},
	}

	store := NewFromClient(client)

	docs := []vectorstore.Document{
		{ID: "doc1", Embedding: []float64{0.1, 0.2}},
	}

	err := store.Add(context.Background(), docs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insert error")
}

func TestStore_Search(t *testing.T) {
	client := &mockClient{
		graphQLQueryFunc: func(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error) {
			assert.Equal(t, "Document", className)
			assert.Equal(t, 5, limit)
			return &models.GraphQLResponse{
				Data: map[string]models.JSONObject{
					"Get": map[string]any{
						"Document": []any{
							map[string]any{
								"content":   "Hello world",
								"docID":     "doc1",
								"timestamp": float64(1234567890),
								"metadata":  "{}",
								"_additional": map[string]any{
									"id":       "uuid-1",
									"distance": 0.1,
								},
							},
						},
					},
				},
			}, nil
		},
	}

	store := NewFromClient(client)

	results, err := store.Search(context.Background(), []float64{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{K: 5})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
	assert.Equal(t, "Hello world", results[0].Content)
	assert.InDelta(t, 0.9, results[0].Score, 0.001)
}

func TestStore_Search_WithMinScore(t *testing.T) {
	client := &mockClient{
		graphQLQueryFunc: func(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error) {
			return &models.GraphQLResponse{
				Data: map[string]models.JSONObject{
					"Get": map[string]any{
						"Document": []any{
							map[string]any{
								"docID": "doc1",
								"_additional": map[string]any{
									"distance": 0.05, // score = 0.95
								},
							},
							map[string]any{
								"docID": "doc2",
								"_additional": map[string]any{
									"distance": 0.5, // score = 0.5
								},
							},
						},
					},
				},
			}, nil
		},
	}

	store := NewFromClient(client)

	results, err := store.Search(context.Background(), []float64{0.1, 0.2}, vectorstore.SearchOptions{
		K:        10,
		MinScore: 0.8,
	})
	require.NoError(t, err)

	assert.Len(t, results, 1)
	assert.Equal(t, "doc1", results[0].ID)
}

func TestStore_Search_WithNamespace(t *testing.T) {
	var capturedWhere *filters.WhereBuilder

	client := &mockClient{
		graphQLQueryFunc: func(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error) {
			capturedWhere = where
			return &models.GraphQLResponse{
				Data: map[string]models.JSONObject{
					"Get": map[string]any{
						"Document": []any{},
					},
				},
			}, nil
		},
	}

	store := NewFromClient(client)

	_, err := store.Search(context.Background(), []float64{0.1, 0.2}, vectorstore.SearchOptions{
		K:         5,
		Namespace: "test-namespace",
	})
	require.NoError(t, err)

	assert.NotNil(t, capturedWhere)
}

func TestStore_Search_Error(t *testing.T) {
	client := &mockClient{
		graphQLQueryFunc: func(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error) {
			return nil, errors.New("query failed")
		},
	}

	store := NewFromClient(client)

	_, err := store.Search(context.Background(), []float64{0.1, 0.2}, vectorstore.SearchOptions{K: 5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "search failed")
}

func TestStore_Search_GraphQLError(t *testing.T) {
	client := &mockClient{
		graphQLQueryFunc: func(ctx context.Context, className string, fields []graphql.Field, nearVector []float32, limit int, where *filters.WhereBuilder) (*models.GraphQLResponse, error) {
			return &models.GraphQLResponse{
				Errors: []*models.GraphQLError{
					{Message: "invalid query"},
				},
			}, nil
		},
	}

	store := NewFromClient(client)

	_, err := store.Search(context.Background(), []float64{0.1, 0.2}, vectorstore.SearchOptions{K: 5})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "search error")
}

func TestStore_Delete(t *testing.T) {
	deletedIDs := []string{}

	client := &mockClient{
		deleteObjectFunc: func(ctx context.Context, className, id string) error {
			assert.Equal(t, "Document", className)
			deletedIDs = append(deletedIDs, id)
			return nil
		},
	}

	store := NewFromClient(client)

	err := store.Delete(context.Background(), []string{"doc1", "doc2"}, "")
	require.NoError(t, err)

	assert.Len(t, deletedIDs, 2)
}

func TestStore_Delete_EmptyIDs(t *testing.T) {
	client := &mockClient{
		deleteObjectFunc: func(ctx context.Context, className, id string) error {
			t.Fatal("should not be called")
			return nil
		},
	}

	store := NewFromClient(client)

	err := store.Delete(context.Background(), []string{}, "")
	assert.NoError(t, err)

	err = store.Delete(context.Background(), nil, "")
	assert.NoError(t, err)
}

func TestStore_Delete_Error(t *testing.T) {
	client := &mockClient{
		deleteObjectFunc: func(ctx context.Context, className, id string) error {
			return errors.New("delete failed")
		},
	}

	store := NewFromClient(client)

	err := store.Delete(context.Background(), []string{"doc1"}, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete object")
}

func TestStore_CreateIndex(t *testing.T) {
	var capturedClass *models.Class

	client := &mockClient{
		classExistsFunc: func(ctx context.Context, className string) (bool, error) {
			return false, nil
		},
		createClassFunc: func(ctx context.Context, class *models.Class) error {
			capturedClass = class
			return nil
		},
	}

	store := NewFromClient(client)

	err := store.CreateIndex(context.Background(), "new-class", 128, embedding.DotProduct)
	require.NoError(t, err)

	assert.Equal(t, "new-class", capturedClass.Class)
	assert.Equal(t, "dot", capturedClass.VectorIndexConfig.(map[string]any)["distance"])
}

func TestStore_CreateIndex_Error(t *testing.T) {
	client := &mockClient{
		classExistsFunc: func(ctx context.Context, className string) (bool, error) {
			return false, nil
		},
		createClassFunc: func(ctx context.Context, class *models.Class) error {
			return errors.New("create failed")
		},
	}

	store := NewFromClient(client)

	err := store.CreateIndex(context.Background(), "new-class", 128, embedding.Cosine)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create class")
}

func TestStore_DeleteIndex(t *testing.T) {
	var deletedClassName string

	client := &mockClient{
		deleteClassFunc: func(ctx context.Context, className string) error {
			deletedClassName = className
			return nil
		},
	}

	store := NewFromClient(client)

	err := store.DeleteIndex(context.Background(), "old-class")
	require.NoError(t, err)

	assert.Equal(t, "old-class", deletedClassName)
}

func TestStore_DeleteIndex_Error(t *testing.T) {
	client := &mockClient{
		deleteClassFunc: func(ctx context.Context, className string) error {
			return errors.New("delete failed")
		},
	}

	store := NewFromClient(client)

	err := store.DeleteIndex(context.Background(), "old-class")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete class")
}

func TestStore_ListIndexes(t *testing.T) {
	client := &mockClient{
		getSchemaFunc: func(ctx context.Context) (*models.Schema, error) {
			return &models.Schema{
				Classes: []*models.Class{
					{Class: "Class1"},
					{Class: "Class2"},
					{Class: "Class3"},
				},
			}, nil
		},
	}

	store := NewFromClient(client)

	indexes, err := store.ListIndexes(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"Class1", "Class2", "Class3"}, indexes)
}

func TestStore_ListIndexes_Error(t *testing.T) {
	client := &mockClient{
		getSchemaFunc: func(ctx context.Context) (*models.Schema, error) {
			return nil, errors.New("get schema failed")
		},
	}

	store := NewFromClient(client)

	_, err := store.ListIndexes(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get schema")
}

func TestStore_Close(t *testing.T) {
	store := NewFromClient(&mockClient{})
	err := store.Close()
	assert.NoError(t, err)
}

func TestToWeaviateDistance(t *testing.T) {
	tests := []struct {
		input    embedding.Metric
		expected string
	}{
		{embedding.Cosine, "cosine"},
		{embedding.Euclidean, "l2-squared"},
		{embedding.DotProduct, "dot"},
		{embedding.Metric(99), "cosine"}, // unknown defaults to cosine
	}

	for _, tt := range tests {
		result := toWeaviateDistance(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestEncodeMetadata(t *testing.T) {
	result := encodeMetadata(map[string]any{
		"key": "value",
	})
	assert.Contains(t, result, `"key"`)
	assert.Contains(t, result, `"value"`)
}

func TestDecodeMetadata(t *testing.T) {
	result := decodeMetadata("{}")
	assert.Nil(t, result)

	result = decodeMetadata("")
	assert.Nil(t, result)
}
