package orderedset

import "sync"

// Set is a generic ordered set that enforces uniqueness
// and preserves insertion order. Not safe for concurrent use
// unless wrapped externally.
type Set[T comparable] struct {
	mu    sync.RWMutex
	items map[T]struct{}
	order []T
}

// New creates an empty ordered set.
func New[T comparable]() *Set[T] {
	return &Set[T]{
		items: make(map[T]struct{}),
		order: make([]T, 0),
	}
}

// Add inserts a new element if it is not already present.
// Returns true if the element was added, false if it was already in the set.
func (s *Set[T]) Add(val T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.items[val]; exists {
		return false
	}
	s.items[val] = struct{}{}
	s.order = append(s.order, val)
	return true
}

// Remove deletes an element if it exists. Returns true if removed.
func (s *Set[T]) Remove(val T) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.items[val]; !exists {
		return false
	}
	delete(s.items, val)

	// maintain order
	for i, v := range s.order {
		if v == val {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}

	return true
}

// Contains reports whether the element is in the set.
func (s *Set[T]) Contains(val T) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.items[val]

	return exists
}

// Values returns elements in insertion order.
func (s *Set[T]) Values() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]T, len(s.order))
	copy(result, s.order)

	return result
}

// Len returns the number of elements in the set.
func (s *Set[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.order)
}
