package vectorstore

import (
	"context"
	"time"

	"github.com/hupe1980/agentmesh/pkg/embedding"
)

// Document represents a storable/retrievable item with vector and metadata.
type Document struct {
	// ID is the unique identifier. If empty on Add, one will be generated.
	ID string

	// Content is the original text content.
	Content string

	// Embedding is the vector representation.
	Embedding embedding.Vector

	// Metadata contains arbitrary key-value pairs for filtering.
	Metadata Metadata

	// Score is the similarity score (populated on search results).
	Score float64

	// Timestamp is when this document was added/updated.
	Timestamp time.Time
}

// Metadata is a map of string keys to any values.
type Metadata = map[string]any

// SearchOptions configures similarity search behavior.
type SearchOptions struct {
	// K is the maximum number of results to return. Default: 10
	K int

	// MinScore filters results below this similarity threshold (0.0-1.0).
	MinScore float64

	// Filter applies metadata-based filtering.
	Filter Filter

	// IncludeEmbeddings controls whether embeddings are returned in results.
	IncludeEmbeddings bool

	// Namespace partitions the store (for multi-tenant scenarios).
	Namespace string
}

// DefaultK is the default number of results if K is not specified.
const DefaultK = 10

// Normalize ensures options have sensible defaults.
func (o *SearchOptions) Normalize() {
	if o.K <= 0 {
		o.K = DefaultK
	}
	if o.MinScore < 0 {
		o.MinScore = 0
	}
	if o.MinScore > 1 {
		o.MinScore = 1
	}
}

// Filter represents a metadata filter for search queries.
// Simple map-based approach - backends translate as needed.
type Filter map[string]any

// Eq creates an equality filter for a field.
func Eq(field string, value any) Filter { return Filter{field: value} }

// In creates a filter matching any of the provided values.
func In(field string, values ...any) Filter { return Filter{field: values} }

// And combines multiple filters (merged map).
func And(filters ...Filter) Filter {
	result := make(Filter)
	for _, f := range filters {
		for k, v := range f {
			result[k] = v
		}
	}
	return result
}

// AddOptions configures document addition behavior.
type AddOptions struct {
	// Namespace partitions the store.
	Namespace string

	// Upsert controls whether to update existing documents. Default: true
	Upsert bool
}

// VectorStore defines the contract for vector storage backends.
type VectorStore interface {
	// Add inserts or updates documents in the store.
	Add(ctx context.Context, docs []Document, opts ...func(*AddOptions)) error

	// Search finds documents similar to the query embedding.
	Search(ctx context.Context, embedding embedding.Vector, opts SearchOptions) ([]Document, error)

	// Delete removes documents by ID.
	Delete(ctx context.Context, ids []string, namespace string) error

	// Close releases resources.
	Close() error
}

// TextSearcher extends VectorStore with hybrid (keyword + vector) search.
type TextSearcher interface {
	VectorStore

	// SearchHybrid combines keyword and vector search.
	// The alpha parameter in HybridSearchOptions controls the balance:
	// 0.0 = pure keyword search, 1.0 = pure vector search.
	SearchHybrid(ctx context.Context, query string, embedding embedding.Vector, opts HybridSearchOptions) ([]Document, error)
}

// HybridSearchOptions configures hybrid search behavior.
type HybridSearchOptions struct {
	SearchOptions

	// Alpha controls the balance between keyword and vector search.
	// 0.0 = pure keyword (BM25/sparse), 1.0 = pure vector (dense).
	// Default: 0.5 (equal weighting)
	Alpha float64

	// FusionAlgorithm specifies how to combine results.
	// Default: RRF (Reciprocal Rank Fusion)
	FusionAlgorithm FusionAlgorithm
}

// FusionAlgorithm specifies how to combine keyword and vector search results.
type FusionAlgorithm string

const (
	// FusionRRF uses Reciprocal Rank Fusion to combine results.
	// Score = sum(1 / (k + rank)) for each result list.
	FusionRRF FusionAlgorithm = "rrf"

	// FusionRelativeScore normalizes and combines scores from both searches.
	FusionRelativeScore FusionAlgorithm = "relativeScore"
)

// Normalize ensures hybrid options have sensible defaults.
func (o *HybridSearchOptions) Normalize() {
	o.SearchOptions.Normalize()
	if o.Alpha < 0 {
		o.Alpha = 0
	}
	if o.Alpha > 1 {
		o.Alpha = 1
	}
	if o.FusionAlgorithm == "" {
		o.FusionAlgorithm = FusionRRF
	}
}

// Indexer extends VectorStore with index lifecycle management.
type Indexer interface {
	VectorStore

	// CreateIndex creates a new index/collection.
	CreateIndex(ctx context.Context, name string, dims int, metric embedding.Metric) error

	// DeleteIndex removes an index and all its data.
	DeleteIndex(ctx context.Context, name string) error

	// ListIndexes returns all available indexes.
	ListIndexes(ctx context.Context) ([]string, error)
}

// Stats provides storage statistics.
type Stats struct {
	DocumentCount int64
	IndexSize     int64 // bytes
	Dimensions    int
}

// StatsProvider optionally exposes storage statistics.
type StatsProvider interface {
	Stats(ctx context.Context, namespace string) (Stats, error)
}
