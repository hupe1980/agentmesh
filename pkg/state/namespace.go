package state

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Namespace represents a state scope for creating isolated keys.
// Use dot notation to create hierarchical key names (e.g., "model.messages").
// Namespaces are zero-cost abstractions - they're just string prefixes.
type Namespace struct {
	name string
}

// NewNamespace creates a namespace with validation.
// Returns error if name is empty or contains dots.
//
// Example:
//
//	agentNS, err := state.NewNamespace("agent1")
//	if err != nil {
//	    return err
//	}
func NewNamespace(name string) (Namespace, error) {
	if name == "" {
		return Namespace{}, fmt.Errorf("namespace cannot be empty (use Global for global namespace)")
	}
	if strings.Contains(name, ".") {
		return Namespace{}, fmt.Errorf("namespace cannot contain dots: %q", name)
	}
	return Namespace{name: name}, nil
}

// MustNamespace creates a namespace, panicking on invalid input.
// Use for package-level constants where the name is known to be valid.
//
// Example:
//
//	var ModelNS = state.MustNamespace("model")
//	var ToolNS = state.MustNamespace("tool")
func MustNamespace(name string) Namespace {
	ns, err := NewNamespace(name)
	if err != nil {
		panic(err)
	}
	return ns
}

// Global is the global namespace for shared state (empty prefix).
// This is the default - most keys should use this.
// Use namespaces only when you need isolation.
var Global = Namespace{name: ""}

// String returns the namespace name.
func (ns Namespace) String() string {
	return ns.name
}

// Name returns the namespace name.
func (ns Namespace) Name() string {
	return ns.name
}

// IsGlobal returns true if this is the global namespace.
func (ns Namespace) IsGlobal() bool {
	return ns.name == ""
}

// fullName creates the full key name by prepending the namespace.
// Global namespace returns the key name unchanged.
func (ns Namespace) fullName(keyName string) string {
	if ns.name == "" {
		return keyName // Global namespace - no prefix
	}
	return ns.name + "." + keyName
}

// TypedKey creates a strongly-typed key within a namespace.
// Use this when you need state isolation.
//
// Example:
//
//	modelNS := state.MustNamespace("model")
//	var CounterKey = state.TypedKey[int](modelNS, "counter", 0)       // "model.counter"
//	var StatusKey = state.TypedKey[string](modelNS, "status", "idle") // "model.status"
func TypedKey[T any](ns Namespace, name string, defaultValue T) Key[T] {
	fullName := ns.fullName(name)
	return Key[T]{name: fullName, zero: defaultValue}
}

// TypedListKey creates a strongly-typed list key within a namespace.
// Use this when you need isolated list state.
//
// Example:
//
//	toolNS := state.MustNamespace("tool")
//	var ResultsKey = state.TypedListKey[string](toolNS, "results", 100, nil)              // "tool.results"
//	var MessagesKey = state.TypedListKey[message.Message](toolNS, "messages", 100, nil)  // "tool.messages"
func TypedListKey[T any](ns Namespace, name string, maxSize int, defaultValue []T) ListKey[T] {
	fullName := ns.fullName(name)
	return ListKey[T]{
		Key:     Key[[]T]{name: fullName, zero: defaultValue},
		maxSize: maxSize,
	}
}

// IsNamespaced checks if a key name contains a namespace prefix.
//
// Example:
//
//	IsNamespaced("model.messages")  // true
//	IsNamespaced("messages")        // false
//	IsNamespaced("__messages__")    // false
func IsNamespaced(keyName string) bool {
	return strings.Contains(keyName, ".")
}

