package pgvector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPool is a mock implementation of Pool.
type mockPool struct {
	ExecFunc      func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryFunc     func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	SendBatchFunc func(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	CloseFunc     func()
}

func (m *mockPool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if m.ExecFunc != nil {
		return m.ExecFunc(ctx, sql, arguments...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockPool) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockPool) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	if m.SendBatchFunc != nil {
		return m.SendBatchFunc(ctx, b)
	}
	return &mockBatchResults{}
}

func (m *mockPool) Close() {
	if m.CloseFunc != nil {
		m.CloseFunc()
	}
}

// mockBatchResults is a mock implementation of pgx.BatchResults.
type mockBatchResults struct {
	ExecFunc  func() (pgconn.CommandTag, error)
	QueryFunc func() (pgx.Rows, error)
	CloseFunc func() error
	execCount int
	maxExec   int
}

func (m *mockBatchResults) Exec() (pgconn.CommandTag, error) {
	if m.ExecFunc != nil {
		m.execCount++
		return m.ExecFunc()
	}
	m.execCount++
	if m.maxExec > 0 && m.execCount > m.maxExec {
		return pgconn.CommandTag{}, errors.New("no more results")
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockBatchResults) Query() (pgx.Rows, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc()
	}
	return nil, nil
}

func (m *mockBatchResults) QueryRow() pgx.Row {
	return nil
}

func (m *mockBatchResults) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// mockRows is a mock implementation of pgx.Rows.
type mockRows struct {
	data       []map[string]any
	currentIdx int
	closed     bool
	scanFunc   func(dest ...any) error
	errFunc    func() error
}

func (m *mockRows) Close() {
	m.closed = true
}

func (m *mockRows) Err() error {
	if m.errFunc != nil {
		return m.errFunc()
	}
	return nil
}

func (m *mockRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (m *mockRows) Next() bool {
	if m.currentIdx < len(m.data) {
		m.currentIdx++
		return true
	}
	return false
}

func (m *mockRows) Scan(dest ...any) error {
	if m.scanFunc != nil {
		return m.scanFunc(dest...)
	}
	return nil
}

func (m *mockRows) Values() ([]any, error) {
	return nil, nil
}

func (m *mockRows) RawValues() [][]byte {
	return nil
}

func (m *mockRows) Conn() *pgx.Conn {
	return nil
}

func TestNewFromPool(t *testing.T) {
	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	assert.NotNil(t, store)
	assert.Equal(t, "documents", store.opts.TableName)
	assert.Equal(t, embedding.Cosine, store.opts.Metric)
	assert.True(t, store.opts.AutoCreateTable)
}

func TestNewFromPool_WithOptions(t *testing.T) {
	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool,
		WithTableName("test-table"),
		WithMetric(embedding.Euclidean),
		WithDimensions(128),
		WithAutoCreateTable(false),
	)
	require.NoError(t, err)

	assert.NotNil(t, store)
	assert.Equal(t, "test-table", store.opts.TableName)
	assert.Equal(t, embedding.Euclidean, store.opts.Metric)
	assert.Equal(t, 128, store.opts.Dimensions)
	assert.False(t, store.opts.AutoCreateTable)
}

func TestNewFromPool_ExtensionError(t *testing.T) {
	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("extension error")
		},
	}

	_, err := NewFromPool(context.Background(), pool)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extension")
}

func TestNewFromPool_AutoCreateTable(t *testing.T) {
	execCalls := []string{}

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			execCalls = append(execCalls, sql)
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool,
		WithDimensions(128),
		WithAutoCreateTable(true),
	)
	require.NoError(t, err)
	assert.NotNil(t, store)

	// Should have called: extension, create table, create index
	assert.GreaterOrEqual(t, len(execCalls), 2)
}

