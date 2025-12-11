package testutil

import "github.com/hupe1980/agentmesh/pkg/graph"

// TestScope implements graph.Scope[O] for testing.
// It wraps a ReadOnlyScope and captures streamed values.
type TestScope[O any] struct {
	readOnly graph.ReadOnlyScope
	stream   func(O)
	nodeName string // Optional node name for testing middleware
	Streamed []O    // Captured streamed values
}

// NewTestScope creates a Scope for testing purposes.
// The stream function can be nil if streaming is not needed for the test.
// If you want to capture streamed values, use NewTestScopeWithCapture instead.
func NewTestScope[O any](readOnly graph.ReadOnlyScope, stream func(O)) *TestScope[O] {
	return &TestScope[O]{
		readOnly: readOnly,
		stream:   stream,
	}
}

// NewTestScopeWithCapture creates a test scope that captures all streamed values.
// Use ts.Streamed to inspect captured values after node execution.
func NewTestScopeWithCapture[O any](readOnly graph.ReadOnlyScope) *TestScope[O] {
	ts := &TestScope[O]{
		readOnly: readOnly,
		Streamed: make([]O, 0),
	}
	ts.stream = func(v O) {
		ts.Streamed = append(ts.Streamed, v)
	}
	return ts
}

// NewTestScopeFromMap creates a test scope from a simple map.
// This is a convenience function for tests that don't need complex state setup.
func NewTestScopeFromMap[O any](data map[string]any) *TestScope[O] {
	return NewTestScopeWithCapture[O](graph.NewBSPState(data).ReadView())
}

// GetValue implements graph.Scope.
func (s *TestScope[O]) GetValue(name string) (any, bool) {
	return s.readOnly.GetValue(name)
}

// ManagedValues implements graph.Scope.
func (s *TestScope[O]) ManagedValues() *graph.ManagedValueRegistry {
	return s.readOnly.ManagedValues()
}

// ToMap implements graph.Scope.
func (s *TestScope[O]) ToMap() map[string]any {
	return s.readOnly.ToMap()
}

// NodeName implements graph.Scope.
func (s *TestScope[O]) NodeName() string {
	return s.nodeName
}

// WithNodeName sets the node name for this test scope and returns the scope for chaining.
func (s *TestScope[O]) WithNodeName(name string) *TestScope[O] {
	s.nodeName = name
	return s
}

// Stream implements graph.Scope.
func (s *TestScope[O]) Stream(value O) {
	if s.stream != nil {
		s.stream(value)
	}
}
