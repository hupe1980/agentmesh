package pinecone

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/internal/floatconv"
	"github.com/hupe1980/agentmesh/internal/safeconv"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
	"github.com/pinecone-io/go-pinecone/pinecone"
	"google.golang.org/protobuf/types/known/structpb"
)

// Ensure Store implements the interfaces.
var (
	_ vectorstore.VectorStore  = (*Store)(nil)
	_ vectorstore.Indexer      = (*Store)(nil)
	_ vectorstore.TextSearcher = (*Store)(nil)
)

// SparseEncoder generates sparse vector representations from text.
// Used for hybrid search to create the keyword/BM25 component.
type SparseEncoder interface {
	// Encode generates a sparse vector from text.
	// Returns indices and values representing non-zero dimensions.
	Encode(text string) (indices []uint32, values []float32, err error)
}

// IndexConnection defines the interface for Pinecone index data operations.
// This interface allows for mocking in tests.
type IndexConnection interface {
	UpsertVectors(ctx context.Context, in []*pinecone.Vector) (uint32, error)
	QueryByVectorValues(ctx context.Context, in *pinecone.QueryByVectorValuesRequest) (*pinecone.QueryVectorsResponse, error)
	DeleteVectorsById(ctx context.Context, ids []string) error
	Close() error
}

// Client defines the interface for Pinecone control plane operations.
// This interface allows for mocking in tests.
type Client interface {
	CreateServerlessIndex(ctx context.Context, in *pinecone.CreateServerlessIndexRequest) (*pinecone.Index, error)
	DeleteIndex(ctx context.Context, name string) error
	ListIndexes(ctx context.Context) ([]*pinecone.Index, error)
}

// Options configures the Pinecone store.
type Options struct {
	// Metric specifies the distance metric. Default: Cosine
	Metric embedding.Metric

	// Cloud specifies the cloud provider for serverless indexes.
	Cloud string

	// Region specifies the region for serverless indexes.
	Region string

	// SparseEncoder generates sparse vectors for hybrid search.
	// If nil, SearchHybrid will perform pure vector search.
	SparseEncoder SparseEncoder
}

// Option configures a Store.
type Option func(*Options)

// WithMetric sets the distance metric.
func WithMetric(metric embedding.Metric) Option {
	return func(o *Options) {
		o.Metric = metric
	}
}

// WithCloud sets the cloud provider for serverless indexes.
func WithCloud(cloud string) Option {
	return func(o *Options) {
		o.Cloud = cloud
	}
}

// WithRegion sets the region for serverless indexes.
func WithRegion(region string) Option {
	return func(o *Options) {
		o.Region = region
	}
}

// WithSparseEncoder sets the sparse encoder for hybrid search.
// This is required for true hybrid search functionality.
func WithSparseEncoder(encoder SparseEncoder) Option {
	return func(o *Options) {
		o.SparseEncoder = encoder
	}
}

// Store is a Pinecone-backed VectorStore implementation.
type Store struct {
	client    Client
	idx       IndexConnection
	indexName string
	opts      Options
}

