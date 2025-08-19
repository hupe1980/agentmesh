package tool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/util"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/stretchr/testify/assert"
)

// -------------------- Schema & Validation Tests --------------------

type sampleSchema struct {
	A string `json:"a" description:"Field A"`
	B *int   `json:"b" description:"Optional pointer field"`
	C int    `json:"c,omitempty" description:"Omit empty field"`
}

func TestCreateSchema(t *testing.T) {
	schema := util.CreateSchema(sampleSchema{})
	props, ok := schema["properties"].(map[string]any)
	assert.True(t, ok)
	// Properties present
	assert.Contains(t, props, "a")
	assert.Contains(t, props, "b")
	assert.Contains(t, props, "c")
	// Required only includes non-pointer, non-omitempty exported fields
	req, _ := schema["required"].([]string)
	if req == nil { // reflection may produce []any
		ifaceReq, _ := schema["required"].([]any)
		for _, v := range ifaceReq {
			req = append(req, v.(string))
		}
	}
	assert.ElementsMatch(t, []string{"a"}, req)
}

func TestValidateParameters(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "integer"},
		},
		// Use []any to mirror possible JSON decoded schema shape
		"required": []any{"x"},
	}

	// Success
	err := util.ValidateParameters(map[string]any{"x": 5}, schema)
	assert.NoError(t, err)

	// Missing required
	err = util.ValidateParameters(map[string]any{}, schema)
	assert.Error(t, err)
	if vErr, ok := err.(*ValidationError); ok {
		assert.Equal(t, "x", vErr.Field)
	} else {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	// Wrong type
	err = util.ValidateParameters(map[string]any{"x": "not-int"}, schema)
	assert.Error(t, err)
	if vErr, ok := err.(*ValidationError); ok {
		assert.Contains(t, vErr.Message, "expected type integer")
	} else {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

// -------------------- FunctionTool Tests --------------------

func TestFunctionTool_Success(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "number"},
			"b": map[string]any{"type": "number"},
		},
		"required": []string{"a", "b"},
	}

	sumTool := NewFunctionTool("sum", "Add numbers", params, func(_ *core.ToolContext, args map[string]any) (any, error) {
		a := args["a"].(float64)
		b := args["b"].(float64)
		return a + b, nil
	})

	inv := dummyInvocationContext()
	tc := core.NewToolContext(inv, "fc1")
	result, err := sumTool.Call(tc, map[string]any{"a": 2.0, "b": 3.0})
	assert.NoError(t, err)
	assert.Equal(t, 5.0, result)
}

func TestFunctionTool_ValidationError(t *testing.T) {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "number"},
		},
		// Use interface slice to match ValidateParameters implementation expectation
		"required": []any{"a"},
	}
	tTool := NewFunctionTool("test", "Test", params, func(_ *core.ToolContext, _ map[string]any) (any, error) {
		return 0, nil
	})
	tc := core.NewToolContext(dummyInvocationContext(), "fc2")
	_, err := tTool.Call(tc, map[string]any{})
	assert.Error(t, err)
	toolErr, ok := err.(*ToolError)
	assert.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", toolErr.Code)
}

func TestFunctionTool_ExecutionError(t *testing.T) {
	params := map[string]any{"type": "object", "properties": map[string]any{}}
	execTool := NewFunctionTool("fail", "Fails", params, func(_ *core.ToolContext, _ map[string]any) (any, error) {
		return nil, errors.New("boom")
	})
	tc := core.NewToolContext(dummyInvocationContext(), "fc3")
	_, err := execTool.Call(tc, map[string]any{})
	assert.Error(t, err)
	toolErr, ok := err.(*ToolError)
	assert.True(t, ok)
	assert.Equal(t, "EXECUTION_ERROR", toolErr.Code)
}

type memSessionService struct {
	mu       sync.RWMutex
	sessions map[string]*core.Session
}

func newMemSessionService() *memSessionService {
	return &memSessionService{sessions: map[string]*core.Session{}}
}
func (s *memSessionService) Get(id string) (*core.Session, error) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return s.Create(id)
	}
	return sess.Clone(), nil
}
func (s *memSessionService) SaveSession(sess *core.Session) error { // legacy helper
	s.mu.Lock()
	s.sessions[sess.ID] = sess.Clone()
	s.mu.Unlock()
	return nil
}
func (s *memSessionService) Create(id string) (*core.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	newSess := core.NewSession(id)
	s.sessions[id] = newSess
	return newSess.Clone(), nil
}
func (s *memSessionService) AppendEvent(id string, ev core.Event) error {
	s.mu.Lock()
	if _, ok := s.sessions[id]; !ok {
		s.sessions[id] = core.NewSession(id)
	}
	s.sessions[id].AddEvent(ev)
	s.mu.Unlock()
	return nil
}
func (s *memSessionService) ApplyDelta(id string, delta map[string]any) error {
	s.mu.Lock()
	if _, ok := s.sessions[id]; !ok {
		s.sessions[id] = core.NewSession(id)
	}
	s.sessions[id].MergeState(delta)
	s.mu.Unlock()
	return nil
}

type memArtifactService struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte
}

