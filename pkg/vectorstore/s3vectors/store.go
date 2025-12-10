package s3vectors

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/document"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/types"
	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/internal/safeconv"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
)

// Ensure Store implements the interfaces.
var (
	_ vectorstore.VectorStore = (*Store)(nil)
	_ vectorstore.Indexer     = (*Store)(nil)
)

// Client defines the interface for S3 Vectors operations.
// This interface allows for mocking in tests.
type Client interface {
	PutVectors(ctx context.Context, params *s3vectors.PutVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.PutVectorsOutput, error)
	QueryVectors(ctx context.Context, params *s3vectors.QueryVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.QueryVectorsOutput, error)
	DeleteVectors(ctx context.Context, params *s3vectors.DeleteVectorsInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteVectorsOutput, error)
	CreateIndex(ctx context.Context, params *s3vectors.CreateIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.CreateIndexOutput, error)
	DeleteIndex(ctx context.Context, params *s3vectors.DeleteIndexInput, optFns ...func(*s3vectors.Options)) (*s3vectors.DeleteIndexOutput, error)
	ListIndexes(ctx context.Context, params *s3vectors.ListIndexesInput, optFns ...func(*s3vectors.Options)) (*s3vectors.ListIndexesOutput, error)
}

// Ensure the AWS SDK client implements our interface.
var _ Client = (*s3vectors.Client)(nil)

// Options configures the S3 Vectors store.
type Options struct {
	// Dimensions is the vector dimensionality. Required for index creation.
	Dimensions int

	// Metric specifies the distance metric. Default: Cosine
	Metric embedding.Metric
}

// Option configures a Store.
type Option func(*Options)

// WithDimensions sets the vector dimensions.
func WithDimensions(dims int) Option {
	return func(o *Options) {
		o.Dimensions = dims
	}
}

// WithMetric sets the distance metric.
func WithMetric(metric embedding.Metric) Option {
	return func(o *Options) {
		o.Metric = metric
	}
}

// Store is an S3 Vectors-backed VectorStore implementation.
type Store struct {
	client           Client
	vectorBucketName string
	indexName        string
	opts             Options
}

