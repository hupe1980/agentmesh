package state

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
	return val.(T) // Safe: key registration enforces type
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
