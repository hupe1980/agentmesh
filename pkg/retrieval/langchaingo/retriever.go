package langchaingo

import (
	"context"
	"errors"
	"strings"

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
func NewRetrieverFromVectorStore(vectorStore vectorstores.VectorStore, optFns ...func(o *Options)) *Retriever {
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
func NewRetriever(retriever schema.Retriever) *Retriever {
	return &Retriever{
		retriever: retriever,
	}
}

// Retrieve fetches relevant documents from the underlying LangChain retriever.
func (r *Retriever) Retrieve(ctx context.Context, query string) ([]retrieval.Document, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("empty langchaingo query string")
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