// New creates a new Pinecone vector store.
func New(client Client, idx IndexConnection, indexName string, optFns ...Option) *Store {
	opts := Options{
		Metric: embedding.Cosine,
		Cloud:  "aws",
		Region: "us-east-1",
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	return &Store{
		client:    client,
		idx:       idx,
		indexName: indexName,
		opts:      opts,
	}
}

// Add inserts or updates documents in the store.
func (s *Store) Add(ctx context.Context, docs []vectorstore.Document, optFns ...func(*vectorstore.AddOptions)) error {
	if len(docs) == 0 {
		return nil
	}

	opts := vectorstore.AddOptions{
		Upsert: true,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	// Build vectors
	vectors := make([]*pinecone.Vector, len(docs))
	now := time.Now()

	for i, doc := range docs {
		docID := doc.ID
		if docID == "" {
			docID = uuid.New().String()
		}

		// Build metadata
		metadata := make(map[string]any)
		metadata["content"] = doc.Content

		ts := doc.Timestamp
		if ts.IsZero() {
			ts = now.Add(time.Duration(i) * time.Nanosecond)
		}
		metadata["timestamp"] = ts.UnixNano()

		maps.Copy(metadata, doc.Metadata)

		metadataStruct, err := structpb.NewStruct(metadata)
		if err != nil {
			return fmt.Errorf("pinecone: failed to create metadata: %w", err)
		}

		vectors[i] = &pinecone.Vector{
			Id:       docID,
			Values:   floatconv.ToFloat32(doc.Embedding),
			Metadata: metadataStruct,
		}
	}

	_, err := s.idx.UpsertVectors(ctx, vectors)
	if err != nil {
		return fmt.Errorf("pinecone: failed to upsert vectors: %w", err)
	}

	return nil
}

// Search finds documents similar to the query embedding.
func (s *Store) Search(ctx context.Context, queryEmbedding embedding.Vector, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	opts.Normalize()

	// Build metadata filter
	filter, err := buildFilter(opts.Filter)
	if err != nil {
		return nil, err
	}

	resp, err := s.idx.QueryByVectorValues(ctx, &pinecone.QueryByVectorValuesRequest{
		Vector:          floatconv.ToFloat32(queryEmbedding),
		TopK:            safeconv.IntToUint32(opts.K),
		MetadataFilter:  filter,
		IncludeValues:   opts.IncludeEmbeddings,
		IncludeMetadata: true,
	})
	if err != nil {
		return nil, fmt.Errorf("pinecone: search failed: %w", err)
	}

	return convertMatches(resp.Matches, opts.MinScore, opts.IncludeEmbeddings), nil
}

// SearchHybrid performs a hybrid search combining dense vector similarity with sparse (keyword) search.
// This requires a SparseEncoder to be configured via WithSparseEncoder option.
// If no sparse encoder is configured, this falls back to regular vector search.
//
// The alpha parameter controls the balance:
//   - 0.0 = pure sparse/keyword search
//   - 1.0 = pure dense/vector search
//   - 0.5 = equal weighting (default)
//
// Note: Pinecone's hybrid search uses the sparse_vector field, which requires:
// 1. A sparse encoder (BM25, SPLADE, etc.) to generate sparse vectors
// 2. Vectors to be indexed with sparse values for hybrid search to work
func (s *Store) SearchHybrid(ctx context.Context, query string, queryEmbedding embedding.Vector, opts vectorstore.HybridSearchOptions) ([]vectorstore.Document, error) {
	opts.Normalize()

	// If no sparse encoder is configured, fall back to regular search
	if s.opts.SparseEncoder == nil {
		return s.Search(ctx, queryEmbedding, opts.SearchOptions)
	}

	// If alpha is 1.0, use pure vector search
	if opts.Alpha >= 1.0 {
		return s.Search(ctx, queryEmbedding, opts.SearchOptions)
	}

	// Generate sparse vector from query text
	indices, values, err := s.opts.SparseEncoder.Encode(query)
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to encode sparse vector: %w", err)
	}

	// Build metadata filter
	filter, err := buildFilter(opts.Filter)
	if err != nil {
		return nil, err
	}

	// Build sparse values
	sparseValues := &pinecone.SparseValues{
		Indices: indices,
		Values:  values,
	}

	// Execute hybrid query
	resp, err := s.idx.QueryByVectorValues(ctx, &pinecone.QueryByVectorValuesRequest{
		Vector:          floatconv.ToFloat32(queryEmbedding),
		SparseValues:    sparseValues,
		TopK:            safeconv.IntToUint32(opts.K),
		MetadataFilter:  filter,
		IncludeValues:   opts.IncludeEmbeddings,
		IncludeMetadata: true,
	})
	if err != nil {
		return nil, fmt.Errorf("pinecone: hybrid search failed: %w", err)
	}

	return convertMatches(resp.Matches, opts.MinScore, opts.IncludeEmbeddings), nil
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string, _ string) error {
	if len(ids) == 0 {
		return nil
	}

	err := s.idx.DeleteVectorsById(ctx, ids)
	if err != nil {
		return fmt.Errorf("pinecone: failed to delete vectors: %w", err)
	}

	return nil
}

// CreateIndex creates a new serverless index.
func (s *Store) CreateIndex(ctx context.Context, name string, dims int, metric embedding.Metric) error {
	_, err := s.client.CreateServerlessIndex(ctx, &pinecone.CreateServerlessIndexRequest{
		Name:      name,
		Dimension: safeconv.IntToInt32(dims),
		Metric:    toPineconeMetric(metric),
		Cloud:     pinecone.Cloud(s.opts.Cloud),
		Region:    s.opts.Region,
	})
	if err != nil {
		return fmt.Errorf("pinecone: failed to create index %q: %w", name, err)
	}

	return nil
}

// DeleteIndex removes an index and all its data.
func (s *Store) DeleteIndex(ctx context.Context, name string) error {
	err := s.client.DeleteIndex(ctx, name)
	if err != nil {
		return fmt.Errorf("pinecone: failed to delete index %q: %w", name, err)
	}

	return nil
}

// ListIndexes returns all available indexes.
func (s *Store) ListIndexes(ctx context.Context) ([]string, error) {
	indexes, err := s.client.ListIndexes(ctx)
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to list indexes: %w", err)
	}

	names := make([]string, len(indexes))
	for i, idx := range indexes {
		names[i] = idx.Name
	}

	return names, nil
}

