package retrieval

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
)

// HybridVectorStoreRetriever adapts a TextSearcher to the Retriever interface
// using hybrid search that combines keyword and vector similarity.
type HybridVectorStoreRetriever struct {
	store      vectorstore.TextSearcher
	embedder   embedding.Embedder
	searchOpts vectorstore.HybridSearchOptions
}

// HybridVectorStoreRetrieverOptions configures the hybrid retriever.
type HybridVectorStoreRetrieverOptions struct {
	// K is the maximum number of documents to retrieve. Default: 10
	K int

	// MinScore filters results below this similarity threshold (0.0-1.0).
	MinScore float64

	// Filter applies metadata-based filtering.
	Filter vectorstore.Filter

	// Namespace partitions the store (for multi-tenant scenarios).
	Namespace string

	// Alpha controls the balance between keyword and vector search.
	// 0.0 = pure keyword (BM25/sparse), 1.0 = pure vector (dense).
	// Default: 0.5 (equal weighting)
	Alpha float64

	// FusionAlgorithm specifies how to combine results.
	// Default: RRF (Reciprocal Rank Fusion)
	FusionAlgorithm vectorstore.FusionAlgorithm
}

// NewHybridVectorStoreRetriever creates a Retriever backed by a TextSearcher
// that uses hybrid search combining keyword and vector similarity.
func NewHybridVectorStoreRetriever(
	store vectorstore.TextSearcher,
	embedder embedding.Embedder,
	optFns ...func(*HybridVectorStoreRetrieverOptions),
) Retriever {
	opts := HybridVectorStoreRetrieverOptions{
		K:               10,
		MinScore:        0,
		Alpha:           0.5,
		FusionAlgorithm: vectorstore.FusionRRF,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &HybridVectorStoreRetriever{
		store:    store,
		embedder: embedder,
		searchOpts: vectorstore.HybridSearchOptions{
			SearchOptions: vectorstore.SearchOptions{
				K:         opts.K,
				MinScore:  opts.MinScore,
				Filter:    opts.Filter,
				Namespace: opts.Namespace,
			},
			Alpha:           opts.Alpha,
			FusionAlgorithm: opts.FusionAlgorithm,
		},
	}
}

// Retrieve implements Retriever by embedding the query and performing hybrid search.
func (r *HybridVectorStoreRetriever) Retrieve(ctx context.Context, query string) ([]Document, error) {
	queryEmbedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	docs, err := r.store.SearchHybrid(ctx, query, queryEmbedding, r.searchOpts)
	if err != nil {
		return nil, err
	}

	results := make([]Document, len(docs))
	for i, doc := range docs {
		results[i] = Document{
			PageContent: doc.Content,
			Score:       doc.Score,
			Metadata:    doc.Metadata,
		}
	}

	return results, nil
}

// WithHybridK sets the maximum number of documents to retrieve.
func WithHybridK(k int) func(*HybridVectorStoreRetrieverOptions) {
	return func(o *HybridVectorStoreRetrieverOptions) {
		o.K = k
	}
}

// WithHybridMinScore sets the minimum similarity threshold.
func WithHybridMinScore(score float64) func(*HybridVectorStoreRetrieverOptions) {
	return func(o *HybridVectorStoreRetrieverOptions) {
		o.MinScore = score
	}
}

// WithHybridFilter sets the metadata filter.
func WithHybridFilter(filter vectorstore.Filter) func(*HybridVectorStoreRetrieverOptions) {
	return func(o *HybridVectorStoreRetrieverOptions) {
		o.Filter = filter
	}
}

// WithHybridNamespace sets the namespace for multi-tenant scenarios.
func WithHybridNamespace(namespace string) func(*HybridVectorStoreRetrieverOptions) {
	return func(o *HybridVectorStoreRetrieverOptions) {
		o.Namespace = namespace
	}
}

// WithAlpha sets the balance between keyword and vector search.
// 0.0 = pure keyword, 1.0 = pure vector, 0.5 = equal weighting (default).
func WithAlpha(alpha float64) func(*HybridVectorStoreRetrieverOptions) {
	return func(o *HybridVectorStoreRetrieverOptions) {
		o.Alpha = alpha
	}
}

// WithFusionAlgorithm sets the algorithm for combining search results.
func WithFusionAlgorithm(algo vectorstore.FusionAlgorithm) func(*HybridVectorStoreRetrieverOptions) {
	return func(o *HybridVectorStoreRetrieverOptions) {
		o.FusionAlgorithm = algo
	}
}
