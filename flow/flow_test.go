package flow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/session"
	"github.com/stretchr/testify/require"
)

// MockModel is a lightweight in‑memory Model useful for tests & examples.
type MockModel struct {
	info      core.ModelInfo
	responses map[string]string
}

// NewMockModel constructs a MockModel with basic tool support enabled.
func NewMockModel(name, provider string) *MockModel {
	return &MockModel{
		info: core.ModelInfo{
			Name:          name,
			Provider:      provider,
			SupportsTools: true,
		},
		responses: make(map[string]string),
	}
}

// AddResponse registers a deterministic canned completion for an input prompt.
func (m *MockModel) AddResponse(prompt, response string) {
	m.responses[prompt] = response
}

// Generate implements Model; emits optional streaming char chunks then final response.
func (m *MockModel) Generate(ctx context.Context, req core.ModelRequest) (<-chan core.ModelResponse, <-chan error) {
	respCh := make(chan core.ModelResponse, 16)
	errCh := make(chan error, 1)

	go func() {
		defer close(respCh)
		defer close(errCh)

		if len(req.Messages) == 0 {
			errCh <- fmt.Errorf("no contents provided")
			return
		}

		// Extract last content text
		last := req.Messages[len(req.Messages)-1]
		var inputText string
		for _, p := range last.Parts {
			if tp, ok := p.(*core.TextPart); ok {
				inputText += tp.Text
			}
		}

		full := m.responses[inputText]
		if full == "" {
			full = fmt.Sprintf("Mock response to: %s", inputText)
		}

		if req.Stream {
			for _, r := range full { // Emit character chunks as partials
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case respCh <- core.ModelResponse{
					Partial: true,
					Parts:   []core.Part{core.NewPartFromText(string(r))},
				}:
				}
			}
		}
		respCh <- core.ModelResponse{ // Final response
			Partial:      false,
			Parts:        []core.Part{core.NewPartFromText(full)},
			FinishReason: "stop",
		}
	}()

	return respCh, errCh
}

// Info implements Model interface
func (m *MockModel) Info() core.ModelInfo { return m.info }

func newTestRunContext() core.RequestContext {
	sessSvc := session.NewInMemoryStore()
	sess, _ := sessSvc.GetOrCreate(context.Background(), "app", "user1", "sess1")

	userParts := []core.Part{core.NewPartFromText("test message")}

	// Ensure current user message is present in history so ContentsProcessor includes it
	sess.AddEvent(core.NewUserContentEvent("run_id", userParts...))

	// Memory store mock
	memStore := &testutil.MemoryStoreMock{
		SearchFunc: func(_ context.Context, _, _ string, _ string) (*core.SearchResult, error) {
			return &core.SearchResult{Memories: nil}, nil
		},
		AddSessionFunc: func(_ context.Context, _ *core.Session) error { return nil },
	}

	reqCtx := core.NewRequestContext(core.RequestContextParams{
		RunID:         "run_id",
		Agent:         core.AgentInfo{Name: "TestAgent", Type: "flow-test"},
		UserParts:     userParts,
		MaxModelCalls: 100,
		Session:       sess,
		SessionStore:  sessSvc,
		ArtifactStore: nil,
		MemoryStore:   memStore,
	})

	return reqCtx
}

func TestSingleAgentFlow(t *testing.T) {
	mockModel := NewMockModel("test-model", "mock")
	mockModel.AddResponse("test message", "Hello! This is a test response.")
	agent := testutil.NewMockAgent("test-agent")
	agent.ModelVal = mockModel
	agent.ResolveInstructionsFunc = func(_ context.Context, _ core.ReadonlyContext) (string, error) {
		return "You are a test assistant.", nil
	}

	runCtx := newTestRunContext()

	f := NewSingleAgentFlow(agent)

	events := make([]*core.Event, 0, 8)
	eventCh := make(chan *core.Event, 64)

	// Queue writes into a channel; no locking required.
	q := testQueue(func(ev *core.Event) {
		eventCh <- ev
	})

	// Collector goroutine appends on the test goroutine’s behalf.
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

	err := f.Execute(ctx, runCtx, q)
	require.NoError(t, err, "Flow execution failed")

	// Close channel to stop collector and wait for it to drain.
	close(eventCh)
	<-done

	require.NotEmpty(t, events, "Expected at least one event from flow execution")
}

func TestSelector_ReturnsSingleAgentFlow_WhenIsolated(t *testing.T) {
	a := testutil.NewMockAgent("iso")
	a.ModelVal = NewMockModel("m", "mock")
	a.TransferToPeersEnabled = false
	a.SubAgentsList = nil
	sel := NewDefaultSelector()
	fl := sel.SelectFlow(a)
	_, ok := fl.(*SingleAgentFlow)
	require.True(t, ok, "expected SingleAgentFlow, got %T", fl)
}

func TestSelector_ReturnsMultiAgentFlow_WhenTransferEnabled(t *testing.T) {
	a := testutil.NewMockAgent("xfer")
	a.ModelVal = NewMockModel("m", "mock")
	a.TransferToPeersEnabled = true
	sel := NewDefaultSelector()
	fl := sel.SelectFlow(a)
	_, ok := fl.(*MultiAgentFlow)
	require.True(t, ok, "expected MultiAgentFlow, got %T", fl)
}

func TestSelector_ReturnsMultiAgentFlow_WhenHasSubAgents(t *testing.T) {
	a := testutil.NewMockAgent("subs")
	a.ModelVal = NewMockModel("m", "mock")
	a.TransferToPeersEnabled = false
	a.HasSubAgentsFunc = func() bool { return true }
	sel := NewDefaultSelector()
	fl := sel.SelectFlow(a)
	_, ok := fl.(*MultiAgentFlow)
	require.True(t, ok, "expected MultiAgentFlow, got %T", fl)
}

// testQueue is a helper EventWriter for tests
type testQueue func(ev *core.Event)

func (q testQueue) Write(ctx context.Context, ev *core.Event) error { q(ev); return nil }
