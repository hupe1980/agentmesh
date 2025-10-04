package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalInstructions_NoInstructionsDefined(t *testing.T) {
	pl := NewGlobalInstructions(nil)

	req := &core.ModelRequest{Instructions: "base"}
	reqCtx := testutil.NewTestRequestContext()
	cbCtx := core.NewCallbackContext(reqCtx)

	res, err := pl.BeforeModel(context.Background(), cbCtx, req)
	require.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, "base", req.Instructions)
}

func TestGlobalInstructions_AppendsStaticInstructions(t *testing.T) {
	inst := core.NewInstructionsFromText("extra guidance")
	pl := NewGlobalInstructions(&inst)

	req := &core.ModelRequest{Instructions: "base"}
	reqCtx := testutil.NewTestRequestContext()
	cbCtx := core.NewCallbackContext(reqCtx)

	res, err := pl.BeforeModel(context.Background(), cbCtx, req)
	require.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, "extra guidance\n\nbase", req.Instructions)
}

func TestGlobalInstructions_RendersTemplateWithState(t *testing.T) {
	inst := core.NewInstructionsFromText("Hello {{ upper .team }}")
	pl := NewGlobalInstructions(&inst)

	session := core.NewSession("app", "user", "sess")
	session.SetState("team", "AgentMesh")

	reqCtx := testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Session = session
		p.SessionStore = &testutil.SessionStoreMock{
			GetOrCreateFunc: func(context.Context, string, string, string) (*core.Session, error) {
				return session, nil
			},
		}
	})
	cbCtx := core.NewCallbackContext(reqCtx)

	req := &core.ModelRequest{Instructions: "base"}

	res, err := pl.BeforeModel(context.Background(), cbCtx, req)
	require.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, "Hello AGENTMESH\n\nbase", req.Instructions)
}

func TestGlobalInstructions_PropagatesResolveError(t *testing.T) {
	expectedErr := errors.New("boom")
	inst := core.NewInstructionsFromFunc(func(context.Context, core.ReadonlyContext) (string, error) {
		return "", expectedErr
	})
	pl := NewGlobalInstructions(&inst)

	req := &core.ModelRequest{Instructions: "base"}
	reqCtx := testutil.NewTestRequestContext()
	cbCtx := core.NewCallbackContext(reqCtx)

	res, err := pl.BeforeModel(context.Background(), cbCtx, req)
	require.ErrorIs(t, err, expectedErr)
	assert.Nil(t, res)
	assert.Equal(t, "base", req.Instructions)
}

func TestGlobalInstructions_EmptyResolvedInstructions(t *testing.T) {
	inst := core.NewInstructionsFromFunc(func(context.Context, core.ReadonlyContext) (string, error) {
		return "", nil
	})
	pl := NewGlobalInstructions(&inst)

	req := &core.ModelRequest{Instructions: "base"}
	reqCtx := testutil.NewTestRequestContext()
	cbCtx := core.NewCallbackContext(reqCtx)

	res, err := pl.BeforeModel(context.Background(), cbCtx, req)
	require.NoError(t, err)
	assert.Nil(t, res)
	assert.Equal(t, "base", req.Instructions)
}
