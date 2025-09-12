package testutil

import (
	"context"

	"github.com/hupe1980/agentmesh/core"
)

// NewTestRequestContext constructs a minimal RequestContext suitable for tests.
// It wires in in-memory session, artifact, and memory stores via existing mocks
// and sets a generous MaxModelCalls default.
func NewTestRequestContext(optFns ...func(*core.RequestContextParams)) core.RequestContext {
	sess := core.NewSession("app", "user", "sess")

	params := core.RequestContextParams{
		RunID:         "run-test",
		Agent:         NewMockAgentIdentity("Agent", "test"),
		UserParts:     nil,
		MaxModelCalls: 999,
		Session:       sess,
		SessionStore: &SessionStoreMock{
			GetOrCreateFunc: func(_ context.Context, _, _, _ string) (*core.Session, error) {
				return sess, nil
			},
		},
		ArtifactStore: &ArtifactStoreMock{},
		MemoryStore:   &MemoryStoreMock{},
		PluginManager: core.NewPluginManager(),
	}

	for _, fn := range optFns {
		fn(&params)
	}

	return core.NewRequestContext(params)
}
