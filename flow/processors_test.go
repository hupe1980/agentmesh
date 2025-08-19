package flow

import (
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/model"
)

type MockFlowAgent struct {
	name      string
	llm       model.Model
	subAgents []FlowAgent
}

func NewMockFlowAgent(name string) *MockFlowAgent {
	return &MockFlowAgent{name: name, subAgents: []FlowAgent{}}
}
func (m *MockFlowAgent) GetName() string                { return m.name }
func (m *MockFlowAgent) GetLLM() model.Model            { return m.llm }
func (m *MockFlowAgent) GetTools() map[string]any       { return map[string]any{} }
func (m *MockFlowAgent) GetSubAgents() []FlowAgent      { return m.subAgents }
func (m *MockFlowAgent) IsFunctionCallingEnabled() bool { return false }
func (m *MockFlowAgent) IsStreamingEnabled() bool       { return false }
func (m *MockFlowAgent) IsTransferEnabled() bool        { return false }
func (m *MockFlowAgent) GetOutputKey() string           { return "" }
func (m *MockFlowAgent) MaxHistoryMessages() int        { return 10 }
func (m *MockFlowAgent) ResolveInstructions(_ *core.RunContext) (string, error) {
	return "You are a test assistant.", nil
}
func (m *MockFlowAgent) ExecuteTool(_ *core.ToolContext, _ string, _ string) (any, error) {
	return "mock tool result", nil
}
func (m *MockFlowAgent) TransferToAgent(_ *core.RunContext, _ string) error {
	return nil
}

func TestInstructionsProcessor_Name(t *testing.T) {
	if NewInstructionsProcessor().Name() != "instructions" {
		t.Errorf("expected name 'instructions'")
	}
}
