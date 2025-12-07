package vectorstore

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/embedding"
)

// EmbeddingStore wraps a VectorStore with automatic embedding generation.
// It simplifies common workflows by handling embedding creation internally.
type EmbeddingStore struct {
	store    VectorStore
	embedder embedding.Embedder
}

// NewEmbeddingStore creates a store that auto-embeds content.
func NewEmbeddingStore(store VectorStore, embedder embedding.Embedder) *EmbeddingStore {
	return &EmbeddingStore{store: store, embedder: embedder}
}

// AddTexts embeds and stores text documents.
// If metadata slice is shorter than texts, remaining documents get nil metadata.
func (es *EmbeddingStore) AddTexts(ctx context.Context, texts []string, metadata []Metadata, opts ...func(*AddOptions)) error {
	if len(texts) == 0 {
		return nil
	}

	embeddings, err := es.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return err
	}

	docs := make([]Document, len(texts))
	now := time.Now()
	for i, text := range texts {
		var meta Metadata
		if i < len(metadata) && metadata[i] != nil {
			meta = metadata[i]
		}

		docs[i] = Document{
			ID:        uuid.New().String(),
			Content:   text,
			Embedding: embeddings[i],
			Metadata:  meta,
			Timestamp: now.Add(time.Duration(i) * time.Nanosecond), // Preserve order
		}
	}

	return es.store.Add(ctx, docs, opts...)
}

// AddDocuments adds pre-structured documents, generating embeddings from their content.
// Documents with existing embeddings are added as-is.
func (es *EmbeddingStore) AddDocuments(ctx context.Context, docs []Document, opts ...func(*AddOptions)) error {
	if len(docs) == 0 {
		return nil
	}

	// Collect documents that need embeddings
	var needEmbedding []int
	var texts []string
	for i, doc := range docs {
		if len(doc.Embedding) == 0 && doc.Content != "" {
			needEmbedding = append(needEmbedding, i)
			texts = append(texts, doc.Content)
		}
	}

	// Generate embeddings for documents that need them
	if len(texts) > 0 {
		embeddings, err := es.embedder.EmbedBatch(ctx, texts)
		if err != nil {
			return err
		}

		for j, idx := range needEmbedding {
			docs[idx].Embedding = embeddings[j]
		}
	}

	// Generate IDs for documents without them
	now := time.Now()
	for i := range docs {
		if docs[i].ID == "" {
			docs[i].ID = uuid.New().String()
		}
		if docs[i].Timestamp.IsZero() {
			docs[i].Timestamp = now.Add(time.Duration(i) * time.Nanosecond)
		}
	}

	return es.store.Add(ctx, docs, opts...)
}

// SearchText embeds query and performs similarity search.
func (es *EmbeddingStore) SearchText(ctx context.Context, query string, opts SearchOptions) ([]Document, error) {
	queryEmbedding, err := es.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	return es.store.Search(ctx, queryEmbedding, opts)
}

// Store returns the underlying VectorStore.
func (es *EmbeddingStore) Store() VectorStore {
	return es.store
}

// Embedder returns the underlying Embedder.
func (es *EmbeddingStore) Embedder() embedding.Embedder {
	return es.embedder
}

// Delete removes documents by ID (delegates to underlying store).
func (es *EmbeddingStore) Delete(ctx context.Context, ids []string, namespace string) error {
	return es.store.Delete(ctx, ids, namespace)
}

// Close releases resources (delegates to underlying store).
func (es *EmbeddingStore) Close() error {
	return es.store.Close()
}
