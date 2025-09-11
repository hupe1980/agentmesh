package processor

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

func TestInstructionsProcessor_ProcessRequest_AppendsResolved(t *testing.T) {
	p := NewInstructionsProcessor()
	req := &core.ModelRequest{}

	sess := core.NewSession("app", "user", "sess")
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:   "run1",
		Agent:   core.AgentInfo{Name: "agent", Type: "flow"},
		Session: sess,
	})

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

	sess := core.NewSession("app", "user", "sess")
	sess.SetState("user", "Alice")
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:   "run1",
		Agent:   core.AgentInfo{Name: "agent", Type: "flow"},
		Session: sess,
	})

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

	sess := core.NewSession("app", "user", "sess")
	// Ensure snapshot is non-empty so templating is attempted, but omit the referenced key
	sess.SetState("other", 1)
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:   "run1",
		Agent:   core.AgentInfo{Name: "agent", Type: "flow"},
		Session: sess,
	})

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

	sess := core.NewSession("app", "user", "sess")
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:   "run1",
		Agent:   core.AgentInfo{Name: "agent", Type: "flow"},
		Session: sess,
	})

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

	sess := core.NewSession("app", "user", "sess")
	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:   "run1",
		Agent:   core.AgentInfo{Name: "agent", Type: "flow"},
		Session: sess,
	})

	agent := testutil.NewMockAgent("a")
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "", errors.New("boom")
	}

	err := p.ProcessRequest(context.Background(), reqCtx, req, agent)
	assert.Error(t, err)
}
