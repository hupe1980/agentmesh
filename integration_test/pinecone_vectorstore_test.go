package integration

import (
	"context"
	"os"
	"testing"
	"time"

	gopinecone "github.com/pinecone-io/go-pinecone/pinecone"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/hupe1980/agentmesh/pkg/vectorstore/pinecone"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createPineconeStore creates a Pinecone store for integration testing.
// Returns the store or nil if credentials are not available.
func createPineconeStore(t *testing.T, ctx context.Context) *pinecone.Store {
	t.Helper()

	apiKey := os.Getenv("PINECONE_API_KEY")
	indexName := os.Getenv("PINECONE_INDEX_NAME")

	if apiKey == "" || indexName == "" {
		t.Skip("skipping Pinecone test: PINECONE_API_KEY or PINECONE_INDEX_NAME not set")
	}

	// Create Pinecone client
	client, err := gopinecone.NewClient(gopinecone.NewClientParams{
		ApiKey: apiKey,
	})
	require.NoError(t, err)

	// Get index host
	host := os.Getenv("PINECONE_INDEX_HOST")
	if host == "" {
		idx, err := client.DescribeIndex(ctx, indexName)
		require.NoError(t, err)
		host = idx.Host
	}

	// Create index connection
	idxConn, err := client.Index(gopinecone.NewIndexConnParams{
		Host: host,
	})
	require.NoError(t, err)

	return pinecone.New(client, idxConn, indexName)
}

// TestPineconeVectorStore_BasicOperations tests basic Pinecone operations.
// This test requires a Pinecone API key and an existing index.
// Set PINECONE_API_KEY, PINECONE_INDEX_NAME, and optionally PINECONE_INDEX_HOST.
func TestPineconeVectorStore_BasicOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := createPineconeStore(t, ctx)
	defer store.Close()

	// Test Add
	docs := []vectorstore.Document{
		{
			ID:        "test-doc1",
			Content:   "Hello world",
			Embedding: make([]float64, 1536),
			Metadata:  map[string]any{"category": "test"},
		},
		{
			ID:        "test-doc2",
			Content:   "Goodbye world",
			Embedding: make([]float64, 1536),
			Metadata:  map[string]any{"category": "test"},
		},
	}
	docs[0].Embedding[0] = 0.1
	docs[1].Embedding[0] = 0.9

	err := store.Add(ctx, docs)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	queryVec := make([]float64, 1536)
	queryVec[0] = 0.1

	results, err := store.Search(ctx, queryVec, vectorstore.SearchOptions{K: 5})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)

	err = store.Delete(ctx, []string{"test-doc1", "test-doc2"}, "")
	require.NoError(t, err)
}

// TestPineconeVectorStore_Namespaces tests namespace support.
func TestPineconeVectorStore_Namespaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := createPineconeStore(t, ctx)
	defer store.Close()

	docs := []vectorstore.Document{
		{
			ID:        "ns-doc1",
			Content:   "Namespaced document",
			Embedding: make([]float64, 1536),
			Metadata:  map[string]any{"test": true},
		},
	}
	docs[0].Embedding[0] = 0.5

	// Add to namespace via AddOptions
	err := store.Add(ctx, docs, func(o *vectorstore.AddOptions) {
		o.Namespace = "test-namespace"
	})
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	queryVec := make([]float64, 1536)
	queryVec[0] = 0.5

	results, err := store.Search(ctx, queryVec, vectorstore.SearchOptions{
		K:         5,
		Namespace: "test-namespace",
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1)

	err = store.Delete(ctx, []string{"ns-doc1"}, "test-namespace")
	require.NoError(t, err)
}

// TestPineconeVectorStore_IndexOperations tests index management.
func TestPineconeVectorStore_IndexOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	store := createPineconeStore(t, ctx)
	defer store.Close()

	indexes, err := store.ListIndexes(ctx)
	require.NoError(t, err)
	t.Logf("Found %d indexes: %v", len(indexes), indexes)

	_ = embedding.Cosine
}