func TestStore_Add(t *testing.T) {
	batchResults := &mockBatchResults{
		maxExec: 2,
	}

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		SendBatchFunc: func(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
			return batchResults
		},
	}

	store, err := NewFromPool(context.Background(), pool, WithAutoCreateTable(false))
	require.NoError(t, err)

	docs := []vectorstore.Document{
		{
			ID:        "doc1",
			Content:   "Hello world",
			Embedding: []float32{0.1, 0.2, 0.3},
			Metadata:  map[string]any{"key": "value"},
		},
		{
			ID:        "doc2",
			Content:   "Test content",
			Embedding: []float32{0.4, 0.5, 0.6},
		},
	}

	err = store.Add(context.Background(), docs)
	require.NoError(t, err)
}

func TestStore_Add_EmptyDocs(t *testing.T) {
	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		SendBatchFunc: func(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
			t.Fatal("SendBatch should not be called for empty docs")
			return nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.Add(context.Background(), nil)
	require.NoError(t, err)
}

func TestStore_Add_WithNamespace(t *testing.T) {
	batchResults := &mockBatchResults{
		maxExec: 1,
	}

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		SendBatchFunc: func(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
			return batchResults
		},
	}

	store, err := NewFromPool(context.Background(), pool, WithAutoCreateTable(false))
	require.NoError(t, err)

	docs := []vectorstore.Document{
		{ID: "doc1", Content: "test", Embedding: []float32{0.1}},
	}

	err = store.Add(context.Background(), docs, func(o *vectorstore.AddOptions) {
		o.Namespace = "ns1"
	})
	require.NoError(t, err)
}

func TestStore_Add_BatchError(t *testing.T) {
	batchResults := &mockBatchResults{
		ExecFunc: func() (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("batch exec failed")
		},
	}

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		SendBatchFunc: func(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
			return batchResults
		},
	}

	store, err := NewFromPool(context.Background(), pool, WithAutoCreateTable(false))
	require.NoError(t, err)

	docs := []vectorstore.Document{
		{ID: "doc1", Embedding: []float32{0.1}},
	}

	err = store.Add(context.Background(), docs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to insert")
}

func TestStore_Delete(t *testing.T) {
	var capturedSQL string
	var capturedArgs []any

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			capturedSQL = sql
			capturedArgs = arguments
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.Delete(context.Background(), []string{"doc1", "doc2"}, "")
	require.NoError(t, err)

	assert.Contains(t, capturedSQL, "DELETE FROM")
	assert.Contains(t, capturedSQL, "documents")
	assert.Len(t, capturedArgs, 2)
}

func TestStore_Delete_EmptyIDs(t *testing.T) {
	execCalled := false

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			if sql != "CREATE EXTENSION IF NOT EXISTS vector" {
				execCalled = true
			}
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.Delete(context.Background(), nil, "")
	require.NoError(t, err)
	assert.False(t, execCalled, "Exec should not be called for empty IDs (except extension)")
}

