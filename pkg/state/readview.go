package state

// ReadView provides read-only access to state (truly immutable).
// This is what nodes receive - cannot be cast to State.
// Enforces interface segregation at runtime.
type ReadView struct {
	snap *Snapshot // Internal, cannot access underlying State
}

// NewReadView creates a read-only view from a snapshot.
func NewReadView(snap *Snapshot) *ReadView {
	return &ReadView{snap: snap}
}

// GetFromView retrieves a typed value from the view.
func GetFromView[T any](rv *ReadView, key Key[T]) T {
	return GetFromSnapshot(rv.snap, key)
}

// Has checks if a key exists in the view.
func (rv *ReadView) Has(keyName string) bool {
	return rv.snap.Has(keyName)
}

// Version returns the view's version.
func (rv *ReadView) Version() uint64 {
	return rv.snap.Version()
}

// Keys returns all keys in the view.
func (rv *ReadView) Keys() []string {
	return rv.snap.Keys()
}

// Updates is a type-safe update builder.
// Validated when applied, not when built.
type Updates map[string]any

// NewUpdates creates a new updates builder.
func NewUpdates() Updates {
	return make(Updates)
}

// SetInUpdates adds a typed key-value pair to the updates.
func SetInUpdates[T any](u Updates, key Key[T], value T) Updates {
	u[key.name] = value
	return u
}

// AppendInUpdates adds a typed value to append to a list.
// Note: This just adds the value to updates. The actual append
// logic happens in State.ApplyUpdates or via Append() function.
func AppendInUpdates[T any](u Updates, key ListKey[T], value T) Updates {
	// For now, just set the value. The executor will handle appending.
	// TODO: Add special marker if needed for append vs set semantics
	u[key.name] = value
	return u
}
