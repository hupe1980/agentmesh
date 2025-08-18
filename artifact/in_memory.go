package artifact

import "sync"

// InMemoryStore is a trivial in‑process ArtifactStore implementation useful
// for tests, examples and single‑process prototypes. It keeps all artifacts in
// a nested map guarded by an RWMutex. Data is copied on save / retrieval to
// avoid accidental external mutation of internal buffers.
//
// Layout: sessionID -> artifactID -> raw bytes
//
// This implementation is intentionally minimal; it does not enforce retention
// limits, size quotas, or eviction. For production, prefer a durable
// implementation (e.g. S3 / GCS / database) that can scale and survive process
// restarts.
type InMemoryStore struct {
	mu        sync.RWMutex
	artifacts map[string]map[string][]byte // sessionID -> artifactID -> data
}

// NewInMemoryStore returns an empty in‑memory artifact store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{artifacts: make(map[string]map[string][]byte)}
}

// Save stores (or overwrites) the artifact bytes for the given session and id.
// The input slice is copied before storage.
func (a *InMemoryStore) Save(sessionID, artifactID string, data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.artifacts[sessionID]; !exists {
		a.artifacts[sessionID] = make(map[string][]byte)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	a.artifacts[sessionID][artifactID] = cp
	return nil
}

// Get returns a copy of the stored artifact bytes or ErrNotFound.
func (a *InMemoryStore) Get(sessionID, artifactID string) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	m, ok := a.artifacts[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	data, ok := m[artifactID]
	if !ok {
		return nil, ErrNotFound
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	return cp, nil
}

// List returns the artifact ids stored for the session. The slice is
// a snapshot and safe for caller mutation.
func (a *InMemoryStore) List(sessionID string) ([]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	m, ok := a.artifacts[sessionID]
	if !ok {
		return []string{}, nil
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	return ids, nil
}

// Delete removes the artifact if present or returns ErrNotFound.
func (a *InMemoryStore) Delete(sessionID, artifactID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	m, ok := a.artifacts[sessionID]
	if !ok {
		return ErrNotFound
	}
	if _, ok := m[artifactID]; !ok {
		return ErrNotFound
	}
	delete(m, artifactID)
	return nil
}
