package memory

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// StoredMemory is the internal representation persisted by InMemoryStore.
// It mirrors the core.SearchResult shape (ID, content, metadata) without a
// score field since scoring is trivial here.
type StoredMemory struct {
	ID       string
	Content  string
	Metadata map[string]any
}

// InMemoryStore is a naive process‑local MemoryStore. It offers:
//  1. Session scoped key/value memory (Get / Put)
//  2. Append‑only stored memories with substring Search
//
// Concurrency: protected by RWMutex.
// Search: linear scan with substring matching (case sensitive) assigning a
// constant score of 1.0 to every hit. Suitable only for tests / demos; swap for
// a vector DB or semantic index for production retrieval.
type InMemoryStore struct {
	mu      sync.RWMutex
	memory  map[string]map[string]any          // sessionID -> key -> value
	storage map[string]map[string]StoredMemory // sessionID -> memoryID -> stored memory
}

// NewInMemoryStore creates a new in-memory memory store
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		memory:  make(map[string]map[string]any),
		storage: make(map[string]map[string]StoredMemory),
	}
}

// Get returns a shallow copy of the key/value memory map for the session.
func (m *InMemoryStore) Get(sessionID string) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessionMemory, exists := m.memory[sessionID]
	if !exists {
		return make(map[string]any), nil
	}
	result := make(map[string]any, len(sessionMemory))
	for k, v := range sessionMemory {
		result[k] = v
	}
	return result, nil
}

// Put merges the provided delta map into the session's key/value memory.
func (m *InMemoryStore) Put(sessionID string, delta map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.memory[sessionID]; !exists {
		m.memory[sessionID] = make(map[string]any)
	}
	for k, v := range delta {
		m.memory[sessionID][k] = v
	}
	return nil
}

// Search performs a simple substring match over stored memories. Results are
// returned in unspecified order up to the provided limit. Each result receives
// a constant score of 1.0.
func (m *InMemoryStore) Search(sessionID string, query string, limit int) ([]core.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessionStorage, exists := m.storage[sessionID]
	if !exists {
		return []core.SearchResult{}, nil
	}
	results := make([]core.SearchResult, 0, limit)
	count := 0
	for _, stored := range sessionStorage {
		if count >= limit {
			break
		}
		if query == "" || strings.Contains(stored.Content, query) {
			md := make(map[string]interface{}, len(stored.Metadata))
			for k, v := range stored.Metadata {
				md[k] = v
			}
			results = append(results, core.SearchResult{ID: stored.ID, Content: stored.Content, Score: 1.0, Metadata: md})
			count++
		}
	}
	return results, nil
}

// Store appends a new stored memory generating a simple incremental id.
func (m *InMemoryStore) Store(sessionID string, content string, metadata map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.storage[sessionID]; !exists {
		m.storage[sessionID] = make(map[string]StoredMemory)
	}
	memoryID := fmt.Sprintf("mem_%d", len(m.storage[sessionID]))
	m.storage[sessionID][memoryID] = StoredMemory{ID: memoryID, Content: content, Metadata: metadata}
	return nil
}

// Delete removes a stored memory entry by id.
func (m *InMemoryStore) Delete(sessionID string, memoryID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sessionStorage, exists := m.storage[sessionID]
	if !exists {
		return fmt.Errorf("memory not found")
	}
	if _, exists := sessionStorage[memoryID]; !exists {
		return fmt.Errorf("memory not found")
	}
	delete(sessionStorage, memoryID)
	return nil
}
