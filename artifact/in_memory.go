package artifact

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// Interface compliance (compile-time assertions)
var _ core.ArtifactStore = (*InMemoryStore)(nil)

// keyForArtifact builds the composite key for artifact storage.
func keyForArtifact(appName, userID, sessionID, fileName string) string {
	return fmt.Sprintf("%s/%s/%s/%s", appName, userID, sessionID, fileName)
}

// InMemoryStore implements ArtifactStore with a flat map keyed by composite string.
type InMemoryStore struct {
	artifacts map[string]core.Part // key -> Part
	order     []string             // insertion order of keys
	mu        sync.RWMutex
}

// NewInMemoryStore returns an empty in‑memory artifact store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		artifacts: make(map[string]core.Part),
		order:     make([]string, 0, 8),
	}
}

// Save stores (or overwrites) the artifact for the given composite key.
func (a *InMemoryStore) Save(_ context.Context, appName, userID, sessionID, fileName string, artifact core.Part) error {
	key := keyForArtifact(appName, userID, sessionID, fileName)

	a.mu.Lock()
	defer a.mu.Unlock()

	// Track insertion order only on first insert.
	if _, exists := a.artifacts[key]; !exists {
		a.order = append(a.order, key)
	}

	// Store a deep clone to prevent external mutation from affecting the store
	a.artifacts[key] = core.ClonePart(artifact)

	return nil
}

// Load returns a copy of the stored artifact or ErrNotFound.
func (a *InMemoryStore) Load(_ context.Context, appName, userID, sessionID, fileName string) (core.Part, error) {
	key := keyForArtifact(appName, userID, sessionID, fileName)

	a.mu.RLock()
	defer a.mu.RUnlock()

	part, ok := a.artifacts[key]
	if !ok {
		return core.Part(nil), fmt.Errorf("%w: artifact=%s", core.ErrArtifactNotFound, key)
	}

	// Return a deep clone to prevent callers from mutating internal state
	return core.ClonePart(part), nil
}

// ListKeys returns all artifact keys for the given session.
func (a *InMemoryStore) ListKeys(_ context.Context, appName, userID, sessionID string) ([]string, error) {
	prefix := fmt.Sprintf("%s/%s/%s/", appName, userID, sessionID)

	a.mu.RLock()
	defer a.mu.RUnlock()

	// Return a new slice to avoid exposing internal order slice for mutation
	var keys []string
	for _, key := range a.order {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			// Extract fileName from key
			fileName := key[len(prefix):]
			keys = append(keys, fileName)
		}
	}
	return keys, nil
}

// Delete removes the artifact if present or returns ErrNotFound.
func (a *InMemoryStore) Delete(_ context.Context, appName, userID, sessionID, fileName string) error {
	key := keyForArtifact(appName, userID, sessionID, fileName)

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.artifacts[key]; !ok {
		return fmt.Errorf("%w: artifact=%s", core.ErrArtifactNotFound, key)
	}

	delete(a.artifacts, key)

	// Remove from insertion order
	for i, k := range a.order {
		if k == key {
			a.order = append(a.order[:i], a.order[i+1:]...)
			break
		}
	}

	return nil
}

// Close implements core.ArtifactStore. No resources to release for in-memory store.
func (a *InMemoryStore) Close() error { return nil }
