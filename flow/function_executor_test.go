package flow

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/model"
	"github.com/hupe1980/agentmesh/session"
	"github.com/hupe1980/agentmesh/tool"
)

type teMockTool struct {
	name        string
	delay       time.Duration
	result      any
	err         error
	panicMsg    any
	actionState map[string]any
	transferTo  string
}

func (mt *teMockTool) Name() string               { return mt.name }
func (mt *teMockTool) Description() string        { return "mock tool" }
func (mt *teMockTool) Parameters() map[string]any { return map[string]any{} }
func (mt *teMockTool) Call(tc *core.ToolContext, _ map[string]any) (any, error) {
	if mt.delay > 0 {
		select {
		case <-time.After(mt.delay):
		case <-tc.Context().Done():
			return nil, tc.Context().Err()
		}
	}
	if mt.panicMsg != nil {
		panic(mt.panicMsg)
	}
	for k, v := range mt.actionState {
		// ensure actions get applied
		cval := v
		tc.SetState(k, cval)
	}
	if mt.transferTo != "" {
		// signal transfer
		tc.TransferToAgent(mt.transferTo)
	}
	return mt.result, mt.err
}

type teAgent struct {
	name  string
	tools map[string]tool.Tool
}

func (a *teAgent) GetName() string                                      { return a.name }
func (a *teAgent) GetLLM() model.Model                                  { return nil }
func (a *teAgent) ResolveInstructions(*core.RunContext) (string, error) { return "", nil }
func (a *teAgent) GetTools() map[string]tool.Tool                       { return a.tools }
func (a *teAgent) GetSubAgents() []FlowAgent                            { return nil }
func (a *teAgent) IsFunctionCallingEnabled() bool                       { return true }
func (a *teAgent) IsStreamingEnabled() bool                             { return false }
func (a *teAgent) IsTransferEnabled() bool                              { return true }
func (a *teAgent) GetOutputKey() string                                 { return "" }
func (a *teAgent) MaxHistoryMessages() int                              { return 50 }
func (a *teAgent) TransferToAgent(*core.RunContext, string) error       { return nil }

// helper to make run context
func newTERunContext(t *testing.T) *core.RunContext {
	ctx := context.Background()
	eventChan := make(chan core.Event, 100)
	sessSvc := session.NewInMemoryStore()
	sess, _ := sessSvc.Create("sess")
	userContent := core.Content{Role: "user", Parts: []core.Part{core.TextPart{Text: "msg"}}}

	return core.NewRunContext(ctx, "sess", "run", core.AgentInfo{Name: "agent", Type: "test"}, userContent, eventChan, nil, sess, sessSvc, nil, nil, logging.NoOpLogger{})
}

func TestFunctionExecutor_Single(t *testing.T) {
	a := &teAgent{name: "A", tools: map[string]tool.Tool{
		"one": &teMockTool{name: "one", result: 42},
	}}
	te := NewParallelFunctionExecutor(FunctionExecutorConfig{MaxParallel: 4, PreserveOrder: true})
	rc := newTERunContext(t)
	fnCalls := []core.FunctionCall{{ID: "1", Name: "one", Arguments: "{}"}}
	events := make([]core.Event, 0)
	emit := func(ev core.Event) error { events = append(events, ev); return nil }
	te.Execute(rc, a, a.tools, fnCalls, emit)
	if len(events) != 1 {
		t.Fatalf("expected 1 event got %d", len(events))
	}
}

func TestFunctionExecutor_ParallelUnordered(t *testing.T) {
	a := &teAgent{name: "A", tools: map[string]tool.Tool{
		"slow": &teMockTool{name: "slow", delay: 60 * time.Millisecond, result: "s"},
		"fast": &teMockTool{name: "fast", delay: 5 * time.Millisecond, result: "f"},
	}}
	te := NewParallelFunctionExecutor(FunctionExecutorConfig{MaxParallel: 2, PreserveOrder: false})
	rc := newTERunContext(t)
	fnCalls := []core.FunctionCall{{ID: "1", Name: "slow", Arguments: "{}"}, {ID: "2", Name: "fast", Arguments: "{}"}}
	var order []string
	emit := func(ev core.Event) error { order = append(order, ev.GetFunctionResponses()[0].Name); return nil }
	start := time.Now()
	te.Execute(rc, a, a.tools, fnCalls, emit)
	if len(order) != 2 {
		t.Fatalf("want 2 events got %d", len(order))
	}
	if order[0] != "fast" {
		t.Fatalf("expected fast first got %s", order[0])
	}
	elapsed := time.Since(start)
	if elapsed > 90*time.Millisecond {
		t.Fatalf("expected parallel speedup, elapsed=%v", elapsed)
	}
}

