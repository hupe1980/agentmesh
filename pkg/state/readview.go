package state

// ReadView provides read-only access to state (truly immutable).
// This is what nodes receive for concurrent safe reads.
// Cannot mutate underlying state - enforces separation at the type level.
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

// Updates represents state modifications to be applied.
// This is a type alias for map[string]any for compatibility.
//
// For type-safe updates, use UpdateBuilder instead:
//
//	builder := NewUpdateBuilder()
//	SetUpdate(builder, counterKey, 42)
//	AppendUpdate(builder, messagesKey, msg1, msg2)
//	updates, err := builder.Build()
type Updates map[string]any

// NoUpdate returns an empty updates map, indicating no state changes.
func NoUpdate() Updates {
	return map[string]any{}
}
