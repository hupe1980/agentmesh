package core

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/logging"
)

// --- Test helpers ---
type tcMockSessionService struct{ sessions map[string]*Session }

func (m *tcMockSessionService) Get(id string) (*Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	s := NewSession(id)
	if m.sessions == nil {
		m.sessions = map[string]*Session{}
	}
	m.sessions[id] = s
	return s, nil
}
func (m *tcMockSessionService) Create(id string) (*Session, error) { return m.Get(id) }
func (m *tcMockSessionService) AppendEvent(id string, ev Event) error {
	if s, ok := m.sessions[id]; ok {
		s.Events = append(s.Events, ev)
	}
	return nil
}
func (m *tcMockSessionService) ApplyDelta(id string, delta map[string]interface{}) error {
	if s, ok := m.sessions[id]; ok {
		for k, v := range delta {
			s.State[k] = v
		}
	}
	return nil
}

type tcMockArtifactService struct{ data map[string]map[string][]byte }

func (a *tcMockArtifactService) Save(sid, aid string, b []byte) error {
	if a.data == nil {
		a.data = map[string]map[string][]byte{}
	}
	if _, ok := a.data[sid]; !ok {
		a.data[sid] = map[string][]byte{}
	}
	a.data[sid][aid] = append([]byte{}, b...)
	return nil
}
func (a *tcMockArtifactService) Get(sid, aid string) ([]byte, error) {
	if a.data == nil {
		return nil, nil
	}
	if m, ok := a.data[sid]; ok {
		return m[aid], nil
	}
	return nil, nil
}
func (a *tcMockArtifactService) List(sid string) ([]string, error) {
	if a.data == nil {
		return []string{}, nil
	}
	res := []string{}
	for k := range a.data[sid] {
		res = append(res, k)
	}
	return res, nil
}
func (a *tcMockArtifactService) Delete(sid, aid string) error { return nil }

type tcMockMemoryService struct{}

func (m *tcMockMemoryService) Get(sid string) (map[string]any, error)     { return map[string]any{}, nil }
func (m *tcMockMemoryService) Put(sid string, delta map[string]any) error { return nil }
func (m *tcMockMemoryService) Search(sid, q string, limit int) ([]SearchResult, error) {
	return []SearchResult{{ID: "test-memory", Content: "Test memory content", Score: 0.9, Metadata: map[string]interface{}{"test": true}}}, nil
}
func (m *tcMockMemoryService) Store(sid, content string, metadata map[string]interface{}) error {
	return nil
}
func (m *tcMockMemoryService) Delete(sid, memoryID string) error { return nil }

func createTestInvocationContext() *RunContext {
	sessSvc := &tcMockSessionService{sessions: map[string]*Session{}}
	artSvc := &tcMockArtifactService{data: map[string]map[string][]byte{}}
	memSvc := &tcMockMemoryService{}
	sess, _ := sessSvc.Create("test-session")
	emit := make(chan Event, 10)
	resume := make(chan struct{}, 10)
	return NewRunContext(
		context.Background(), "test-session", "test-invocation", AgentInfo{Name: "Test Agent", Type: "test"},
		Content{Role: "user", Parts: []Part{TextPart{Text: "Test input"}}},
		emit, resume, sess, sessSvc, artSvc, memSvc, logging.NoOpLogger{},
	)
}

func TestToolContext_BasicFunctionality(t *testing.T) {
	inv := createTestInvocationContext()
	tc := NewToolContext(inv, "test-call-id")
	if !tc.IsValid() {
		t.Fatal("expected valid tool context")
	}
	if tc.SessionID() != "test-session" {
		t.Errorf("session id mismatch")
	}
	if tc.RunID() != "test-invocation" {
		t.Errorf("run id mismatch")
	}
	if tc.FunctionCallID() != "test-call-id" {
		t.Errorf("function call id mismatch")
	}
	if tc.AgentName() != "Test Agent" {
		t.Errorf("agent name mismatch")
	}
	if tc.Logger() == nil {
		t.Errorf("expected logger")
	}
}

func TestToolContext_StateManagement(t *testing.T) {
	tc := NewToolContext(NewRunContext(
		context.Background(), "test-session", "test-invocation", AgentInfo{Name: "Test Agent", Type: "test"},
		Content{}, nil, nil, nil, nil, nil, nil, nil,
	), "test-call-id")
	tc.SetState("test_key", "test_value")
	actions := tc.Actions()
	if actions.StateDelta == nil {
		t.Fatal("missing state delta")
	}
	if v, ok := actions.StateDelta["test_key"]; !ok || v != "test_value" {
		t.Errorf("unexpected state delta: %+v", actions.StateDelta)
	}
}

func TestToolContext_AgentFlowControl(t *testing.T) {
	tc := NewToolContext(createTestInvocationContext(), "test-call-id")
	tc.SkipSummarization()
	tc.TransferToAgent("other-agent")
	tc.Escalate()
	actions := tc.Actions()
	if actions.SkipSummarization == nil || !*actions.SkipSummarization {
		t.Error("skip summarization not set")
	}
	if actions.TransferToAgent == nil || *actions.TransferToAgent != "other-agent" {
		t.Error("transfer not set")
	}
	if actions.Escalate == nil || !*actions.Escalate {
		t.Error("escalate not set")
	}
}

func TestToolContext_ArtifactManagement(t *testing.T) {
	tc := NewToolContext(createTestInvocationContext(), "test-call-id")
	if err := tc.SaveArtifact("a1", []byte("data")); err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	b, err := tc.LoadArtifact("a1")
	if err != nil || string(b) != "data" {
		t.Fatalf("load artifact mismatch: %v %s", err, string(b))
	}
	list, err := tc.ListArtifacts()
	if err != nil || len(list) != 1 || list[0] != "a1" {
		t.Fatalf("list artifacts mismatch: %v %v", err, list)
	}
}

func TestToolContext_MemoryManagement(t *testing.T) {
	tc := NewToolContext(createTestInvocationContext(), "test-call-id")
	if err := tc.StoreMemory("content", map[string]interface{}{"test": true}); err != nil {
		t.Fatalf("store memory: %v", err)
	}
	res, err := tc.SearchMemory("test", 10)
	if err != nil || len(res) != 1 {
		t.Fatalf("search memory: %v len=%d", err, len(res))
	}
}

func TestToolContext_Validation(t *testing.T) {
	if (&ToolContext{}).IsValid() {
		t.Error("invalid context should not be valid")
	}
	inv := createTestInvocationContext()
	tc := NewToolContext(inv, "test-call-id")
	if !tc.IsValid() {
		t.Error("expected valid tool context")
	}
	if err := tc.Validate(); err != nil {
		t.Errorf("validate error: %v", err)
	}
}
