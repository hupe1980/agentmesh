package langchaingo

import (
	"context"
	"fmt"
	"strings"

	"github.com/hupe1980/agentmesh/internal/validate"
	"github.com/hupe1980/agentmesh/pkg/retrieval"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

// Options customises how the LangChain Go retriever fetches documents.
type Options struct {
	NumDocuments       int
	VectorStoreOptions []vectorstores.Option
}

// Retriever adapts LangChain Go retrievers to the AgentMesh retrieval.Document API.
type Retriever struct {
	retriever schema.Retriever
}

// NewRetrieverFromVectorStore builds a Retriever from a LangChain vector store.
// Returns an error if the vector store is nil or NewRetriever fails.
func NewRetrieverFromVectorStore(vectorStore vectorstores.VectorStore, optFns ...func(o *Options)) (*Retriever, error) {
	opts := Options{
		NumDocuments:       3,
		VectorStoreOptions: []vectorstores.Option{},
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return NewRetriever(vectorstores.ToRetriever(vectorStore, opts.NumDocuments, opts.VectorStoreOptions...))
}

// NewRetriever wraps an existing LangChain retriever.
// Returns an error if the retriever parameter is nil.
func NewRetriever(retriever schema.Retriever) (*Retriever, error) {
	if err := validate.NotNil(retriever, "retriever"); err != nil {
		return nil, fmt.Errorf("langchaingo: %w", err)
	}

	return &Retriever{
		retriever: retriever,
	}, nil
}

// Retrieve fetches relevant documents from the underlying LangChain retriever.
func (r *Retriever) Retrieve(ctx context.Context, query string) ([]retrieval.Document, error) {
	query = strings.TrimSpace(query)
	if err := validate.NotEmpty(query, "query"); err != nil {
		return nil, fmt.Errorf("langchaingo retriever: %w", err)
	}

	lcDocs, err := r.retriever.GetRelevantDocuments(ctx, query)
	if err != nil {
		return nil, err
	}

	docs := make([]retrieval.Document, 0, len(lcDocs))
	for _, doc := range lcDocs {
		docs = append(docs, retrieval.Document{
			PageContent: doc.PageContent,
			Score:       float64(doc.Score),
			Metadata:    doc.Metadata,
		})
	}

	return docs, nil
}
