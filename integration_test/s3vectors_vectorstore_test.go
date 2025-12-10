package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awss3vectors "github.com/aws/aws-sdk-go-v2/service/s3vectors"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/hupe1980/agentmesh/pkg/vectorstore/s3vectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestS3VectorsStore_BasicOperations tests basic S3 Vectors operations.
// This test requires AWS credentials and S3 Vectors access.
// Set AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, S3_VECTORS_BUCKET, and S3_VECTORS_INDEX.
func TestS3VectorsStore_BasicOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	bucketName := os.Getenv("S3_VECTORS_BUCKET")
	indexName := os.Getenv("S3_VECTORS_INDEX")

	if bucketName == "" || indexName == "" {
		t.Skip("skipping S3 Vectors test: S3_VECTORS_BUCKET or S3_VECTORS_INDEX not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Skipf("skipping S3 Vectors test: cannot load AWS config: %v", err)
	}

	// Create S3 Vectors client
	client := awss3vectors.NewFromConfig(cfg)

	store := s3vectors.New(client, bucketName, indexName,
		s3vectors.WithDimensions(4),
		s3vectors.WithMetric(embedding.Cosine),
	)
	defer store.Close()

	// Test Add
	docs := []vectorstore.Document{
		{
			ID:        "s3-doc1",
			Content:   "Hello world from S3 Vectors",
			Embedding: []float32{0.1, 0.2, 0.3, 0.4},
			Metadata:  map[string]any{"source": "test"},
		},
		{
			ID:        "s3-doc2",
			Content:   "Goodbye world from S3 Vectors",
			Embedding: []float32{0.4, 0.3, 0.2, 0.1},
			Metadata:  map[string]any{"source": "test"},
		},
	}

	err = store.Add(ctx, docs)
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(2 * time.Second)

	// Test Search
	results, err := store.Search(ctx, []float32{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{K: 5})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)

	// Test Delete
	err = store.Delete(ctx, []string{"s3-doc1", "s3-doc2"}, "")
	require.NoError(t, err)
}

// TestS3VectorsStore_Namespaces tests namespace support.
func TestS3VectorsStore_Namespaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	bucketName := os.Getenv("S3_VECTORS_BUCKET")
	indexName := os.Getenv("S3_VECTORS_INDEX")

	if bucketName == "" || indexName == "" {
		t.Skip("skipping S3 Vectors test: S3_VECTORS_BUCKET or S3_VECTORS_INDEX not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Skipf("skipping S3 Vectors test: cannot load AWS config: %v", err)
	}

	// Create S3 Vectors client
	client := awss3vectors.NewFromConfig(cfg)

	store := s3vectors.New(client, bucketName, indexName,
		s3vectors.WithDimensions(4),
	)
	defer store.Close()

	// Add documents to different namespaces
	docs1 := []vectorstore.Document{
		{ID: "ns1-doc1", Content: "Namespace 1 doc", Embedding: []float32{0.1, 0.2, 0.3, 0.4}},
	}
	docs2 := []vectorstore.Document{
		{ID: "ns2-doc1", Content: "Namespace 2 doc", Embedding: []float32{0.5, 0.6, 0.7, 0.8}},
	}

	err = store.Add(ctx, docs1, func(o *vectorstore.AddOptions) { o.Namespace = "namespace1" })
	require.NoError(t, err)

	err = store.Add(ctx, docs2, func(o *vectorstore.AddOptions) { o.Namespace = "namespace2" })
	require.NoError(t, err)

	// Wait for indexing
	time.Sleep(2 * time.Second)

	// Search in namespace1
	results, err := store.Search(ctx, []float32{0.1, 0.2, 0.3, 0.4}, vectorstore.SearchOptions{
		K:         5,
		Namespace: "namespace1",
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)

	// Clean up
	err = store.Delete(ctx, []string{"ns1-doc1"}, "namespace1")
	require.NoError(t, err)

	err = store.Delete(ctx, []string{"ns2-doc1"}, "namespace2")
	require.NoError(t, err)
}

// TestS3VectorsStore_IndexOperations tests index management.
func TestS3VectorsStore_IndexOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	bucketName := os.Getenv("S3_VECTORS_BUCKET")
	indexName := os.Getenv("S3_VECTORS_INDEX")

	if bucketName == "" || indexName == "" {
		t.Skip("skipping S3 Vectors test: S3_VECTORS_BUCKET or S3_VECTORS_INDEX not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		t.Skipf("skipping S3 Vectors test: cannot load AWS config: %v", err)
	}

	// Create S3 Vectors client
	client := awss3vectors.NewFromConfig(cfg)

	store := s3vectors.New(client, bucketName, indexName)
	defer store.Close()

	// List indexes
	indexes, err := store.ListIndexes(ctx)
	require.NoError(t, err)
	t.Logf("Found %d indexes: %v", len(indexes), indexes)

	_ = embedding.Cosine
}
