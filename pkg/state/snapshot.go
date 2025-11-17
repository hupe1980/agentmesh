package state

import "reflect"

// Snapshot is an immutable point-in-time view of state.
// Safe for concurrent access without locks.
// Used in BSP execution to give all vertices a consistent view during a superstep.
type Snapshot struct {
	data    map[string]any
	version uint64
}

// Get retrieves a typed value from the snapshot.
// Returns the key's zero value if the key doesn't exist.
func GetFromSnapshot[T any](snap *Snapshot, key Key[T]) T {
	val, ok := snap.data[key.name]
	if !ok {
		return key.zero
	}

	// Try direct type assertion first (fast path)
	if typed, ok := val.(T); ok {
		return typed
	}

	// Handle slice conversion from []any to []T (for list keys)
	// When TopicChannel stores []T, it unpacks to []any elements
	// We need to repack them into the correct []T type
	var zero T
	targetType := reflect.TypeOf(zero)
	if targetType.Kind() == reflect.Slice {
		sourceSlice, ok := val.([]any)
		if !ok {
			// Not a []any, fall back to original type assertion (will panic if wrong type)
			return val.(T)
		}

		// Create a new slice of the target type
		resultSlice := reflect.MakeSlice(targetType, len(sourceSlice), len(sourceSlice))
		for i, elem := range sourceSlice {
			resultSlice.Index(i).Set(reflect.ValueOf(elem))
		}
		return resultSlice.Interface().(T)
	}

	// Non-slice type, use direct assertion (may panic if type mismatch)
	return val.(T)
}

// Has checks if a key exists in the snapshot.
func (snap *Snapshot) Has(keyName string) bool {
	_, ok := snap.data[keyName]
	return ok
}

// Keys returns all registered keys in the snapshot.
func (snap *Snapshot) Keys() []string {
	keys := make([]string, 0, len(snap.data))
	for k := range snap.data {
		keys = append(keys, k)
	}
	return keys
}

// Version returns the snapshot's version number.
func (snap *Snapshot) Version() uint64 {
	return snap.version
}

// Data returns the raw data map (for checkpointing).
func (snap *Snapshot) Data() map[string]any {
	// Return a copy to prevent external mutation
	data := make(map[string]any, len(snap.data))
	for k, v := range snap.data {
		data[k] = v
	}
	return data
}
