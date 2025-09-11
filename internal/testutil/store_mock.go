package testutil

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/core"
)

// SessionStoreMock is a function-based mock for core.SessionStore used in unit tests.
type SessionStoreMock struct {
	GetOrCreateFunc func(ctx context.Context, appName, userID, sessionID string) (*core.Session, error)
	AppendEventFunc func(ctx context.Context, sess *core.Session, ev *core.Event) error
	CloseFunc       func() error
}

// GetOrCreate calls the configured GetOrCreateFunc or returns a not implemented error.
func (m *SessionStoreMock) GetOrCreate(ctx context.Context, appName, userID, sessionID string) (*core.Session, error) {
	if m.GetOrCreateFunc != nil {
		return m.GetOrCreateFunc(ctx, appName, userID, sessionID)
	}

	return nil, fmt.Errorf("GetOrCreate not implemented")
}

// AppendEvent calls the configured AppendEventFunc or returns a not implemented error.
func (m *SessionStoreMock) AppendEvent(ctx context.Context, sess *core.Session, ev *core.Event) error {
	if m.AppendEventFunc != nil {
		return m.AppendEventFunc(ctx, sess, ev)
	}

	return fmt.Errorf("AppendEvent not implemented")
}

// Close calls the configured CloseFunc or returns nil.
func (m *SessionStoreMock) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// ArtifactStoreMock is a function-based mock for core.ArtifactStore used in unit tests.
type ArtifactStoreMock struct {
	SaveFunc func(
		ctx context.Context,
		appName, userID, sessionID, fileName string,
		artifact core.Part,
	) error

	LoadFunc func(
		ctx context.Context,
		appName, userID, sessionID, fileName string,
	) (core.Part, error)

	ListKeysFunc func(
		ctx context.Context,
		appName, userID, sessionID string,
	) ([]string, error)

	DeleteFunc func(
		ctx context.Context,
		appName, userID, sessionID, fileName string,
	) error
	CloseFunc func() error
}

// Save calls the configured SaveFunc or returns a not implemented error.
func (m *ArtifactStoreMock) Save(
	ctx context.Context,
	appName, userID, sessionID, fileName string,
	artifact core.Part,
) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, appName, userID, sessionID, fileName, artifact)
	}
	return fmt.Errorf("Save not implemented")
}

// Load calls the configured LoadFunc or returns a not implemented error.
func (m *ArtifactStoreMock) Load(
	ctx context.Context,
	appName, userID, sessionID, fileName string,
) (core.Part, error) {
	if m.LoadFunc != nil {
		return m.LoadFunc(ctx, appName, userID, sessionID, fileName)
	}
	return nil, fmt.Errorf("Load not implemented")
}

// ListKeys calls the configured ListKeysFunc or returns a not implemented error.
func (m *ArtifactStoreMock) ListKeys(
	ctx context.Context,
	appName, userID, sessionID string,
) ([]string, error) {
	if m.ListKeysFunc != nil {
		return m.ListKeysFunc(ctx, appName, userID, sessionID)
	}

	return nil, fmt.Errorf("ListKeys not implemented")
}

// Delete calls the configured DeleteFunc or returns a not implemented error.
func (m *ArtifactStoreMock) Delete(
	ctx context.Context,
	appName, userID, sessionID, fileName string,
) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, appName, userID, sessionID, fileName)
	}

	return fmt.Errorf("Delete not implemented")
}

// Close calls the configured CloseFunc or returns nil.
func (m *ArtifactStoreMock) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// MemoryStoreMock is a function-based mock for core.MemoryStore used in unit tests.
type MemoryStoreMock struct {
	AddSessionFunc func(ctx context.Context, session *core.Session) error
	SearchFunc     func(ctx context.Context, appName, userID, query string) (*core.SearchResult, error)
	CloseFunc      func() error
}

// AddSession calls the configured AddSessionFunc or returns a not implemented error.
func (m *MemoryStoreMock) AddSession(ctx context.Context, session *core.Session) error {
	if m.AddSessionFunc != nil {
		return m.AddSessionFunc(ctx, session)
	}

	return fmt.Errorf("AddSession not implemented")
}

// Search calls the configured SearchFunc or returns a not implemented error.
func (m *MemoryStoreMock) Search(
	ctx context.Context,
	appName, userID string,
	query string,
) (*core.SearchResult, error) {
	if m.SearchFunc != nil {
		return m.SearchFunc(ctx, appName, userID, query)
	}
	return nil, fmt.Errorf("Search not implemented")
}

// Close calls the configured CloseFunc or returns nil.
func (m *MemoryStoreMock) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// Compile-time assertions for interfaces
var (
	_ core.SessionStore  = (*SessionStoreMock)(nil)
	_ core.ArtifactStore = (*ArtifactStoreMock)(nil)
	_ core.MemoryStore   = (*MemoryStoreMock)(nil)
)
