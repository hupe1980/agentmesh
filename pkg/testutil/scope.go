package testutil

import (
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// TestScope implements graph.Scope for testing.
// It wraps a ReadOnlyScope and captures streamed values.
type TestScope struct {
	readOnly graph.ReadOnlyScope
	stream   func(message.Message)
	nodeName string            // Optional node name for testing middleware
	Streamed []message.Message // Captured streamed values
}

// NewTestScope creates a Scope for testing purposes.
// The stream function can be nil if streaming is not needed for the test.
// If you want to capture streamed values, use NewTestScopeWithCapture instead.
func NewTestScope(readOnly graph.ReadOnlyScope, stream func(message.Message)) *TestScope {
	return &TestScope{
		readOnly: readOnly,
		stream:   stream,
	}
}

// NewTestScopeWithCapture creates a test scope that captures all streamed values.
// Use ts.Streamed to inspect captured values after node execution.
func NewTestScopeWithCapture(readOnly graph.ReadOnlyScope) *TestScope {
	ts := &TestScope{
		readOnly: readOnly,
		Streamed: make([]message.Message, 0),
	}
	ts.stream = func(v message.Message) {
		ts.Streamed = append(ts.Streamed, v)
	}
	return ts
}

// NewTestScopeFromMap creates a test scope from a simple map.
// This is a convenience function for tests that don't need complex state setup.
func NewTestScopeFromMap(data map[string]any) *TestScope {
	return NewTestScopeWithCapture(graph.NewBSPState(data, graph.NewKeyRegistry()).ReadView())
}

// GetValue implements graph.Scope.
func (s *TestScope) GetValue(name string) (any, bool) {
	return s.readOnly.GetValue(name)
}

// ManagedValues implements graph.Scope.
func (s *TestScope) ManagedValues() *graph.ManagedValueRegistry {
	return s.readOnly.ManagedValues()
}

// ToMap implements graph.Scope.
func (s *TestScope) ToMap() map[string]any {
	return s.readOnly.ToMap()
}

// NodeName implements graph.Scope.
func (s *TestScope) NodeName() string {
	return s.nodeName
}

// WithNodeName sets the node name for this test scope and returns the scope for chaining.
func (s *TestScope) WithNodeName(name string) *TestScope {
	s.nodeName = name
	return s
}

// Stream implements graph.Scope.
func (s *TestScope) Stream(value message.Message) {
	if s.stream != nil {
		s.stream(value)
	}
}

// Messages implements graph.Scope.
func (s *TestScope) Messages() []message.Message {
	return graph.GetMessages(s.readOnly)
}

// LastMessage implements graph.Scope.
func (s *TestScope) LastMessage() message.Message {
	msgs := s.Messages()
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}
