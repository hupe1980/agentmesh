package testutil

import "github.com/hupe1980/agentmesh/core"

// MockFlowSelector is a test double for core.FlowSelector. Customize the
// SelectFunc to control behaviour; by default it returns nil.
type MockFlowSelector struct {
	// SelectFunc, when non-nil, is invoked to choose a flow. It can be used to
	// assert the provided agent or control the return value.
	SelectFunc func(agent core.FlowAgent) core.Flow
}

// NewMockFlowSelector creates a mock flow selector that returns nil flows by
// default. Tests can override SelectFunc on the returned instance.
func NewMockFlowSelector() *MockFlowSelector {
	return &MockFlowSelector{}
}

// Select implements core.FlowSelector, delegating to SelectFunc when set.
func (m *MockFlowSelector) Select(agent core.FlowAgent) core.Flow {
	if m != nil && m.SelectFunc != nil {
		return m.SelectFunc(agent)
	}

	return nil
}
