package flow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mock types for processors
type mockReqProc struct {
	name string
	fn   func(ctx context.Context, reqCtx core.RequestContext, req *core.ModelRequest, agent Agent) error
}

func (m *mockReqProc) Name() string { return m.name }

func (m *mockReqProc) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent Agent,
) error {
	return m.fn(ctx, reqCtx, req, agent)
}

type mockRespProc struct {
	name string
	fn   func(ctx context.Context, reqCtx core.RequestContext, resp *core.ModelResponse, agent Agent) error
}

func (m *mockRespProc) Name() string { return m.name }

func (m *mockRespProc) ProcessResponse(
	ctx context.Context,
	reqCtx core.RequestContext,
	resp *core.ModelResponse,
	agent Agent,
) error {
	return m.fn(ctx, reqCtx, resp, agent)
}

// model that emits a final assistant response with a single function call on first Generate,
// then emits a plain text response on the next call.
type onceToolCallModel struct{ called int }

func (m *onceToolCallModel) Generate(
	ctx context.Context,
	_ *core.ModelRequest,
) (<-chan *core.ModelResponse, <-chan error) {
	rc := make(chan *core.ModelResponse, 1)
	ec := make(chan error, 1)
	go func() {
		defer close(rc)
		defer close(ec)
		if m.called == 0 {
			rc <- &core.ModelResponse{Partial: false, Parts: []core.Part{
				core.NewPartFromFunctionCall("call1", "echo", "{\"x\":1}"),
			}}
		} else {
			rc <- &core.ModelResponse{Partial: false, Parts: []core.Part{core.NewPartFromText("done")}}
		}
		m.called++
	}()
	return rc, ec
}
func (m *onceToolCallModel) Info() core.ModelInfo {
	return core.ModelInfo{Name: "m", Provider: "test", SupportsTools: true}
}

// model that emits a single plain text response
type textModel struct{ text string }

func (m *textModel) Generate(ctx context.Context, _ *core.ModelRequest) (<-chan *core.ModelResponse, <-chan error) {
	rc := make(chan *core.ModelResponse, 1)
	ec := make(chan error, 1)
	go func() {
		defer close(rc)
		defer close(ec)
		rc <- &core.ModelResponse{Partial: false, Parts: []core.Part{core.NewPartFromText(m.text)}}
	}()
	return rc, ec
}
func (m *textModel) Info() core.ModelInfo { return core.ModelInfo{Name: "m", Provider: "test"} }

// fake executor that emits a simple function response event per call, with optional transfer.
type fakeExec struct{ transferTo core.Opt[string] }

func (e *fakeExec) Execute(
	ctx context.Context,
	reqCtx core.RequestContext,
	agent Agent,
	toolRegistry map[string]core.Tool,
	fnCalls []*core.FunctionCall,
	emit func(*core.Event) error,
) error {
	for _, c := range fnCalls {
		ev := core.NewFunctionResponseEvent(reqCtx.RunID(), agent.Name(), c.ID, c.Name, map[string]any{"ok": true})
		ev.Actions.TransferToAgent = e.transferTo
		if err := emit(ev); err != nil {
			return err
		}
	}

	return nil
}

func TestBaseFlow_Simple_NoTools(t *testing.T) {
	// Use text-only model, ensure processors run and at least one assistant event is written
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &textModel{text: "hello"}
	agent.ToolsMap = map[string]core.Tool{}
	agent.FunctionCallingEnabled = true
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent)
	// Minimal request processor to add user content (so model has input)
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent Agent,
		) error {
			req.Messages = []*core.Message{{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("hi")}}}
			return nil
		},
	})

	q := &testutil.CollectingWriter{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	reqCtx := newTestRunContext()
	err := f.Execute(ctx, reqCtx, q)
	require.NoError(t, err)
	require.NotEmpty(t, q.Events)

	// last event should be assistant, non-partial, no tool calls
	last := q.Events[len(q.Events)-1]
	assert.Equal(t, core.RoleAssistant, last.Role())
	assert.False(t, last.IsPartial())
	assert.False(t, last.HasFunctionCalls())
}

