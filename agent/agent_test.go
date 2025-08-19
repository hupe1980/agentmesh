package agent

import (
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockAgent for testing composite agents
type MockAgent struct {
	mock.Mock
	name string
}

func NewMockAgent(name string) *MockAgent {
	return &MockAgent{name: name}
}

func (m *MockAgent) Name() string { return m.name }

func (m *MockAgent) Run(invocationCtx *core.InvocationContext) error {
	args := m.Called(invocationCtx)
	return args.Error(0)
}

func (m *MockAgent) Start(invocationCtx *core.InvocationContext) error {
	args := m.Called(invocationCtx)
	return args.Error(0)
}

func (m *MockAgent) Stop(invocationCtx *core.InvocationContext) error {
	args := m.Called(invocationCtx)
	return args.Error(0)
}

func (m *MockAgent) SubAgents() []core.Agent {
	args := m.Called()
	return args.Get(0).([]core.Agent)
}

func (m *MockAgent) AddSubAgent(agent core.Agent) { m.Called(agent) }

func (m *MockAgent) RemoveSubAgent(id string) bool {
	args := m.Called(id)
	return args.Bool(0)
}

func (m *MockAgent) SetOutputKey(key string) { m.Called(key) }

func (m *MockAgent) SubstituteTemplateVariables(template string, state map[string]interface{}) string {
	args := m.Called(template, state)
	return args.String(0)
}

func (m *MockAgent) Description() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockAgent) SetSubAgents(children ...core.Agent) error {
	args := m.Called(children)
	return args.Error(0)
}

func (m *MockAgent) Parent() core.Agent {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(core.Agent)
}

func (m *MockAgent) FindAgent(name string) core.Agent {
	args := m.Called(name)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(core.Agent)
}

// Common test utilities
func TestNewEventID(t *testing.T) {
	eventID := util.NewID()
	assert.NotEmpty(t, eventID)
	assert.Len(t, eventID, 36) // UUID length
}

func TestStringPtr(t *testing.T) {
	str := "test"
	ptr := stringPtr(str)
	assert.NotNil(t, ptr)
	assert.Equal(t, str, *ptr)
}

func TestBoolPtr(t *testing.T) {
	ptr := boolPtr(true)
	assert.NotNil(t, ptr)
	assert.True(t, *ptr)

	ptr = boolPtr(false)
	assert.NotNil(t, ptr)
	assert.False(t, *ptr)
}