func TestFunctionExecutor_PreserveOrder(t *testing.T) {
	a := &teAgent{name: "A", tools: map[string]tool.Tool{
		"t1": &teMockTool{name: "t1", delay: 30 * time.Millisecond, result: 1},
		"t2": &teMockTool{name: "t2", delay: 5 * time.Millisecond, result: 2},
	}}
	te := NewParallelFunctionExecutor(FunctionExecutorConfig{MaxParallel: 2, PreserveOrder: true})
	rc := newTERunContext(t)
	fnCalls := []core.FunctionCall{{ID: "1", Name: "t1", Arguments: "{}"}, {ID: "2", Name: "t2", Arguments: "{}"}}
	var order []string
	emit := func(ev core.Event) error { order = append(order, ev.GetFunctionResponses()[0].Name); return nil }
	te.Execute(rc, a, a.tools, fnCalls, emit)
	if order[0] != "t1" || order[1] != "t2" {
		t.Fatalf("order not preserved: %v", order)
	}
}

func TestFunctionExecutor_ErrorIsolation(t *testing.T) {
	a := &teAgent{name: "A", tools: map[string]tool.Tool{
		"ok":  &teMockTool{name: "ok", result: "fine"},
		"bad": &teMockTool{name: "bad", err: errors.New("boom")},
	}}
	te := NewParallelFunctionExecutor(FunctionExecutorConfig{MaxParallel: 2, PreserveOrder: false})
	rc := newTERunContext(t)
	fnCalls := []core.FunctionCall{{ID: "1", Name: "ok", Arguments: "{}"}, {ID: "2", Name: "bad", Arguments: "{}"}}
	var errs int32
	emit := func(ev core.Event) error {
		if ev.GetFunctionResponses()[0].Error != "" {
			atomic.AddInt32(&errs, 1)
		}
		return nil
	}
	te.Execute(rc, a, a.tools, fnCalls, emit)
	if atomic.LoadInt32(&errs) != 1 {
		t.Fatalf("expected 1 error event got %d", errs)
	}
}

func TestFunctionExecutor_PanicRecovery(t *testing.T) {
	a := &teAgent{name: "A", tools: map[string]tool.Tool{
		"panic": &teMockTool{name: "panic", panicMsg: "boom"},
	}}
	te := NewParallelFunctionExecutor(FunctionExecutorConfig{})
	rc := newTERunContext(t)
	fnCalls := []core.FunctionCall{{ID: "1", Name: "panic", Arguments: "{}"}}
	var got bool
	emit := func(ev core.Event) error {
		if ev.GetFunctionResponses()[0].Error != "" {
			got = true
		}
		return nil
	}
	te.Execute(rc, a, a.tools, fnCalls, emit)
	if !got {
		t.Fatalf("expected panic converted to error")
	}
}

func TestFunctionExecutor_ActionsApplied(t *testing.T) {
	a := &teAgent{name: "A", tools: map[string]tool.Tool{
		"act": &teMockTool{name: "act", actionState: map[string]any{"k": "v"}, transferTo: "next"},
	}}
	te := NewParallelFunctionExecutor(FunctionExecutorConfig{})
	rc := newTERunContext(t)
	fnCalls := []core.FunctionCall{{ID: "1", Name: "act", Arguments: "{}"}}
	var evs []core.Event
	emit := func(ev core.Event) error { evs = append(evs, ev); return nil }
	te.Execute(rc, a, a.tools, fnCalls, emit)
	if len(evs) != 1 {
		t.Fatalf("expected 1 event got %d", len(evs))
	}
	if evs[0].Actions.StateDelta["k"] != "v" {
		t.Fatalf("state delta missing")
	}
	if evs[0].Actions.TransferToAgent == nil || *evs[0].Actions.TransferToAgent != "next" {
		t.Fatalf("transfer action missing")
	}
}
