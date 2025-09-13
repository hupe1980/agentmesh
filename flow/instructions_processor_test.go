package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstructionsProcessor_Name(t *testing.T) {
	assert.Equal(t, "instructions", NewInstructionsProcessor().Name())
}

func buildInstrReqCtx(runID string, mutate func(sess *core.Session)) core.RequestContext {
	sess := core.NewSession("app", "user", "sess")
	if mutate != nil {
		mutate(sess)
	}
	ag := testutil.NewMockAgent("agent")
	return testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = ag
		p.RunID = runID
		p.Session = sess
	})
}

func TestInstructionsProcessor_ProcessRequest_AppendsResolved(t *testing.T) {
	p := NewInstructionsProcessor()
	req := &core.ModelRequest{}
	reqCtx := buildInstrReqCtx("run1", nil)
	agent := testutil.NewMockAgent("a")
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "You are a helpful assistant.", nil
	}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)
	assert.Equal(t, "You are a helpful assistant.", req.Instructions)
}

func TestInstructionsProcessor_ProcessRequest_RendersTemplateWithState(t *testing.T) {
	p := NewInstructionsProcessor()
	req := &core.ModelRequest{}
	reqCtx := buildInstrReqCtx("run1", func(sess *core.Session) { sess.SetState("user", "Alice") })
	agent := testutil.NewMockAgent("a")
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "Hello {{.user}}!", nil
	}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice!", req.Instructions)
}

func TestInstructionsProcessor_ProcessRequest_RendersDefaultWhenMissingKey(t *testing.T) {
	p := NewInstructionsProcessor()
	req := &core.ModelRequest{}
	reqCtx := buildInstrReqCtx("run1", func(sess *core.Session) { sess.SetState("other", 1) })
	agent := testutil.NewMockAgent("a")
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "Hello {{default \"World\" .user}}!", nil
	}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)
	assert.Equal(t, "Hello World!", req.Instructions)
}

func TestInstructionsProcessor_ProcessRequest_AppendsToExisting(t *testing.T) {
	p := NewInstructionsProcessor()
	req := &core.ModelRequest{Instructions: "system base"}
	reqCtx := buildInstrReqCtx("run1", nil)
	agent := testutil.NewMockAgent("a")
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "policy: be concise", nil
	}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	require.NoError(t, err)
	assert.Equal(t, "system base\n\npolicy: be concise", req.Instructions)
}

func TestInstructionsProcessor_ProcessRequest_ErrorOnResolve(t *testing.T) {
	p := NewInstructionsProcessor()
	req := &core.ModelRequest{}
	reqCtx := buildInstrReqCtx("run1", nil)
	agent := testutil.NewMockAgent("a")
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "", errors.New("boom")
	}
	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	assert.Error(t, err)
}
