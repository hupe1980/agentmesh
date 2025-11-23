package state

import (
	"fmt"
	"strings"
)

// ReadView is the interface for read-only access to state.
// This is what nodes receive for concurrent safe reads.
// Implementations include readView (full state) and NamespacedReadView (filtered).
type ReadView interface {
	// Has checks if a key exists in the view.
	Has(keyName string) bool

	// Keys returns all accessible keys in the view.
	Keys() []string

	// Version returns the view's version.
	Version() uint64

	// snapshot returns the underlying snapshot (internal use only).
	snapshot() *Snapshot
}

// readView is the concrete implementation of ReadView for full state access.
// This is the default view that nodes receive unless they use NamespacedCommandNode.
type readView struct {
	snap *Snapshot // Internal, cannot access underlying State
}

// NewReadView creates a read-only view from a snapshot.
func NewReadView(snap *Snapshot) ReadView {
	return &readView{snap: snap}
}

// GetFromView retrieves a typed value from the view.
func GetFromView[T any](rv ReadView, key Key[T]) T {
	return GetFromSnapshot(rv.snapshot(), key)
}

// Has checks if a key exists in the view.
func (rv *readView) Has(keyName string) bool {
	return rv.snap.Has(keyName)
}

// Version returns the view's version.
func (rv *readView) Version() uint64 {
	return rv.snap.Version()
}

// Keys returns all keys in the view.
func (rv *readView) Keys() []string {
	return rv.snap.Keys()
}

// snapshot returns the underlying snapshot.
func (rv *readView) snapshot() *Snapshot {
	return rv.snap
}

// NamespacedReadView provides read-only access to state filtered by namespace.
// Only keys belonging to the specified namespace are visible.
// Optionally includes global (non-namespaced) keys if includeGlobal is true.
// This enables node-level state isolation and implements the ReadView interface.
type NamespacedReadView struct {
	view          ReadView
	namespace     Namespace
	includeGlobal bool
}

// NewNamespacedReadView creates a namespace-scoped read view.
// The view will only expose keys from the specified namespace.
// If includeGlobal is true, global (non-namespaced) keys are also visible.
//
// Example:
//
//	agentNS := state.MustNamespace("agent1")
//	scopedView := state.NewNamespacedReadView(view, agentNS, false)
//	// scopedView can only access "agent1.*" keys
//
//	scopedViewWithGlobal := state.NewNamespacedReadView(view, agentNS, true)
//	// scopedViewWithGlobal can access "agent1.*" keys and global keys
func NewNamespacedReadView(view ReadView, ns Namespace, includeGlobal bool) *NamespacedReadView {
	return &NamespacedReadView{
		view:          view,
		namespace:     ns,
		includeGlobal: includeGlobal,
	}
}

// GetFromNamespacedView retrieves a typed value from a namespace-scoped view.
// Panics if the key doesn't belong to the view's namespace.
//
// Example:
//
//	agentNS := state.MustNamespace("agent1")
//	scopedView := state.NewNamespacedReadView(view, agentNS)
//	statusKey := state.TypedKey[string](agentNS, "status", "")
//	status := state.GetFromNamespacedView(scopedView, statusKey)
func GetFromNamespacedView[T any](nv *NamespacedReadView, key Key[T]) T {
	// Verify key belongs to this namespace
	if !nv.namespace.IsGlobal() {
		expectedPrefix := nv.namespace.name + "."
		if !strings.HasPrefix(key.name, expectedPrefix) {
			panic(fmt.Sprintf("key %q does not belong to namespace %q", key.name, nv.namespace.name))
		}
	}
	return GetFromView(nv.view, key)
}

// Has checks if a key exists in the namespace-scoped view.
// Only returns true for keys belonging to this namespace.
func (nv *NamespacedReadView) Has(keyName string) bool {
	if !nv.isNamespacedKey(keyName) {
		return false
	}
	return nv.view.Has(keyName)
}

// Keys returns only keys from this namespace.
func (nv *NamespacedReadView) Keys() []string {
	allKeys := nv.view.Keys()
	filtered := make([]string, 0, len(allKeys))
	for _, key := range allKeys {
		if nv.isNamespacedKey(key) {
			filtered = append(filtered, key)
		}
	}
	return filtered
}

// Version returns the underlying view's version.
func (nv *NamespacedReadView) Version() uint64 {
	return nv.view.Version()
}

// Namespace returns the namespace this view is scoped to.
func (nv *NamespacedReadView) Namespace() Namespace {
	return nv.namespace
}

// UnderlyingView returns the full read view (for internal use).
func (nv *NamespacedReadView) UnderlyingView() ReadView {
	return nv.view
}

// snapshot returns the underlying snapshot (implements ReadView interface).
func (nv *NamespacedReadView) snapshot() *Snapshot {
	return nv.view.snapshot()
}

// isNamespacedKey checks if a key belongs to this namespace or is global (if includeGlobal is true).
func (nv *NamespacedReadView) isNamespacedKey(keyName string) bool {
	if nv.namespace.IsGlobal() {
		// Global namespace sees all keys without dots
		return !strings.Contains(keyName, ".")
	}

	// Check if it's a global key (no namespace prefix)
	if !strings.Contains(keyName, ".") {
		return nv.includeGlobal
	}

	// Check prefix match for namespaced keys
	prefix := nv.namespace.name + "."
	return strings.HasPrefix(keyName, prefix)
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
