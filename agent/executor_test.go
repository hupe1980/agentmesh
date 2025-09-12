package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hupe1980/agentmesh/core"
	tu "github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/plugin"
)

// helper to build RequestContext with plugin using mocks
func newExecReqCtx(runID string, ag core.Agent, plugins ...core.Plugin) core.RequestContext {
	s := core.NewSession("app", "user", "sess")
	agInfo := core.AgentInfo{Name: ag.Name(), Type: "mock"}
	pm := plugin.NewManager(plugins...)
	return core.NewRequestContext(core.RequestContextParams{
		RunID:   runID,
		Agent:   agInfo,
		Session: s,
		SessionStore: &tu.SessionStoreMock{GetOrCreateFunc: func(_ context.Context, _, _, _ string) (*core.Session, error) {
			return s, nil
		}},
		ArtifactStore: &tu.ArtifactStoreMock{},
		MemoryStore:   &tu.MemoryStoreMock{},
		PluginManager: pm,
	})
}

func TestExecuteAgent_BeforeShortCircuit(t *testing.T) {
	ctx := context.Background()
	before := []core.Part{core.NewPartFromText("hi")}
	p := &tu.PluginMock{BeforeAgentFunc: func(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error) {
		return before, nil
	}}
	ag := newMockAgent("A", nil)
	w := &tu.CollectingWriter{}
	reqCtx := newExecReqCtx("run1", ag, p)

	require.NoError(t, ExecuteAgent(ctx, reqCtx, ag, w))
	require.Equal(t, 0, ag.RunCount(), "agent Run should be skipped")
	require.Len(t, w.Events, 1)
	require.Equal(t, "hi", w.Events[0].Parts[0].(*core.TextPart).Text)
}

func TestExecuteAgent_BeforeAndAfterShortCircuit(t *testing.T) {
	ctx := context.Background()
	before := []core.Part{core.NewPartFromText("pre")}
	after := []core.Part{core.NewPartFromText("post")}
	p := &tu.PluginMock{BeforeAgentFunc: func(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error) {
		return before, nil
	}, AfterAgentFunc: func(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error) {
		return after, nil
	}}
	ag := newMockAgent("A", nil)
	w := &tu.CollectingWriter{}
	reqCtx := newExecReqCtx("run2", ag, p)

	require.NoError(t, ExecuteAgent(ctx, reqCtx, ag, w))
	require.Equal(t, 0, ag.RunCount())
	require.Len(t, w.Events, 2)
	require.Equal(t, "pre", w.Events[0].Parts[0].(*core.TextPart).Text)
	require.Equal(t, "post", w.Events[1].Parts[0].(*core.TextPart).Text)
}

func TestExecuteAgent_Normal_NoAfter(t *testing.T) {
	ctx := context.Background()
	ag := newMockAgent("A", func(_ context.Context, _ core.RequestContext, w core.EventWriter) error {
		return w.Write(context.Background(), core.NewFullAssistantEvent("run3", "A", core.NewPartFromText("orig")))
	})
	w := &tu.CollectingWriter{}
	reqCtx := newExecReqCtx("run3", ag /* no plugins */)

	require.NoError(t, ExecuteAgent(ctx, reqCtx, ag, w))
	require.Equal(t, 1, ag.RunCount())
	require.Len(t, w.Events, 1)
	require.Equal(t, "orig", w.Events[0].Parts[0].(*core.TextPart).Text)
}

func TestExecuteAgent_Normal_WithAfter(t *testing.T) {
	ctx := context.Background()
	p := &tu.PluginMock{AfterAgentFunc: func(context.Context, core.CallbackContext, core.Agent) ([]core.Part, error) {
		return []core.Part{core.NewPartFromText("after")}, nil
	}}
	ag := newMockAgent("A", func(_ context.Context, _ core.RequestContext, w core.EventWriter) error {
		return w.Write(context.Background(), core.NewFullAssistantEvent("run4", "A", core.NewPartFromText("orig")))
	})
	w := &tu.CollectingWriter{}
	reqCtx := newExecReqCtx("run4", ag, p)

	require.NoError(t, ExecuteAgent(ctx, reqCtx, ag, w))
	require.Equal(t, 1, ag.RunCount())
	require.Len(t, w.Events, 2)
	require.Equal(t, "orig", w.Events[0].Parts[0].(*core.TextPart).Text)
	require.Equal(t, "after", w.Events[1].Parts[0].(*core.TextPart).Text)
}