// New creates a new S3 Vectors store.
// client is the S3 Vectors client (use s3vectors.NewFromConfig to create one).
// vectorBucketName is the name of the vector bucket.
// indexName is the name of the vector index.
func New(client Client, vectorBucketName, indexName string, optFns ...Option) *Store {
	opts := Options{
		Metric: embedding.Cosine,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	return &Store{
		client:           client,
		vectorBucketName: vectorBucketName,
		indexName:        indexName,
		opts:             opts,
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

	// S3 Vectors supports batch operations
	vectors := make([]types.PutInputVector, len(docs))
	now := time.Now()

	for i, doc := range docs {
		docID := doc.ID
		if docID == "" {
			docID = uuid.New().String()
		}

		ts := doc.Timestamp
		if ts.IsZero() {
			ts = now.Add(time.Duration(i) * time.Nanosecond)
		}

		// Build metadata
		metadata := make(map[string]any)
		metadata["content"] = doc.Content
		metadata["timestamp"] = ts.UnixNano()
		if opts.Namespace != "" {
			metadata["namespace"] = opts.Namespace
		}
		for k, v := range doc.Metadata {
			metadata[k] = v
		}

		vectors[i] = types.PutInputVector{
			Key:      aws.String(docID),
			Data:     &types.VectorDataMemberFloat32{Value: doc.Embedding},
			Metadata: toDocumentInterface(metadata),
		}
	}

	_, err := s.client.PutVectors(ctx, &s3vectors.PutVectorsInput{
		VectorBucketName: aws.String(s.vectorBucketName),
		IndexName:        aws.String(s.indexName),
		Vectors:          vectors,
	})
	if err != nil {
		return fmt.Errorf("s3vectors: failed to put vectors: %w", err)
	}

	return nil
}

// Search finds documents similar to the query embedding.
func (s *Store) Search(ctx context.Context, queryEmbedding embedding.Vector, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	opts.Normalize()

	input := &s3vectors.QueryVectorsInput{
		VectorBucketName: aws.String(s.vectorBucketName),
		IndexName:        aws.String(s.indexName),
		QueryVector:      &types.VectorDataMemberFloat32{Value: queryEmbedding},
		TopK:             aws.Int32(safeconv.IntToInt32(opts.K)),
		ReturnMetadata:   true,
		ReturnDistance:   true,
	}

	// Add filter if specified
	if len(opts.Filter) > 0 || opts.Namespace != "" {
		filterMap := make(map[string]any)
		for k, v := range opts.Filter {
			filterMap[k] = v
		}
		if opts.Namespace != "" {
			filterMap["namespace"] = opts.Namespace
		}
		input.Filter = toDocumentInterface(filterMap)
	}

	resp, err := s.client.QueryVectors(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("s3vectors: query failed: %w", err)
	}

	// Convert results
	results := make([]vectorstore.Document, 0, len(resp.Vectors))
	for _, v := range resp.Vectors {
		// Convert distance to similarity score
		score := 1.0
		if v.Distance != nil {
			score = 1.0 - float64(*v.Distance)
		}

		if score < opts.MinScore {
			continue
		}

		doc := vectorstore.Document{
			ID:    aws.ToString(v.Key),
			Score: score,
		}

		// Parse metadata
		if v.Metadata != nil {
			metadata := fromDocumentInterface(v.Metadata)
			if content, ok := metadata["content"].(string); ok {
				doc.Content = content
				delete(metadata, "content")
			}
			if ts, ok := metadata["timestamp"].(float64); ok {
				doc.Timestamp = time.Unix(0, int64(ts))
				delete(metadata, "timestamp")
			}
			delete(metadata, "namespace")
			if len(metadata) > 0 {
				doc.Metadata = metadata
			}
		}

		// S3 Vectors doesn't return vectors in query response by default
		// Would need GetVectors for embeddings

		results = append(results, doc)
	}

	return results, nil
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string, namespace string) error {
	if len(ids) == 0 {
		return nil
	}

	// Convert to aws.String pointers
	keys := make([]string, len(ids))
	copy(keys, ids)

	_, err := s.client.DeleteVectors(ctx, &s3vectors.DeleteVectorsInput{
		VectorBucketName: aws.String(s.vectorBucketName),
		IndexName:        aws.String(s.indexName),
		Keys:             keys,
	})
	if err != nil {
		return fmt.Errorf("s3vectors: failed to delete vectors: %w", err)
	}

	return nil
}

// CreateIndex creates a new vector index.
func (s *Store) CreateIndex(ctx context.Context, name string, dims int, metric embedding.Metric) error {
	_, err := s.client.CreateIndex(ctx, &s3vectors.CreateIndexInput{
		VectorBucketName: aws.String(s.vectorBucketName),
		IndexName:        aws.String(name),
		Dimension:        aws.Int32(safeconv.IntToInt32(dims)),
		DistanceMetric:   toS3VectorsMetric(metric),
	})
	if err != nil {
		return fmt.Errorf("s3vectors: failed to create index %q: %w", name, err)
	}

	return nil
}

// DeleteIndex removes an index and all its data.
func (s *Store) DeleteIndex(ctx context.Context, name string) error {
	_, err := s.client.DeleteIndex(ctx, &s3vectors.DeleteIndexInput{
		VectorBucketName: aws.String(s.vectorBucketName),
		IndexName:        aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("s3vectors: failed to delete index %q: %w", name, err)
	}

	return nil
}

// ListIndexes returns all available indexes in the vector bucket.
func (s *Store) ListIndexes(ctx context.Context) ([]string, error) {
	resp, err := s.client.ListIndexes(ctx, &s3vectors.ListIndexesInput{
		VectorBucketName: aws.String(s.vectorBucketName),
	})
	if err != nil {
		return nil, fmt.Errorf("s3vectors: failed to list indexes: %w", err)
	}

	names := make([]string, len(resp.Indexes))
	for i, idx := range resp.Indexes {
		names[i] = aws.ToString(idx.IndexName)
	}

	return names, nil
}

// Close releases resources.
func (s *Store) Close() error {
	// AWS SDK client doesn't require explicit closing
	return nil
}

// toS3VectorsMetric converts embedding metric to S3 Vectors distance metric.
func toS3VectorsMetric(m embedding.Metric) types.DistanceMetric {
	switch m {
	case embedding.Cosine:
		return types.DistanceMetricCosine
	case embedding.Euclidean:
		return types.DistanceMetricEuclidean
	default:
		return types.DistanceMetricCosine
	}
}

// toDocumentInterface converts a map to document.Interface for AWS SDK.
func toDocumentInterface(m map[string]any) document.Interface {
	return document.NewLazyDocument(m)
}

// fromDocumentInterface converts document.Interface to a map.
func fromDocumentInterface(d document.Interface) map[string]any {
	if d == nil {
		return nil
	}
	var result map[string]any
	if err := d.UnmarshalSmithyDocument(&result); err != nil {
		return nil
	}
	return result
}
