package model

import (
    "context"
    "errors"
    "testing"

    "github.com/hupe1980/agentmesh/core"
    "github.com/hupe1980/agentmesh/internal/testutil"
    "github.com/stretchr/testify/require"
)

// helper to build minimal RequestContext for tests
func newModelReqCtx(runID string, ag core.Agent, pm core.PluginManager) core.RequestContext {
    return testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
        p.Agent = ag
        p.RunID = runID
        p.MaxModelCalls = 10
        p.PluginManager = pm
    })
}

func TestExecuteModel_ShortCircuit(t *testing.T) {
    shortParts := []core.Part{core.NewPartFromText("cached")}
    pm := core.NewPluginManager(&testutil.PluginMock{
        BeforeModelFunc: func(
            context.Context,
            core.CallbackContext,
            *core.ModelRequest,
        ) (*core.ModelResponse, error) {
            return &core.ModelResponse{Parts: shortParts}, nil
        },
    })

    ag := testutil.NewMockAgent("A")
    ag.DescriptionVal = "model"
    ag.ModelVal = &testutil.MockModel{}
    rc := newModelReqCtx("run-short", ag, pm)

    respCh, errCh := ExecuteModel(context.Background(), rc, ag.ModelVal, &core.ModelRequest{})
    var resp *core.ModelResponse
    select {
    case r := <-respCh:
        resp = r
    case err := <-errCh:
        require.NoError(t, err)
    }
    // ensure no further responses
    _, ok := <-respCh
    require.False(t, ok)

    require.NotNil(t, resp)
    require.Equal(t, "cached", resp.Parts[0].(*core.TextPart).Text)
}

func TestExecuteModel_AfterReplacement(t *testing.T) {
    repl := []core.Part{core.NewPartFromText("replaced")}
    pm := core.NewPluginManager(&testutil.PluginMock{
        AfterModelFunc: func(
            context.Context,
            core.CallbackContext,
            *core.ModelResponse,
        ) (*core.ModelResponse, error) {
            return &core.ModelResponse{Parts: repl}, nil
        },
    })

    ag := testutil.NewMockAgent("A")
    ag.DescriptionVal = "model"
    ag.ModelVal = &testutil.MockModel{
        GenerateFunc: func(
            ctx context.Context,
            req *core.ModelRequest,
        ) (<-chan *core.ModelResponse, <-chan error) {
            respCh := make(chan *core.ModelResponse, 1)
            errCh := make(chan error, 1)
            respCh <- &core.ModelResponse{Parts: []core.Part{core.NewPartFromText("orig")}}
            close(respCh)
            close(errCh)
            return respCh, errCh
        },
    }
    rc := newModelReqCtx("run-after", ag, pm)

    respCh, errCh := ExecuteModel(context.Background(), rc, ag.ModelVal, &core.ModelRequest{})
    var resp *core.ModelResponse
    select {
    case r := <-respCh:
        resp = r
    case err := <-errCh:
        require.NoError(t, err)
    }

    require.NotNil(t, resp)
    require.Equal(t, "replaced", resp.Parts[0].(*core.TextPart).Text)
}

func TestExecuteModel_OnModelErrorRecovery(t *testing.T) {
    recov := []core.Part{core.NewPartFromText("recovered")}
    pm := core.NewPluginManager(&testutil.PluginMock{
        OnModelErrorFunc: func(
            ctx context.Context,
            cbCtx core.CallbackContext,
            req *core.ModelRequest,
            err error,
        ) (*core.ModelResponse, error) {
            return &core.ModelResponse{Parts: recov}, nil
        },
    })

    ag := testutil.NewMockAgent("A")
    ag.DescriptionVal = "model"
    ag.ModelVal = &testutil.MockModel{
        GenerateFunc: func(
            ctx context.Context,
            req *core.ModelRequest,
        ) (<-chan *core.ModelResponse, <-chan error) {
            respCh := make(chan *core.ModelResponse)
            errCh := make(chan error, 1)
            errCh <- errors.New("boom")
            close(respCh)
            close(errCh)
            return respCh, errCh
        },
    }
    rc := newModelReqCtx("run-err", ag, pm)

    respCh, errCh := ExecuteModel(context.Background(), rc, ag.ModelVal, &core.ModelRequest{})
    var resp *core.ModelResponse
    select {
    case r := <-respCh:
        resp = r
    case err := <-errCh:
        require.NoError(t, err)
    }

    require.NotNil(t, resp)
    require.Equal(t, "recovered", resp.Parts[0].(*core.TextPart).Text)
}
