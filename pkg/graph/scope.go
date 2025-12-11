package graph

import "context"

// ReadOnlyScope provides read-only access to state and execution context.
// This is the base interface that Scope[O] embeds.
type ReadOnlyScope interface {
	// NodeName returns the name of the currently executing node.
	// Returns empty string if not in a node execution context.
	NodeName() string

	// GetValue returns the raw value for a key name.
	GetValue(name string) (any, bool)

	// ManagedValues returns the managed values registry, or nil if not configured.
	ManagedValues() *ManagedValueRegistry

	// ToMap returns regular state values as a map for template rendering.
	// Only includes checkpointed state values, NOT managed values.
	// Managed values (ephemeral runtime state) are excluded for safety.
	ToMap() map[string]any
}

// Scope provides the execution context for a node.
// It embeds ReadOnlyScope for state access and adds typed output streaming.
// The type parameter O matches the graph's output type.
type Scope[O any] interface {
	ReadOnlyScope

	// Stream emits an output value immediately to the graph's output iterator.
	// Use for partial/streaming results during node execution.
	// This is type-safe: the value type must match the graph's output type O.
	Stream(value O)
}

// scope implements Scope[O] by wrapping a ReadOnlyScope and stream function.
type scope[O any] struct {
	ReadOnlyScope         // Embedded read-only state (includes NodeName)
	stream        func(O) // Output emitter (may be nil if streaming disabled)
}

// newScope creates a new Scope with the given ReadOnlyScope and stream function.
func newScope[O any](ros ReadOnlyScope, stream func(O)) Scope[O] {
	return &scope[O]{ReadOnlyScope: ros, stream: stream}
}

func (s *scope[O]) Stream(value O) {
	if s.stream != nil {
		s.stream(value)
	}
}

// scopeKey is the context key for accessing Scope from tools.
type scopeKey struct{}

// WithScope attaches a Scope to the context.
// This allows tools to access streaming capabilities.
func WithScope[O any](ctx context.Context, scope Scope[O]) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// GetScope retrieves the Scope from context.
// Returns nil if scope is not available or type doesn't match.
// This is primarily used by tools that need streaming access.
//
// Example usage in a tool:
//
//	func (t *MyTool) Run(ctx context.Context, input string) (string, error) {
//	    if scope := graph.GetScope[message.Message](ctx); scope != nil {
//	        scope.Stream(message.NewAIMessageFromText("progress..."))
//	    }
//	    return result, nil
//	}
func GetScope[O any](ctx context.Context) Scope[O] {
	if s, ok := ctx.Value(scopeKey{}).(Scope[O]); ok {
		return s
	}
	return nil
}

// ScopeGet returns the typed value for a key from the scope.
// This is a convenience function that works with Scope[O].
func ScopeGet[T any, O any](scope Scope[O], key Key[T]) T {
	if v, ok := scope.GetValue(key.name); ok {
		if typed, ok := v.(T); ok {
			return typed
		}
	}
	return key.zero
}

// ScopeGetList returns the typed list for a list key from the scope.
// Handles both []T and SliceOf[T] storage formats.
func ScopeGetList[T any, O any](scope Scope[O], key ListKey[T]) []T {
	if v, ok := scope.GetValue(key.name); ok {
		// Handle SliceOf[T] (used by Append/AppendValue for zero-reflection)
		if sliceOf, ok := v.(SliceOf[T]); ok {
			return sliceOf
		}
		// Handle plain []T (legacy or external sources)
		if typed, ok := v.([]T); ok {
			return typed
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// readOnlyScope implementation
// -----------------------------------------------------------------------------

// readOnlyScope implements ReadOnlyScope for reading state.
// This is the base implementation without node context.
type readOnlyScope struct {
	data    map[string]any
	managed *ManagedValueRegistry
}

func (v *readOnlyScope) NodeName() string {
	return "" // No node context in base implementation
}

func (v *readOnlyScope) GetValue(name string) (any, bool) {
	val, ok := v.data[name]
	return val, ok
}

func (v *readOnlyScope) ManagedValues() *ManagedValueRegistry {
	return v.managed
}

// ToMap returns a copy of the state values for template rendering.
// Does not include managed values.
func (v *readOnlyScope) ToMap() map[string]any {
	result := make(map[string]any, len(v.data))
	for k, val := range v.data {
		result[k] = val
	}
	return result
}

// -----------------------------------------------------------------------------
// nodeScope wrapper - adds node name to any ReadOnlyScope
// -----------------------------------------------------------------------------

// nodeScope wraps a ReadOnlyScope and adds node execution context.
// This allows the underlying state view to be cached/shared while
// providing per-node identity.
type nodeScope struct {
	ReadOnlyScope        // Embedded state view
	nodeName      string // Name of the executing node
}

// withNodeName wraps a ReadOnlyScope with node context.
func withNodeName(ros ReadOnlyScope, nodeName string) ReadOnlyScope {
	return &nodeScope{ReadOnlyScope: ros, nodeName: nodeName}
}

func (s *nodeScope) NodeName() string {
	return s.nodeName
}
