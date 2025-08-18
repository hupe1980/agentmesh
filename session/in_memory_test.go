package session

import (
	"testing"

	"github.com/hupe1980/agentmesh/core"
)

// Interface compliance (compile-time assertion)
var _ core.SessionStore = (*InMemoryStore)(nil)

func TestInMemorySessionStore_InterfaceOnly(t *testing.T) {
	// No runtime behavior needed; existence of this test file ensures the assertion above is compiled.
}
