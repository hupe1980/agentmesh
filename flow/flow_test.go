package flow

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/session"
	"github.com/stretchr/testify/require"
)

func newTestRunContext() core.RequestContext {
	sessSvc := session.NewInMemoryStore()
	sess, _ := sessSvc.GetOrCreate(context.Background(), "app", "user1", "sess1")
	ag := testutil.NewMockAgent("TestAgent")
	userParts := []core.Part{core.NewPartFromText("test message")}
	sess.AddEvent(core.NewUserContentEvent("run_id", userParts...))
	memStore := &testutil.MemoryStoreMock{
		SearchFunc: func(_ context.Context, _, _ string, _ string) (*core.SearchResult, error) {
			return &core.SearchResult{Memories: nil}, nil
		},
		AddSessionFunc: func(_ context.Context, _ *core.Session) error { return nil },
	}
	return testutil.NewTestRequestContext(func(p *core.RequestContextParams) {
		p.Agent = ag
		p.RunID = "run_id"
		p.UserParts = userParts
		p.MaxModelCalls = 100
		p.Session = sess
		p.SessionStore = sessSvc
		p.MemoryStore = memStore
	})
}

func TestSingleAgentFlow(t *testing.T) {
	mockModel := &testutil.MockModel{InfoVal: core.ModelInfo{Name: "test-model", Provider: "mock", SupportsTools: true}}
	// Optionally customize via mockModel.GenerateFunc
	agent := testutil.NewMockAgent("test-agent")
	agent.ModelVal = mockModel
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "You are a test assistant.", nil
	}

	f := NewSingleAgentFlow(
		agent,
		&Executors{
			AgentExecutor: testutil.NewAgentExecutorMock(),
			ModelExecutor: testutil.NewModelExecutorMock(),
		},
	)
	reqCtx := newTestRunContext()

	events := make([]*core.Event, 0, 8)
	eventCh := make(chan *core.Event, 64)

	// Queue writes into a channel; no locking required.
	q := testQueue(func(ev *core.Event) {
		eventCh <- ev
	})

	// Collector goroutine appends on the test goroutine's behalf.
	done := make(chan struct{})
	go func() {
		for ev := range eventCh {
			events = append(events, ev)
		}
		close(done)
	}()

	// Execute with a timeout to avoid hanging tests.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := f.Execute(ctx, reqCtx, q)
	require.NoError(t, err, "Flow execution failed")

	// Close channel to stop collector and wait for it to drain.
	close(eventCh)
	<-done

	require.NotEmpty(t, events, "Expected at least one event from flow execution")
}

func TestSelector_ReturnsSingleAgentFlow_WhenIsolated(t *testing.T) {
	a := testutil.NewMockAgent("iso")
	a.ModelVal = &testutil.MockModel{InfoVal: core.ModelInfo{Name: "m", Provider: "mock", SupportsTools: true}}
	a.TransferToPeersEnabled = false
	a.SubAgentsList = nil
	sel := NewDefaultSelector(&Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
	})
	fl := sel.SelectFlow(a)
	_, ok := fl.(*SingleAgentFlow)
	require.True(t, ok, "expected SingleAgentFlow, got %T", fl)
}

func TestSelector_ReturnsMultiAgentFlow_WhenTransferEnabled(t *testing.T) {
	a := testutil.NewMockAgent("xfer")
	a.ModelVal = &testutil.MockModel{InfoVal: core.ModelInfo{Name: "m", Provider: "mock", SupportsTools: true}}
	a.TransferToPeersEnabled = true
	sel := NewDefaultSelector(&Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
	})
	fl := sel.SelectFlow(a)
	_, ok := fl.(*MultiAgentFlow)
	require.True(t, ok, "expected MultiAgentFlow, got %T", fl)
}

func TestSelector_ReturnsMultiAgentFlow_WhenHasSubAgents(t *testing.T) {
	a := testutil.NewMockAgent("subs")
	a.ModelVal = &testutil.MockModel{InfoVal: core.ModelInfo{Name: "m", Provider: "mock", SupportsTools: true}}
	a.TransferToPeersEnabled = false
	a.HasSubAgentsFunc = func() bool { return true }
	sel := NewDefaultSelector(&Executors{
		AgentExecutor: testutil.NewAgentExecutorMock(),
		ModelExecutor: testutil.NewModelExecutorMock(),
	})
	fl := sel.SelectFlow(a)
	_, ok := fl.(*MultiAgentFlow)
	require.True(t, ok, "expected MultiAgentFlow, got %T", fl)
}

// testQueue is a helper EventWriter for tests
type testQueue func(ev *core.Event)

func (q testQueue) Write(ctx context.Context, ev *core.Event) error { q(ev); return nil }