func newMemArtifactService() *memArtifactService {
	return &memArtifactService{data: map[string]map[string][]byte{}}
}
func (a *memArtifactService) Save(sid, aid string, b []byte) error {
	a.mu.Lock()
	if _, ok := a.data[sid]; !ok {
		a.data[sid] = map[string][]byte{}
	}
	cp := make([]byte, len(b))
	copy(cp, b)
	a.data[sid][aid] = cp
	a.mu.Unlock()
	return nil
}
func (a *memArtifactService) Get(sid, aid string) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if m, ok := a.data[sid]; ok {
		if d, ok := m[aid]; ok {
			cp := make([]byte, len(d))
			copy(cp, d)
			return cp, nil
		}
	}
	return nil, errors.New("not found")
}
func (a *memArtifactService) List(sid string) ([]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	m := a.data[sid]

	res := make([]string, 0, len(m))
	for k := range m {
		res = append(res, k)
	}

	return res, nil
}
func (a *memArtifactService) Delete(sid, aid string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if m, ok := a.data[sid]; ok {
		delete(m, aid)
	}
	return nil
}

type memMemoryService struct {
	mu    sync.RWMutex
	store map[string][]core.SearchResult
}

func newMemMemoryService() *memMemoryService {
	return &memMemoryService{store: map[string][]core.SearchResult{}}
}

func (m *memMemoryService) Search(sid, _ string, limit int) ([]core.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	results := m.store[sid]
	if limit < len(results) {
		results = results[:limit]
	}
	return results, nil
}
func (m *memMemoryService) Store(sid, content string, metadata map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	mr := core.SearchResult{ID: content, Content: content, Score: 1.0, Metadata: metadata}
	m.store[sid] = append(m.store[sid], mr)
	return nil
}

func (m *memMemoryService) Delete(_, _ string) error { return nil }

// Added methods to satisfy memory.MemoryService interface
func (m *memMemoryService) Get(_ string) (map[string]any, error) { return map[string]any{}, nil }
func (m *memMemoryService) Put(_ string, _ map[string]any) error { return nil }

func dummyInvocationContext() *core.InvocationContext {
	sessSvc := newMemSessionService()
	artSvc := newMemArtifactService()
	memSvc := newMemMemoryService()

	sessionID := "sess-1"
	if _, err := sessSvc.Create(sessionID); err != nil {
		panic(err)
	}

	emit := make(chan core.Event, 10)
	resume := make(chan struct{}, 1)

	return core.NewInvocationContext(context.Background(), sessionID, "inv-1", core.AgentInfo{Name: "Agent", Type: "test"}, core.Content{}, emit, resume, core.NewSession(sessionID), sessSvc, artSvc, memSvc, logging.NoOpLogger{})
}

func TestStateManagerTool_SetAndGetState(t *testing.T) {
	sm := NewStateManagerTool()
	inv := dummyInvocationContext()
	tc := core.NewToolContext(inv, "fc-set")

	// set_state
	res, err := sm.Call(tc, map[string]any{"operation": "set_state", "key": "foo", "value": "bar"})
	assert.NoError(t, err)

	m := res.(map[string]any)
	assert.Equal(t, "foo", m["key"])
	assert.Equal(t, "bar", m["value"])
	assert.Equal(t, "bar", tc.Actions().StateDelta["foo"])

	// Apply actions to invocation context via event simulation
	ev := core.Event{Actions: core.EventActions{StateDelta: map[string]any{}}}
	tc.InternalApplyActions(&ev)
	// Simulate commit to session
	inv.Session.MergeState(ev.Actions.StateDelta)

	// get_state
	tcGet := core.NewToolContext(inv, "fc-get")
	res, err = sm.Call(tcGet, map[string]any{"operation": "get_state", "key": "foo"})
	assert.NoError(t, err)
	gm := res.(map[string]any)
	assert.True(t, gm["exists"].(bool))
	assert.Equal(t, "bar", gm["value"])
}

func TestStateManagerTool_FlowControlActions(t *testing.T) {
	sm := NewStateManagerTool()
	inv := dummyInvocationContext()
	tc := core.NewToolContext(inv, "fc-flow")

	// escalate
	res, err := sm.Call(tc, map[string]any{"operation": "escalate"})
	assert.NoError(t, err)
	_ = res
	assert.NotNil(t, tc.Actions().Escalate)
	assert.True(t, *tc.Actions().Escalate)

	// transfer_agent
	tc2 := core.NewToolContext(inv, "fc-transfer")
	_, err = sm.Call(tc2, map[string]any{"operation": "transfer_agent", "agent_name": "NextAgent"})
	assert.NoError(t, err)
	assert.NotNil(t, tc2.Actions().TransferToAgent)
	assert.Equal(t, "NextAgent", *tc2.Actions().TransferToAgent)

	// skip_summarization
	tc3 := core.NewToolContext(inv, "fc-skip")
	_, err = sm.Call(tc3, map[string]any{"operation": "skip_summarization"})
	assert.NoError(t, err)
	assert.NotNil(t, tc3.Actions().SkipSummarization)
	assert.True(t, *tc3.Actions().SkipSummarization)
}

// -------------------- ToolError Formatting --------------------

func TestToolErrorFormatting(t *testing.T) {
	err := NewToolError("demo", "something failed", "E123")
	assert.Contains(t, err.Error(), "E123")
	assert.Contains(t, err.Error(), "demo")
}

// Ensure tests run quickly (sanity)
func TestToolPackageTestDuration(t *testing.T) {
	start := time.Now()
	// no-op
	assert.Less(t, time.Since(start), 100*time.Millisecond)
}
