package viz

import (
	"fmt"
	"sync"
)

// Registry manages registered graphs for visualization.
// It provides thread-safe storage and retrieval of viz.Runnable instances.
type Registry struct {
	mu       sync.RWMutex
	runnables map[string]Runnable
}

// NewRegistry creates a new graph registry.
func NewRegistry() *Registry {
	return &Registry{
		runnables: make(map[string]Runnable),
	}
}

// Register adds a runnable to the registry.
// Returns an error if the ID is already registered.
func (r *Registry) Register(id string, runnable Runnable) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runnables[id]; exists {
		return fmt.Errorf("runnable already registered: %s", id)
	}

	r.runnables[id] = runnable
	return nil
}

// Get retrieves a runnable from the registry.
// Returns an error if the ID is not found.
func (r *Registry) Get(id string) (Runnable, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	runnable, exists := r.runnables[id]
	if !exists {
		return nil, fmt.Errorf("runnable not found: %s", id)
	}

	return runnable, nil
}

// List returns all registered runnable IDs.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.runnables))
	for id := range r.runnables {
		ids = append(ids, id)
	}

	return ids
}

// Unregister removes a runnable from the registry.
// Returns an error if the ID is not found.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.runnables[id]; !exists {
		return fmt.Errorf("runnable not found: %s", id)
	}

	delete(r.runnables, id)
	return nil
}
