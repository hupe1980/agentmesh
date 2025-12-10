// Package memory provides an in-memory VectorStore implementation.
//
// This implementation is suitable for testing, development, and small datasets.
// For production use with large datasets, consider external backends like Pinecone or Qdrant.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/vectorstore"
)

// Options configures the in-memory store.
type Options struct {
	// Metric specifies the similarity metric. Default: Cosine
	Metric embedding.Metric
}

// Store is an in-memory VectorStore implementation.
type Store struct {
	docs   map[string]map[string]vectorstore.Document // namespace -> id -> document
	metric embedding.Metric
	mu     sync.RWMutex
}

// New creates a new in-memory vector store.
func New(optFns ...func(*Options)) *Store {
	opts := Options{
		Metric: embedding.Cosine,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	return &Store{
		docs:   make(map[string]map[string]vectorstore.Document),
		metric: opts.Metric,
	}
}

// Add inserts or updates documents in the store.
func (s *Store) Add(ctx context.Context, docs []vectorstore.Document, optFns ...func(*vectorstore.AddOptions)) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	opts := vectorstore.AddOptions{
		Upsert: true,
	}
	for _, fn := range optFns {
		fn(&opts)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ns := opts.Namespace
	if s.docs[ns] == nil {
		s.docs[ns] = make(map[string]vectorstore.Document)
	}

	now := time.Now()
	for i, doc := range docs {
		// Generate ID if not provided
		if doc.ID == "" {
			doc.ID = uuid.New().String()
		}

		// Set timestamp if not provided
		if doc.Timestamp.IsZero() {
			doc.Timestamp = now.Add(time.Duration(i) * time.Nanosecond)
		}

		// Check for existing document
		if !opts.Upsert {
			if _, exists := s.docs[ns][doc.ID]; exists {
				continue // Skip existing documents when upsert is disabled
			}
		}

		s.docs[ns][doc.ID] = doc
	}

	return nil
}

// Search finds documents similar to the query embedding.
func (s *Store) Search(ctx context.Context, queryEmbedding embedding.Vector, opts vectorstore.SearchOptions) ([]vectorstore.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	opts.Normalize()

	s.mu.RLock()
	nsDocs := s.docs[opts.Namespace]
	s.mu.RUnlock()

	if len(nsDocs) == 0 {
		return nil, nil
	}

	// Calculate similarity scores
	type scored struct {
		doc   vectorstore.Document
		score float64
	}

	var candidates []scored
	for _, doc := range nsDocs {
		// Apply metadata filter
		if opts.Filter != nil && !matchesFilter(doc.Metadata, opts.Filter) {
			continue
		}

		score := float64(embedding.Similarity(queryEmbedding, doc.Embedding, s.metric))
		if score >= opts.MinScore {
			candidates = append(candidates, scored{doc: doc, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Limit to K results
	if len(candidates) > opts.K {
		candidates = candidates[:opts.K]
	}

	// Convert to result slice
	results := make([]vectorstore.Document, len(candidates))
	for i, c := range candidates {
		results[i] = c.doc
		results[i].Score = c.score

		// Optionally strip embeddings
		if !opts.IncludeEmbeddings {
			results[i].Embedding = nil
		}
	}

	return results, nil
}

// Delete removes documents by ID.
func (s *Store) Delete(ctx context.Context, ids []string, namespace string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nsDocs := s.docs[namespace]
	if nsDocs == nil {
		return nil
	}

	for _, id := range ids {
		delete(nsDocs, id)
	}

	return nil
}

// Close releases resources. For in-memory store, this is a no-op.
func (s *Store) Close() error {
	return nil
}

// Stats returns storage statistics.
func (s *Store) Stats(ctx context.Context, namespace string) (vectorstore.Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nsDocs := s.docs[namespace]
	if len(nsDocs) == 0 {
		return vectorstore.Stats{}, nil
	}

	var dims int
	for _, doc := range nsDocs {
		if len(doc.Embedding) > 0 {
			dims = len(doc.Embedding)
			break
		}
	}

	return vectorstore.Stats{
		DocumentCount: int64(len(nsDocs)),
		Dimensions:    dims,
	}, nil
}

// Clear removes all documents from the store.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = make(map[string]map[string]vectorstore.Document)
}

// matchesFilter checks if document metadata matches the filter criteria.
func matchesFilter(metadata vectorstore.Metadata, filter vectorstore.Filter) bool {
	if metadata == nil && len(filter) > 0 {
		return false
	}

	for key, filterValue := range filter {
		metaValue, exists := metadata[key]
		if !exists {
			return false
		}

		// Handle slice values (IN filter)
		if values, ok := filterValue.([]any); ok {
			found := false
			for _, v := range values {
				if metaValue == v {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		} else if metaValue != filterValue {
			return false
		}
	}

	return true
}