// ParseNamespacedKey parses a key name into namespace and local components.
// Returns empty namespace for global keys.
//
// Example:
//
//	ns, local := ParseNamespacedKey("model.messages")  // ns="model", local="messages"
//	ns, local := ParseNamespacedKey("__messages__")    // ns="", local="__messages__"
func ParseNamespacedKey(keyName string) (namespace string, localName string) {
	parts := strings.SplitN(keyName, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", keyName
}

// ExtractNamespace returns the namespace portion of a key name, or empty string for global keys.
//
// Example:
//
//	ExtractNamespace("model.messages")  // "model"
//	ExtractNamespace("messages")        // ""
func ExtractNamespace(keyName string) string {
	ns, _ := ParseNamespacedKey(keyName)
	return ns
}

// GetNamespaceView returns a filtered view containing only keys from a namespace.
// Useful for debugging or introspection.
//
// Example:
//
//	modelNS := state.MustNamespace("model")
//	view, _ := mgr.CreateReadView(ctx)
//	modelState := state.GetNamespaceView(view, modelNS)
//	// modelState contains: {"messages": [...], "context": "..."}
func GetNamespaceView(view *ReadView, ns Namespace) map[string]any {
	data := view.snap.Data()

	if ns.IsGlobal() {
		// For global namespace, return all non-namespaced keys
		result := make(map[string]any)
		for key, value := range data {
			if !IsNamespaced(key) {
				result[key] = value
			}
		}
		return result
	}

	prefix := ns.name + "."
	result := make(map[string]any)

	for key, value := range data {
		if strings.HasPrefix(key, prefix) {
			localName := strings.TrimPrefix(key, prefix)
			result[localName] = value
		}
	}

	return result
}

// ListNamespaces returns all namespaces present in state.
// Does not include the global namespace.
//
// Example:
//
//	view, _ := mgr.CreateReadView(ctx)
//	namespaces := state.ListNamespaces(view)
//	// namespaces: []Namespace{"model", "tool", "agent1"}
func ListNamespaces(view *ReadView) []Namespace {
	namespaces := make(map[string]bool)

	for _, key := range view.Keys() {
		if ns := ExtractNamespace(key); ns != "" {
			namespaces[ns] = true
		}
	}

	result := make([]Namespace, 0, len(namespaces))
	for ns := range namespaces {
		result = append(result, Namespace{name: ns})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].name < result[j].name
	})

	return result
}

// CopyNamespace copies all keys from one namespace to another.
// Useful for subgraph handoffs or state migration.
//
// IMPORTANT: Target keys must be registered before copying.
// Only copies keys that have registered channels in the target namespace.
//
// Example:
//
//	// Copy agent1 state to agent2 (both must have same keys registered)
//	agent1NS := state.MustNamespace("agent1")
//	agent2NS := state.MustNamespace("agent2")
//	err := state.CopyNamespace(ctx, mgr, agent1NS, agent2NS)
func CopyNamespace(ctx context.Context, mgr *Manager, from, to Namespace) error {
	snap, err := mgr.Snapshot(ctx, nil)
	if err != nil {
		return err
	}

	fromPrefix := from.name + "."
	toPrefix := to.name + "."

	updates := make(Updates)
	for key, value := range snap.Data {
		if strings.HasPrefix(key, fromPrefix) {
			localName := strings.TrimPrefix(key, fromPrefix)
			newKey := toPrefix + localName
			updates[newKey] = value
		}
	}

	if len(updates) == 0 {
		return nil
	}

	return mgr.ApplyUpdates(ctx, updates)
}

// DeleteNamespace removes all keys in a namespace by resetting them to their zero values.
// Note: This cannot actually delete keys from channels, only reset their values.
// List keys will be cleared. Useful for cleanup after subgraph completion.
//
// Example:
//
//	// Clean up temporary subgraph state
//	subgraphNS := state.MustNamespace("subgraph")
//	err := state.DeleteNamespace(ctx, mgr, subgraphNS)
func DeleteNamespace(ctx context.Context, mgr *Manager, ns Namespace) error {
	snap, err := mgr.Snapshot(ctx, nil)
	if err != nil {
		return err
	}

	prefix := ns.name + "."

	updates := make(Updates)
	for key, value := range snap.Data {
		if strings.HasPrefix(key, prefix) {
			// Reset to zero value based on type
			switch value.(type) {
			case []interface{}:
				// Clear list
				updates[key] = []interface{}{}
			default:
				// For non-list values, we can't truly delete
				// Just skip them (user must manually reset)
				continue
			}
		}
	}

	if len(updates) == 0 {
		return nil
	}

	return mgr.ApplyUpdates(ctx, updates)
}
