package auth

import (
	"context"
	"sync"

	"github.com/hupe1980/agentmesh/core"
)

// Interface compliance (compile-time assertions)
var _ core.CredentialStore = (*InMemoryCredentialStore)(nil)

// InMemoryCredentialStore is a naive process-local CredentialStore.
type InMemoryCredentialStore struct {
	mu          sync.RWMutex
	credentials map[string]map[string]map[string]core.Credential
}

// NewInMemoryCredentialStore returns a new instance.
func NewInMemoryCredentialStore() *InMemoryCredentialStore {
	return &InMemoryCredentialStore{
		credentials: make(map[string]map[string]map[string]core.Credential),
	}
}

// Load retrieves the credential for the given (app, user).
func (s *InMemoryCredentialStore) Load(
	ctx context.Context,
	cbCtx core.CallbackContext,
) (core.Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bucket := s.getBucket(cbCtx)
	if bucket == nil {
		return nil, nil
	}

	return bucket["auth"], nil
}

// Save stores a credential for the given (app, user).
func (s *InMemoryCredentialStore) Save(
	ctx context.Context,
	cbCtx core.CallbackContext,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bucket := s.getBucket(cbCtx)
	bucket["auth"] = &core.NoAuthCredential{} // TODO: replace with real credential

	return nil
}

// Close implements io.Closer, is a no-op for the in-memory store.
func (s *InMemoryCredentialStore) Close() error {
	return nil
}

// getBucket returns the credential bucket for (app, user).
func (s *InMemoryCredentialStore) getBucket(
	cbCtx core.CallbackContext,
) map[string]core.Credential {
	app := cbCtx.AppName()
	user := cbCtx.UserID()

	if _, ok := s.credentials[app]; !ok {
		s.credentials[app] = make(map[string]map[string]core.Credential)
	}

	if _, ok := s.credentials[app]; !ok {
		s.credentials[app] = make(map[string]map[string]core.Credential)
	}

	if _, ok := s.credentials[app][user]; !ok {
		s.credentials[app][user] = make(map[string]core.Credential)
	}

	return s.credentials[app][user]
}
