package graph

import (
	"context"
	"sync"
	"time"
)

// InMemoryCacheStore is a simple in-memory cache implementation.
// Thread-safe for concurrent access.
type InMemoryCacheStore struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	value     CacheEntry
	expiresAt time.Time
}

// NewInMemoryCacheStore creates a new in-memory cache store.
func NewInMemoryCacheStore() *InMemoryCacheStore {
	store := &InMemoryCacheStore{
		entries: make(map[string]*cacheEntry),
	}

	// Start background cleanup goroutine
	go store.cleanup()

	return store
}

// Get retrieves a cached value by key.
func (s *InMemoryCacheStore) Get(ctx context.Context, key string) (CacheEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.entries[key]
	if !ok {
		return CacheEntry{}, nil
	}

	// Check expiration
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return CacheEntry{}, nil
	}

	return entry.value, nil
}

// Set stores a value in the cache.
func (s *InMemoryCacheStore) Set(ctx context.Context, key string, value CacheEntry, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	s.entries[key] = &cacheEntry{
		value:     value,
		expiresAt: expiresAt,
	}

	return nil
}

// Delete removes a cached value.
func (s *InMemoryCacheStore) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, key)
	return nil
}

// cleanup runs periodically to remove expired entries.
func (s *InMemoryCacheStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for key, entry := range s.entries {
			if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
				delete(s.entries, key)
			}
		}
		s.mu.Unlock()
	}
}

// Clear removes all entries from the cache.
func (s *InMemoryCacheStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*cacheEntry)
}

// Size returns the number of entries in the cache.
func (s *InMemoryCacheStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}
