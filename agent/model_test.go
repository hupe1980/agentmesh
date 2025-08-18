package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockModelImpl for testing LLM functionality
type MockModelImpl struct{ mock.Mock }

func (m *MockModelImpl) Generate(ctx context.Context, req model.Request) (<-chan model.Response, <-chan error) {
	args := m.Called(ctx, req)
	// Allow tests to provide channels or create a simple default
	if ch, ok := args.Get(0).(<-chan model.Response); ok {
		return ch, args.Get(1).(<-chan error)
	}

	respCh := make(chan model.Response, 1)
	errCh := make(chan error, 1)

	// If a *model.Response was supplied, adapt its first choice
	if cr, ok := args.Get(0).(*model.Response); ok && len(cr.Content.Parts) > 0 {
		respCh <- model.Response{
			Partial:      false,
			Content:      core.Content{Role: "assistant", Parts: cr.Content.Parts},
			FinishReason: "stop",
		}
	} else {
		respCh <- model.Response{
			Partial:      false,
			Content:      core.Content{Role: "assistant", Parts: []core.Part{core.TextPart{Text: "test"}}},
			FinishReason: "stop",
		}
	}

	close(respCh)
	close(errCh)

	return respCh, errCh
}

func (m *MockModelImpl) Info() model.Info {
	args := m.Called()
	return args.Get(0).(model.Info)
}

// LLM Agent Test Cases
func TestModelAgent_NewAgent(t *testing.T) {
	mockLLM := &MockModelImpl{}
	agent := NewModelAgent("Test Agent", mockLLM)

	assert.NotNil(t, agent)
	assert.Equal(t, mockLLM, agent.llm)
	assert.NotNil(t, agent.tools)
	assert.Empty(t, agent.tools)
	assert.True(t, agent.enableStreaming)
	assert.True(t, agent.enableFunctionCalling)
}
