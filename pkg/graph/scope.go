package graph

import (
	"context"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// ReadOnlyScope provides read-only access to state and execution context.
// This is the base interface that Scope embeds.
type ReadOnlyScope interface {
	// NodeName returns the name of the currently executing node.
	// Returns empty string if not in a node execution context.
	NodeName() string

	// GetValue returns the raw value for a key name.
	GetValue(name string) (any, bool)

	// Messages returns the current conversation history.
	// This is the primary way to access the message list.
	Messages() []message.Message

	// LastMessage returns the most recent message in the conversation history.
	// Returns nil if there are no messages.
	LastMessage() message.Message

	// ManagedValues returns the managed values registry, or nil if not configured.
	ManagedValues() *ManagedValueRegistry

	// ToMap returns regular state values as a map for template rendering.
	// Only includes checkpointed state values, NOT managed values.
	// Managed values (ephemeral runtime state) are excluded for safety.
	ToMap() map[string]any
}

// Scope provides the execution context for a node.
// It embeds ReadOnlyScope for state access and adds typed output streaming.
// Output type is fixed to Message for agent workflows.
type Scope interface {
	ReadOnlyScope

	// Stream emits a Message immediately to the graph's output iterator.
	// Use for partial/streaming results during node execution.
	Stream(value message.Message)
}

// scope implements Scope by wrapping a ReadOnlyScope and stream function.
type scope struct {
	ReadOnlyScope                       // Embedded read-only state (includes NodeName)
	stream        func(message.Message) // Output emitter (may be nil if streaming disabled)
}

// newScope creates a new Scope with the given ReadOnlyScope and stream function.
func newScope(ros ReadOnlyScope, stream func(message.Message)) Scope {
	return &scope{ReadOnlyScope: ros, stream: stream}
}

func (s *scope) Stream(value message.Message) {
	if s.stream != nil {
		s.stream(value)
	}
}

// scopeKey is the context key for accessing Scope from tools.
type scopeKey struct{}

// WithScope attaches a Scope to the context.
// This allows tools to access streaming capabilities.
func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// GetScope retrieves the Scope from context.
// Returns nil if scope is not available.
// This is primarily used by tools that need streaming access.
//
// Example usage in a tool:
//
//	func (t *MyTool) Run(ctx context.Context, input string) (string, error) {
//	    if scope := graph.GetScope(ctx); scope != nil {
//	        scope.Stream(graph.NewAIMessageFromText("progress..."))
//	    }
//	    return result, nil
//	}
func GetScope(ctx context.Context) Scope {
	if s, ok := ctx.Value(scopeKey{}).(Scope); ok {
		return s
	}
	return nil
}

// ScopeGet returns the typed value for a key from the scope.
// This is a convenience function that works with Scope.
func ScopeGet[T any](scope Scope, key Key[T]) T {
	if v, ok := scope.GetValue(key.name); ok {
		if typed, ok := v.(T); ok {
			return typed
		}
	}
	return key.Zero()
}

// ScopeGetList returns the typed list for a slice key from the scope.
func ScopeGetList[T any](scope Scope, key Key[[]T]) []T {
	if v, ok := scope.GetValue(key.name); ok {
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

func (v *readOnlyScope) Messages() []message.Message {
	if val, ok := v.data[MessagesKeyName]; ok {
		if msgs, ok := val.([]message.Message); ok {
			return msgs
		}
	}
	return nil
}

func (v *readOnlyScope) LastMessage() message.Message {
	msgs := v.Messages()
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
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
