package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/embedding"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// Memory is an in-memory semantic cache with LRU eviction.
// It uses embeddings to find similar prompts and caches responses.
// Thread-safe for concurrent access.
type Memory struct {
	embedder embedding.Embedder
	options  Options

	mu      sync.RWMutex
	entries map[string]*list.Element // key -> LRU element
	lru     *list.List               // LRU list of entries

	// Statistics
	hits      int64
	misses    int64
	evictions int64
}

// lruEntry wraps a cache entry with its key for LRU tracking.
type lruEntry struct {
	key   string
	entry *Entry
}

// NewMemory creates an in-memory semantic cache.
// The embedder is used to convert prompts into vectors for similarity matching.
func NewMemory(embedder embedding.Embedder, opts ...Option) *Memory {
	return &Memory{
		embedder: embedder,
		options:  ApplyOptions(opts...),
		entries:  make(map[string]*list.Element),
		lru:      list.New(),
	}
}

// Get retrieves a cached response for a similar request.
func (m *Memory) Get(ctx context.Context, req *model.Request) (*model.Response, error) {
	// Generate cache key
	key := m.options.KeyFunc(req)

	// Compute embedding for the request
	embedding, err := m.embedder.Embed(ctx, key)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Build list of entries to check
	candidates := make([]*Entry, 0, len(m.entries))
	now := time.Now()

	for _, elem := range m.entries {
		lruEntry := elem.Value.(*lruEntry)

		// Skip expired entries
		if m.options.TTL > 0 && now.Sub(lruEntry.entry.Timestamp) > m.options.TTL {
			continue
		}

		candidates = append(candidates, lruEntry.entry)
	}

	// Find most similar entry above threshold
	bestEntry, score := FindMostSimilar(embedding, candidates, m.options.SimilarityThreshold)

	if bestEntry != nil {
		m.mu.RUnlock()
		m.mu.Lock()
		m.hits++
		// Move to front (most recently used)
		if elem, ok := m.entries[m.options.KeyFunc(bestEntry.Request)]; ok {
			m.lru.MoveToFront(elem)
		}
		m.mu.Unlock()
		m.mu.RLock()

		// Add similarity score to metadata
		if bestEntry.Response.Metadata == nil {
			bestEntry.Response.Metadata = make(map[string]any)
		}
		bestEntry.Response.Metadata["cache_hit"] = true
		bestEntry.Response.Metadata["cache_similarity"] = score

		return bestEntry.Response, nil
	}

	m.mu.RUnlock()
	m.mu.Lock()
	m.misses++
	m.mu.Unlock()
	m.mu.RLock()

	return nil, nil // Cache miss
}

// Set stores a response in the cache.
func (m *Memory) Set(ctx context.Context, req *model.Request, resp *model.Response) error {
	// Generate cache key
	key := m.options.KeyFunc(req)

	// Compute embedding for the request
	embedding, err := m.embedder.Embed(ctx, key)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we need to evict (LRU)
	if m.options.MaxSize > 0 && m.lru.Len() >= m.options.MaxSize {
		// Remove least recently used
		oldest := m.lru.Back()
		if oldest != nil {
			oldEntry := oldest.Value.(*lruEntry)
			delete(m.entries, oldEntry.key)
			m.lru.Remove(oldest)
			m.evictions++
		}
	}

	// Create entry
	entry := &Entry{
		Request:   req,
		Response:  resp,
		Embedding: embedding,
		Timestamp: time.Now(),
		Metadata:  make(map[string]any),
	}

	// Add to LRU (front = most recently used)
	elem := m.lru.PushFront(&lruEntry{
		key:   key,
		entry: entry,
	})

	// Add to map
	m.entries[key] = elem

	return nil
}

// Clear removes all entries from the cache.
func (m *Memory) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]*list.Element)
	m.lru = list.New()

	return nil
}

// Close releases resources (no-op for memory cache).
func (m *Memory) Close() error {
	return nil
}

// Stats returns cache statistics.
func (m *Memory) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := m.hits + m.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(m.hits) / float64(total)
	}

	return Stats{
		Size:      m.lru.Len(),
		Hits:      m.hits,
		Misses:    m.misses,
		HitRate:   hitRate,
		Evictions: m.evictions,
	}
}

// Stats contains cache performance metrics.
type Stats struct {
	Size      int
	Hits      int64
	Misses    int64
	HitRate   float64
	Evictions int64
}