// Close releases resources.
func (s *Store) Close() error {
	if s.idx != nil {
		return s.idx.Close()
	}
	return nil
}

// buildFilter creates a Pinecone metadata filter from vectorstore filter options.
func buildFilter(filter vectorstore.Filter) (*pinecone.MetadataFilter, error) {
	if len(filter) == 0 {
		return nil, nil
	}

	filterMap := make(map[string]any)
	for k, v := range filter {
		filterMap[k] = map[string]any{"$eq": v}
	}
	filterStruct, err := structpb.NewStruct(filterMap)
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to create filter: %w", err)
	}
	return filterStruct, nil
}

// convertMatches converts Pinecone scored vectors to vectorstore documents.
func convertMatches(matches []*pinecone.ScoredVector, minScore float64, includeEmbeddings bool) []vectorstore.Document {
	results := make([]vectorstore.Document, 0, len(matches))
	for _, match := range matches {
		score := float64(match.Score)
		if score < minScore {
			continue
		}

		doc := vectorstore.Document{
			ID:    match.Vector.Id,
			Score: score,
		}

		// Extract metadata
		if match.Vector.Metadata != nil {
			metadata := match.Vector.Metadata.AsMap()
			if content, ok := metadata["content"].(string); ok {
				doc.Content = content
				delete(metadata, "content")
			}
			if ts, ok := metadata["timestamp"].(float64); ok {
				doc.Timestamp = time.Unix(0, int64(ts))
				delete(metadata, "timestamp")
			}
			doc.Metadata = metadata
		}

		// Include embeddings if requested
		if includeEmbeddings && match.Vector.Values != nil {
			doc.Embedding = floatconv.ToFloat64(match.Vector.Values)
		}

		results = append(results, doc)
	}
	return results
}

// toPineconeMetric converts embedding metric to Pinecone metric.
func toPineconeMetric(m embedding.Metric) pinecone.IndexMetric {
	switch m {
	case embedding.Cosine:
		return pinecone.Cosine
	case embedding.Euclidean:
		return pinecone.Euclidean
	case embedding.DotProduct:
		return pinecone.Dotproduct
	default:
		return pinecone.Cosine
	}
}
