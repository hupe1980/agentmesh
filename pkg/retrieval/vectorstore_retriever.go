package retrieval

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
)

// VectorStoreRetriever adapts a VectorStore to the Retriever interface.
type VectorStoreRetriever struct {
	store      vectorstore.VectorStore
	embedder   embedding.Embedder
	searchOpts vectorstore.SearchOptions
}

// VectorStoreRetrieverOptions configures the retriever.
type VectorStoreRetrieverOptions struct {
	// K is the maximum number of documents to retrieve. Default: 10
	K int

	// MinScore filters results below this similarity threshold (0.0-1.0).
	MinScore float64

	// Filter applies metadata-based filtering.
	Filter vectorstore.Filter

	// Namespace partitions the store (for multi-tenant scenarios).
	Namespace string
}

// NewVectorStoreRetriever creates a Retriever backed by a VectorStore.
func NewVectorStoreRetriever(
	store vectorstore.VectorStore,
	embedder embedding.Embedder,
	optFns ...func(*VectorStoreRetrieverOptions),
) Retriever {
	opts := VectorStoreRetrieverOptions{
		K:        10,
		MinScore: 0,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &VectorStoreRetriever{
		store:    store,
		embedder: embedder,
		searchOpts: vectorstore.SearchOptions{
			K:         opts.K,
			MinScore:  opts.MinScore,
			Filter:    opts.Filter,
			Namespace: opts.Namespace,
		},
	}
}

// Retrieve implements Retriever by embedding the query and searching the VectorStore.
func (r *VectorStoreRetriever) Retrieve(ctx context.Context, query string) ([]Document, error) {
	queryEmbedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	docs, err := r.store.Search(ctx, queryEmbedding, r.searchOpts)
	if err != nil {
		return nil, err
	}

	results := make([]Document, len(docs))
	for i, doc := range docs {
		results[i] = Document{
			PageContent: doc.Content, // VectorStore uses Content, Retriever uses PageContent
			Score:       doc.Score,
			Metadata:    doc.Metadata,
		}
	}

	return results, nil
}

// WithK sets the maximum number of documents to retrieve.
func WithK(k int) func(*VectorStoreRetrieverOptions) {
	return func(o *VectorStoreRetrieverOptions) {
		o.K = k
	}
}

// WithMinScore sets the minimum similarity threshold.
func WithMinScore(score float64) func(*VectorStoreRetrieverOptions) {
	return func(o *VectorStoreRetrieverOptions) {
		o.MinScore = score
	}
}

// WithFilter sets the metadata filter.
func WithFilter(filter vectorstore.Filter) func(*VectorStoreRetrieverOptions) {
	return func(o *VectorStoreRetrieverOptions) {
		o.Filter = filter
	}
}

// WithNamespace sets the namespace for multi-tenant scenarios.
func WithNamespace(namespace string) func(*VectorStoreRetrieverOptions) {
	return func(o *VectorStoreRetrieverOptions) {
		o.Namespace = namespace
	}
}
