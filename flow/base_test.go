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
	fn   func(ctx context.Context, reqCtx core.RequestContext, req *core.ModelRequest, agent core.FlowAgent) error
}

func (m *mockReqProc) Name() string { return m.name }

func (m *mockReqProc) ProcessRequest(
	ctx context.Context,
	reqCtx core.RequestContext,
	req *core.ModelRequest,
	agent core.FlowAgent,
) error {
	return m.fn(ctx, reqCtx, req, agent)
}

type mockRespProc struct {
	name string
	fn   func(ctx context.Context, reqCtx core.RequestContext, resp *core.ModelResponse, agent core.FlowAgent) error
}

func (m *mockRespProc) Name() string { return m.name }

func (m *mockRespProc) ProcessResponse(
	ctx context.Context,
	reqCtx core.RequestContext,
	resp *core.ModelResponse,
	agent core.FlowAgent,
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

func (m *onceToolCallModel) Capabilities() core.ModelCapabilities {
	return core.ModelCapabilities{}
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

func (m *textModel) Capabilities() core.ModelCapabilities {
	return core.ModelCapabilities{}
}

func TestBaseFlow_Simple_NoTools(t *testing.T) {
	// Use text-only model, ensure processors run and at least one assistant event is written
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &textModel{text: "hello"}
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent, &Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
		ToolExecutor:  testutil.NewToolExecutorMock(),
	})
	// Minimal request processor to add user content (so model has input)
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent core.FlowAgent,
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
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent, &Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
		ToolExecutor:  testutil.NewToolExecutorMock(),
	})
	// Provide a trivial request
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent core.FlowAgent,
		) error {
			req.Messages = []*core.Message{{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("hi")}}}
			return nil
		},
	})

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
	agent.ResolveInstructionsFunc = func(
		ctx context.Context,
		roCtx core.ReadonlyContext,
	) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent, &Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
		ToolExecutor:  testutil.NewToolExecutorMock(),
	})

	f.AddRequestProcessor(&mockReqProc{
		name: "fail",
		fn: func(
			context.Context,
			core.RequestContext,
			*core.ModelRequest,
			core.FlowAgent,
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
	agent.ResolveInstructionsFunc = func(
		ctx context.Context,
		roCtx core.ReadonlyContext,
	) (string, error) {
		return "inst", nil
	}

	toolExexutor := testutil.NewToolExecutorMock()
	toolExexutor.TransferTo = core.String("non-existent")

	f := NewBaseFlow(agent, &Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
		ToolExecutor:  toolExexutor,
	})

	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent core.FlowAgent,
		) error {
			req.Messages = []*core.Message{{
				Role:  core.RoleUser,
				Parts: []core.Part{core.NewPartFromText("hi")},
			}}
			return nil
		},
	})

	q := &testutil.CollectingWriter{}
	err := f.Execute(context.Background(), newTestRunContext(), q)
	require.Error(t, err)
}

func TestBaseFlow_TransferToAgent_RunsTargetAgent(t *testing.T) {
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &onceToolCallModel{}
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	// Provide a child agent in the hierarchy to receive the transfer and track Run calls
	child := testutil.NewMockAgent("child")
	agent.SubAgentsList = []core.Agent{child}

	toolExexutor := testutil.NewToolExecutorMock()
	toolExexutor.TransferTo = core.String("child")

	f := NewBaseFlow(agent, &Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
		ToolExecutor:  toolExexutor,
	})
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent core.FlowAgent,
		) error {
			req.Messages = []*core.Message{{Role: core.RoleUser, Parts: []core.Part{core.NewPartFromText("hi")}}}
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	q := &testutil.CollectingWriter{}
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
	agent.ResolveInstructionsFunc = func(ctx context.Context, _ core.ReadonlyContext) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent, &Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
		ToolExecutor:  testutil.NewToolExecutorMock(),
	})

	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			reqCtx core.RequestContext,
			req *core.ModelRequest,
			agent core.FlowAgent,
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
			agent core.FlowAgent,
		) error {
			return assert.AnError
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	q := &testutil.CollectingWriter{}
	err := f.Execute(ctx, newTestRunContext(), q)
	require.Error(t, err)
}
