package flow

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// partialStreamingModel emits two partial chunks followed by a final full response.
type partialStreamingModel struct{}

func (m *partialStreamingModel) Generate(
	ctx context.Context,
	_ *core.ModelRequest,
) (
	<-chan *core.ModelResponse,
	<-chan error,
) {
	respCh := make(chan *core.ModelResponse, 3)
	errCh := make(chan error, 1)
	go func() {
		defer close(respCh)
		defer close(errCh)
		respCh <- &core.ModelResponse{Partial: true, Parts: []core.Part{core.NewPartFromText("pa")}}
		respCh <- &core.ModelResponse{Partial: true, Parts: []core.Part{core.NewPartFromText("rt")}}
		respCh <- &core.ModelResponse{Partial: false, Parts: []core.Part{core.NewPartFromText("final")}}
	}()
	return respCh, errCh
}
func (m *partialStreamingModel) Info() core.ModelInfo {
	return core.ModelInfo{Name: "m", Provider: "test"}
}

// TestBaseFlow_PartialBuffering ensures partial events are captured and written in order
// and that only the final event is non-partial.
func TestBaseFlow_PartialBuffering(t *testing.T) {
	agent := testutil.NewMockAgent("A")
	agent.ModelVal = &partialStreamingModel{}
	agent.ToolsMap = map[string]core.Tool{}
	agent.FunctionCallingEnabled = true
	agent.ResolveInstructionsFunc = func(
		ctx context.Context,
		_ core.ReadonlyContext,
	) (string, error) {
		return "inst", nil
	}

	f := NewBaseFlow(agent, &Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
	})

	// Add minimal request processor
	f.AddRequestProcessor(&mockReqProc{
		name: "addUser",
		fn: func(
			ctx context.Context,
			rc core.RequestContext,
			req *core.ModelRequest,
			a Agent,
		) error {
			req.Messages = []*core.Message{{
				Role:  core.RoleUser,
				Parts: []core.Part{core.NewPartFromText("hi")},
			}}
			return nil
		},
	})

	q := &testutil.CollectingWriter{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, f.Execute(ctx, newTestRunContext(), q))

	// Expect 3 events: 2 partial assistant + 1 final assistant
	require.Len(t, q.Events, 3)
	for i, ev := range q.Events {
		assert.Equal(t, core.RoleAssistant, ev.Role())
		if i < 2 {
			assert.True(t, ev.IsPartial())
		} else {
			assert.False(t, ev.IsPartial())
		}
	}
}