func TestBaseFlow_FunctionCall_LoopsOnce(t *testing.T) {
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &onceToolCallModel{}
	agent.ToolsMap = map[string]core.Tool{}
	agent.FunctionCallingEnabled = true
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent)
	// Provide a trivial request
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent Agent,
		) error {
			req.Messages = []*core.Message{{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("hi")}}}
			return nil
		},
	})

	// Inject fake executor that returns a tool response (no transfer), so flow loops back to build -> call
	f.functionExecutor = &fakeExec{}

	q := &testutil.CollectingWriter{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := f.Execute(ctx, newTestRunContext(), q)
	require.NoError(t, err)

	// Expect sequence: assistant (with fn call), tool response, assistant (final)
	require.GreaterOrEqual(t, len(q.Events), 3)
	first := q.Events[0]
	second := q.Events[1]
	last := q.Events[len(q.Events)-1]

	assert.True(t, first.HasFunctionCalls())
	assert.True(t, second.HasFunctionResponses())
	assert.False(t, last.HasFunctionCalls())
}

func TestBaseFlow_NoLoopOnTransferOrEscalate(t *testing.T) {
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &onceToolCallModel{}
	agent.ToolsMap = map[string]core.Tool{}
	agent.FunctionCallingEnabled = true
	agent.ResolveInstructionsFunc = func(
		ctx context.Context,
		roCtx core.ReadonlyContext,
	) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent)
	f.AddRequestProcessor(&mockReqProc{
		name: "fail",
		fn: func(
			context.Context,
			core.RequestContext,
			*core.ModelRequest,
			Agent,
		) error {
			return errors.New("boom")
		},
	})

	q := &testutil.CollectingWriter{}
	err := f.Execute(context.Background(), newTestRunContext(), q)
	require.Error(t, err)
}

func TestBaseFlow_TransferMissingAgentError(t *testing.T) {
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &onceToolCallModel{}
	agent.ToolsMap = map[string]core.Tool{}
	agent.FunctionCallingEnabled = true
	agent.ResolveInstructionsFunc = func(
		ctx context.Context,
		roCtx core.ReadonlyContext,
	) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent)
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent Agent,
		) error {
			req.Messages = []*core.Message{{
				Role:  core.RoleUser,
				Parts: []core.Part{core.NewPartFromText("hi")},
			}}
			return nil
		},
	})
	// executor signals transfer to non-existent agent
	f.functionExecutor = &fakeExec{transferTo: core.String("does-not-exist")}
	q := &testutil.CollectingWriter{}
	err := f.Execute(context.Background(), newTestRunContext(), q)
	require.Error(t, err)
}

func TestBaseFlow_TransferToAgent_RunsTargetAgent(t *testing.T) {
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &onceToolCallModel{}
	agent.ToolsMap = map[string]core.Tool{}
	agent.FunctionCallingEnabled = true
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	// Provide a child agent in the hierarchy to receive the transfer and track Run calls
	child := testutil.NewMockAgent("child")
	agent.SubAgentsList = []core.Agent{child}

	f := NewBaseFlow(agent)
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent Agent,
		) error {
			req.Messages = []*core.Message{{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("hi")}}}
			return nil
		},
	})

	// Function executor will request a transfer to the child
	f.functionExecutor = &fakeExec{transferTo: core.String("child")}

	q := &testutil.CollectingWriter{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := f.Execute(ctx, newTestRunContext(), q)
	require.NoError(t, err)

	// Assert that child.Run was called exactly once as a result of the transfer
	assert.Equal(t, 1, child.RunCount)

	// We should have at least assistant with function call and tool response before the transfer
	require.GreaterOrEqual(t, len(q.Events), 2)
	assert.True(t, q.Events[0].HasFunctionCalls())
	assert.True(t, q.Events[1].HasFunctionResponses())
}

func TestBaseFlow_ResponseProcessorError(t *testing.T) {
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &textModel{text: "hello"}
	agent.ToolsMap = map[string]core.Tool{}
	agent.FunctionCallingEnabled = true
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent)
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent Agent,
		) error {
			req.Messages = []*core.Message{{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("hi")}}}
			return nil
		},
	})
	f.AddResponseProcessor(&mockRespProc{
		name: "fail",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			res *core.ModelResponse,
			agent Agent,
		) error {
			return assert.AnError
		},
	})

	q := &testutil.CollectingWriter{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := f.Execute(ctx, newTestRunContext(), q)
	require.Error(t, err)
}