func TestStore_Delete_Error(t *testing.T) {
	callCount := 0

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			callCount++
			if callCount > 1 { // First call is extension
				return pgconn.CommandTag{}, errors.New("delete failed")
			}
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.Delete(context.Background(), []string{"doc1"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete")
}

func TestStore_Search(t *testing.T) {
	rows := &mockRows{
		data: []map[string]any{
			{"id": "doc1", "content": "test", "distance": 0.1},
		},
		scanFunc: func(dest ...any) error {
			// Scan: id, content, metadata, namespace, created_at, distance
			if len(dest) >= 6 {
				if idPtr, ok := dest[0].(*string); ok {
					*idPtr = "doc1"
				}
				if contentPtr, ok := dest[1].(*string); ok {
					*contentPtr = "Hello world"
				}
				if metadataPtr, ok := dest[2].(*[]byte); ok {
					*metadataPtr = []byte(`{"key":"value"}`)
				}
				if nsPtr, ok := dest[3].(*string); ok {
					*nsPtr = ""
				}
				if distPtr, ok := dest[5].(*float64); ok {
					*distPtr = 0.1
				}
			}
			return nil
		},
	}

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	results, err := store.Search(context.Background(), []float32{0.1, 0.2}, vectorstore.SearchOptions{K: 10})
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestStore_Search_Error(t *testing.T) {
	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, errors.New("query failed")
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	_, err = store.Search(context.Background(), []float32{0.1}, vectorstore.SearchOptions{K: 10})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "search failed")
}

func TestStore_CreateIndex(t *testing.T) {
	execCalls := []string{}

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			execCalls = append(execCalls, sql)
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.CreateIndex(context.Background(), "test_index", 128, embedding.Cosine)
	require.NoError(t, err)

	// Should have CREATE TABLE and CREATE INDEX
	found := false
	for _, sql := range execCalls {
		if strings.Contains(sql, "CREATE TABLE") && strings.Contains(sql, "test_index") {
			found = true
			break
		}
	}
	assert.True(t, found, "Should have created table")
}

func TestStore_DeleteIndex(t *testing.T) {
	var capturedSQL string

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			capturedSQL = sql
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.DeleteIndex(context.Background(), "test_index")
	require.NoError(t, err)

	assert.Contains(t, capturedSQL, "DROP TABLE")
	assert.Contains(t, capturedSQL, "test_index")
}

func TestStore_DeleteIndex_Error(t *testing.T) {
	callCount := 0

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			callCount++
			if callCount > 1 { // First call is extension
				return pgconn.CommandTag{}, errors.New("drop failed")
			}
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.DeleteIndex(context.Background(), "test_index")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "drop")
}

func TestStore_ListIndexes(t *testing.T) {
	callCount := 0
	rows := &mockRows{
		data: []map[string]any{
			{"table_name": "table1"},
			{"table_name": "table2"},
		},
		scanFunc: func(dest ...any) error {
			callCount++
			if namePtr, ok := dest[0].(*string); ok {
				if callCount == 1 {
					*namePtr = "table1"
				} else {
					*namePtr = "table2"
				}
			}
			return nil
		},
	}

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return rows, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	names, err := store.ListIndexes(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"table1", "table2"}, names)
}

func TestStore_ListIndexes_Error(t *testing.T) {
	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		QueryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, errors.New("query failed")
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	_, err = store.ListIndexes(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list")
}

func TestStore_Close(t *testing.T) {
	closeCalled := false

	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
		CloseFunc: func() {
			closeCalled = true
		},
	}

	store, err := NewFromPool(context.Background(), pool)
	require.NoError(t, err)

	err = store.Close()
	require.NoError(t, err)
	assert.True(t, closeCalled)
}

func TestTableName(t *testing.T) {
	pool := &mockPool{
		ExecFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, nil
		},
	}

	store, err := NewFromPool(context.Background(), pool, WithTableName("mydata"))
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
			result := store.tableName(tt.namespace)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"documents", "documents"},
		{"my_table", "my_table"},
		{"Table123", "Table123"},
		{"drop table; --", "droptable"},
		{"table-name", "tablename"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeIdentifier(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetricToOpClass(t *testing.T) {
	tests := []struct {
		metric   embedding.Metric
		expected string
	}{
		{embedding.Cosine, "vector_cosine_ops"},
		{embedding.Euclidean, "vector_l2_ops"},
		{embedding.DotProduct, "vector_ip_ops"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := metricToOpClass(tt.metric)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetricToOperator(t *testing.T) {
	tests := []struct {
		metric   embedding.Metric
		expected string
	}{
		{embedding.Cosine, "<=>"},
		{embedding.Euclidean, "<->"},
		{embedding.DotProduct, "<#>"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := metricToOperator(tt.metric)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDistanceToScore(t *testing.T) {
	tests := []struct {
		name     string
		distance float64
		metric   embedding.Metric
		expected float64
	}{
		{"cosine_identical", 0.0, embedding.Cosine, 1.0},
		{"cosine_orthogonal", 1.0, embedding.Cosine, 0.0},
		{"euclidean_identical", 0.0, embedding.Euclidean, 1.0},
		{"euclidean_far", 1.0, embedding.Euclidean, 0.5},
		{"dot_product", -0.5, embedding.DotProduct, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := distanceToScore(tt.distance, tt.metric)
			assert.InDelta(t, tt.expected, result, 0.0001)
		})
	}
}
