package orderedmap

import "sync"

// OrderedMap preserves insertion order for keys while allowing fast lookups.
type OrderedMap[K comparable, V any] struct {
	mu    sync.RWMutex
	keys  []K
	items map[K]V
}

// New creates an empty OrderedMap.
func New[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		keys:  make([]K, 0),
		items: make(map[K]V),
	}
}

// Set inserts or updates a value by key.
// New keys are appended at the end; existing keys retain their position.
func (m *OrderedMap[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.items[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.items[key] = value
}

// Get retrieves a value by key.
func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.items[key]

	return val, ok
}

// Delete removes a key (if present). Order of other keys is preserved.
func (m *OrderedMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.items[key]; !exists {
		return
	}
	delete(m.items, key)

	// Rebuild keys slice without the deleted key
	newKeys := make([]K, 0, len(m.keys)-1)
	for _, k := range m.keys {
		if k != key {
			newKeys = append(newKeys, k)
		}
	}
	m.keys = newKeys
}

// Keys returns all keys in insertion order.
func (m *OrderedMap[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]K, len(m.keys))
	copy(keys, m.keys)

	return keys
}

// Values returns all values in insertion order.
func (m *OrderedMap[K, V]) Values() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()

	values := make([]V, 0, len(m.keys))
	for _, k := range m.keys {
		values = append(values, m.items[k])
	}

	return values
}

// Len returns the number of items.
func (m *OrderedMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.items)
}

// Clear removes all keys and values.
func (m *OrderedMap[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = make(map[K]V)
	m.keys = nil
}
