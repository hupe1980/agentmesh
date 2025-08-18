package core

import (
	"context"
)

type testLogger struct{}

func (l testLogger) Debug(string, ...interface{}) {}
func (l testLogger) Info(string, ...interface{})  {}
func (l testLogger) Warn(string, ...interface{})  {}
func (l testLogger) Error(string, ...interface{}) {}
func (l testLogger) Fatal(string, ...interface{}) {}

type icMockSessionService struct {
	applied map[string]map[string]interface{}
}

func (s *icMockSessionService) Get(id string) (*Session, error)       { return NewSession(id), nil }
func (s *icMockSessionService) Create(id string) (*Session, error)    { return NewSession(id), nil }
func (s *icMockSessionService) AppendEvent(id string, ev Event) error { return nil }
func (s *icMockSessionService) ApplyDelta(id string, delta map[string]interface{}) error {
	if s.applied == nil {
		s.applied = map[string]map[string]interface{}{}
	}
	cp := map[string]interface{}{}
	for k, v := range delta {
		cp[k] = v
	}
	s.applied[id] = cp
	return nil
}

type icMockArtifactService struct{ saved map[string]map[string][]byte }

func (a *icMockArtifactService) Save(sid, aid string, data []byte) error {
	if a.saved == nil {
		a.saved = map[string]map[string][]byte{}
	}
	if _, ok := a.saved[sid]; !ok {
		a.saved[sid] = map[string][]byte{}
	}
	a.saved[sid][aid] = append([]byte{}, data...)
	return nil
}
func (a *icMockArtifactService) Get(sid, aid string) ([]byte, error) {
	if a.saved == nil {
		return nil, nil
	}
	if m, ok := a.saved[sid]; ok {
		return m[aid], nil
	}
	return nil, nil
}
func (a *icMockArtifactService) List(sid string) ([]string, error) {
	if a.saved == nil {
		return []string{}, nil
	}
	m := a.saved[sid]
	res := []string{}
	for k := range m {
		res = append(res, k)
	}
	return res, nil
}
func (a *icMockArtifactService) Delete(sid, aid string) error { return nil }

type icMockMemoryService struct{}

func (m *icMockMemoryService) Get(sessionID string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (m *icMockMemoryService) Put(sessionID string, delta map[string]any) error { return nil }
func (m *icMockMemoryService) Search(sid, q string, limit int) ([]SearchResult, error) {
	return []SearchResult{}, nil
}
func (m *icMockMemoryService) Store(sid, content string, metadata map[string]interface{}) error {
	return nil
}
func (m *icMockMemoryService) Delete(sid, memoryID string) error { return nil }

func newInvocationContextForTest() (*InvocationContext, chan Event) {
	emit := make(chan Event, 5)
	resume := make(chan struct{}, 5)
	sess := NewSession("sess-x")
	sSvc := &icMockSessionService{}
	aSvc := &icMockArtifactService{}
	mSvc := &icMockMemoryService{}
	return NewInvocationContext(context.Background(), "sess-x", "inv-x", AgentInfo{Name: "Agent1", Type: "test"}, Content{}, emit, resume, sess, sSvc, aSvc, mSvc, testLogger{}), emit
}
