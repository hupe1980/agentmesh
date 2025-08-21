package flow

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/session"
	"github.com/hupe1980/agentmesh/tool"
)

// mergeMockModel emits one assistant response containing two function calls then closes.
type mergeMockModel struct{}

func (m *mergeMockModel) Generate(ctx context.Context, req model.Request) (<-chan model.Response, <-chan error) {
	respCh := make(chan model.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		defer close(respCh)
		defer close(errCh)
		// Build content with two function calls
		parts := []core.Part{
			core.FunctionCallPart{FunctionCall: core.FunctionCall{ID: "fc1", Name: "t1", Arguments: "{}"}},
			core.FunctionCallPart{FunctionCall: core.FunctionCall{ID: "fc2", Name: "t2", Arguments: "{}"}},
		}
		respCh <- model.Response{Partial: false, Content: core.Content{Role: "assistant", Parts: parts}, FinishReason: "tool_calls"}
	}()
	return respCh, errCh
}

func (m *mergeMockModel) Info() model.Info {
	return model.Info{Name: "merge-mock", Provider: "mock", SupportsTools: true}
}

type mergeAgent struct {
	*teAgent
	llm model.Model
}

func (a *mergeAgent) GetLLM() model.Model { return a.llm }

// newMergeRunContext replicates helper from function_executor_test but exported here locally.
func newMergeRunContext() *core.RunContext {
	ctx := context.Background()
	eventChan := make(chan core.Event, 100)
	sessSvc := session.NewInMemoryStore()
	sess, _ := sessSvc.Create("sess")
	userContent := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "merge"}}}
	return core.NewRunContext(ctx, "sess", "run", core.AgentInfo{Name: "agent", Type: "test"}, userContent, 100, eventChan, nil, sess, sessSvc, nil, nil, logging.NoOpLogger{})
}

func TestBaseFlow_MergeFunctionResponses(t *testing.T) {
	tools := map[string]tool.Tool{
		"t1": &teMockTool{name: "t1", delay: 20 * time.Millisecond, result: "r1", actionState: map[string]any{"a": 1}},
		"t2": &teMockTool{name: "t2", delay: 10 * time.Millisecond, result: "r2", transferTo: "next"},
	}
	agent := &mergeAgent{teAgent: &teAgent{name: "A", tools: tools}, llm: &mergeMockModel{}}
	bf := NewBaseFlow(agent)
	rc := newMergeRunContext()

	evCh, errCh, err := bf.Execute(rc)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var toolEvents []core.Event
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev, ok := <-evCh:
			if !ok {
				break loop
			}
			if len(ev.GetFunctionResponses()) > 0 {
				toolEvents = append(toolEvents, ev)
			}
		case e, ok := <-errCh:
			if ok && e != nil {
				t.Fatalf("error: %v", e)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for events")
		}
		if len(toolEvents) == 1 { // merged event arrived
			// Wait for channel close
			for range evCh {
			}
			break
		}
	}

	if len(toolEvents) != 1 {
		t.Fatalf("expected 1 merged tool event, got %d", len(toolEvents))
	}
	merged := toolEvents[0]
	frs := merged.GetFunctionResponses()
	if len(frs) != 2 {
		t.Fatalf("expected 2 function responses in merged event, got %d", len(frs))
	}
	// verify order preserved (t1 then t2 from function call list)
	if frs[0].Name != "t1" || frs[1].Name != "t2" {
		t.Fatalf("unexpected order: %+v", frs)
	}
	if merged.Actions.StateDelta["a"].(int) != 1 {
		t.Fatalf("merged state delta missing")
	}
	if merged.Actions.TransferToAgent == nil || *merged.Actions.TransferToAgent != "next" {
		t.Fatalf("transfer not merged")
	}
}
