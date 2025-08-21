package flow

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/session"
	"github.com/hupe1980/agentmesh/tool"
)

type tiMockAgent struct {
	name      string
	transfer  bool
	subAgents []FlowAgent
	tools     map[string]tool.Tool
}

func (a *tiMockAgent) GetName() string                                            { return a.name }
func (a *tiMockAgent) GetLLM() model.Model                                        { return nil }
func (a *tiMockAgent) ResolveInstructions(*core.RunContext) (string, error)       { return "", nil }
func (a *tiMockAgent) GetTools() map[string]tool.Tool                             { return a.tools }
func (a *tiMockAgent) GetSubAgents() []FlowAgent                                  { return a.subAgents }
func (a *tiMockAgent) IsFunctionCallingEnabled() bool                             { return true }
func (a *tiMockAgent) IsStreamingEnabled() bool                                   { return false }
func (a *tiMockAgent) IsTransferEnabled() bool                                    { return a.transfer }
func (a *tiMockAgent) GetOutputKey() string                                       { return "" }
func (a *tiMockAgent) MaxHistoryMessages() int                                    { return 10 }
func (a *tiMockAgent) ExecuteTool(*core.ToolContext, string, string) (any, error) { return nil, nil }
func (a *tiMockAgent) TransferToAgent(*core.RunContext, string) error             { return nil }

func TestTransferToolInjector_Injection(t *testing.T) {
	agent := &tiMockAgent{name: "root", transfer: true, subAgents: []FlowAgent{&tiMockAgent{name: "child"}}, tools: map[string]tool.Tool{}}
	inj := NewTransferToolInjector()
	sessStore := session.NewInMemoryStore()
	sess, _ := sessStore.Create("sess")
	userContent := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "hi"}}}
	runCtx := core.NewRunContext(context.Background(), "sess", "run", core.AgentInfo{Name: "root", Type: "test"}, userContent, make(chan core.Event, 1), nil, sess, sessStore, nil, nil, logging.NoOpLogger{})
	req := &model.Request{}
	if err := inj.ProcessRequest(runCtx, req, agent); err != nil {
		t.Fatalf("inject error: %v", err)
	}
	found := false
	for _, td := range req.Tools {
		if td.Function.Name == "transfer_to_agent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected transfer_to_agent tool definition injected")
	}
	// second call should not duplicate
	_ = inj.ProcessRequest(runCtx, req, agent)
	count := 0
	for _, td := range req.Tools {
		if td.Function.Name == "transfer_to_agent" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected single definition, got %d", count)
	}
}
