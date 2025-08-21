package flow

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/session"
	"github.com/hupe1980/agentmesh/tool"
)

// MockModel is a lightweight in‑memory Model useful for tests & examples.
type MockModel struct {
	info      model.Info
	responses map[string]string
}

// NewMockModel constructs a MockModel with basic tool support enabled.
func NewMockModel(name, provider string) *MockModel {
	return &MockModel{
		info: model.Info{
			Name:          name,
			Provider:      provider,
			SupportsTools: true,
		},
		responses: make(map[string]string),
	}
}

// AddResponse registers a deterministic canned completion for an input prompt.
func (m *MockModel) AddResponse(prompt, response string) {
	m.responses[prompt] = response
}

// Generate implements Model; emits optional streaming char chunks then final response.
func (m *MockModel) Generate(ctx context.Context, req model.Request) (<-chan model.Response, <-chan error) {
	respCh := make(chan model.Response, 16)
	errCh := make(chan error, 1)

	go func() {
		defer close(respCh)
		defer close(errCh)
		if len(req.Contents) == 0 {
			errCh <- fmt.Errorf("no contents provided")
			return
		}
		// Extract last content text
		last := req.Contents[len(req.Contents)-1]
		var inputText string
		for _, p := range last.Parts {
			if tp, ok := p.(core.TextPart); ok {
				inputText += tp.Text
			}
		}
		full := m.responses[inputText]
		if full == "" {
			full = fmt.Sprintf("Mock response to: %s", inputText)
		}
		if req.Stream {
			for _, r := range full { // Emit character chunks as partials
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case respCh <- model.Response{
					Partial: true,
					Content: core.Content{
						Role:  "assistant",
						Parts: []core.Part{core.TextPart{Text: string(r)}},
					},
				}:
				}
			}
		}
		respCh <- model.Response{ // Final response
			Partial: false,
			Content: core.Content{
				Role:  "assistant",
				Parts: []core.Part{core.TextPart{Text: full}},
			},
			FinishReason: "stop",
		}
	}()
	return respCh, errCh
}

// Info implements Model interface
func (m *MockModel) Info() model.Info { return m.info }

type MockMemoryService struct{}

func (m *MockMemoryService) Get(_ string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (m *MockMemoryService) Put(_ string, _ map[string]any) error { return nil }

func (m *MockMemoryService) Search(_ string, _ string, _ int) ([]core.SearchResult, error) {
	return []core.SearchResult{}, nil
}

func (m *MockMemoryService) Store(_ string, _ string, _ map[string]any) error {
	return nil
}

func (m *MockMemoryService) Delete(_ string, _ string) error { return nil }

func newTestRunContext() *core.RunContext {
	ctx := context.Background()
	eventChan := make(chan core.Event, 10)
	sessSvc := session.NewInMemoryStore()
	sess, _ := sessSvc.Create("test-session")

	userContent := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "test message"}}}
	runCtx := core.NewRunContext(
		ctx,
		"test-session",
		"test-invocation",
		core.AgentInfo{Name: "TestAgent", Type: "flow-test"},
		userContent,
		100,
		eventChan,
		nil,
		sess,
		sessSvc,
		nil,
		&MockMemoryService{},
		logging.NoOpLogger{},
	)

	return runCtx
}

type mockFlowAgent struct {
	name string
	llm  model.Model
}

func (m *mockFlowAgent) GetName() string { return m.name }

func (m *mockFlowAgent) GetLLM() model.Model { return m.llm }

func (m *mockFlowAgent) ResolveInstructions(_ *core.RunContext) (string, error) {
	return "You are a test assistant.", nil
}

func (m *mockFlowAgent) GetTools() map[string]tool.Tool { return map[string]tool.Tool{} }

func (m *mockFlowAgent) GetSubAgents() []FlowAgent { return []FlowAgent{} }

func (m *mockFlowAgent) IsFunctionCallingEnabled() bool { return false }

func (m *mockFlowAgent) IsStreamingEnabled() bool { return false }

func (m *mockFlowAgent) IsTransferEnabled() bool { return false }

func (m *mockFlowAgent) GetOutputKey() string { return "" }

func (m *mockFlowAgent) MaxHistoryMessages() int { return 10 }

func (m *mockFlowAgent) ExecuteTool(_ *core.ToolContext, _ string, _ string) (any, error) {
	return nil, nil
}

func (m *mockFlowAgent) TransferToAgent(_ *core.RunContext, _ string) error {
	return nil
}

func TestSingleAgentFlow(t *testing.T) {
	mockModel := NewMockModel("test-model", "mock")
	mockModel.AddResponse("test message", "Hello! This is a test response.")
	agent := &mockFlowAgent{name: "test-agent", llm: mockModel}
	runCtx := newTestRunContext()
	f := NewSingleAgentFlow(agent)
	eventChan, errChan, err := f.Execute(runCtx)
	if err != nil {
		t.Fatalf("Flow execution failed: %v", err)
	}
	var events []core.Event
	for ev := range eventChan {
		events = append(events, ev)
	}
	// check async error
	select {
	case e, ok := <-errChan:
		if ok && e != nil {
			t.Fatalf("Flow async error: %v", e)
		}
	default:
	}
	if len(events) == 0 {
		t.Error("Expected at least one event from flow execution")
	}
}
