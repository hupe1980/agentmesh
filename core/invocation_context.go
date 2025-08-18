package core

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/logging"
)

// InvocationContext carries execution state & helpers for an agent run.
// It encapsulates the mutable, per-invocation execution scope passed to an
// Agent's Run method. It aggregates:
//   - The ambient cancellation Context
//   - Identifiers (SessionID, InvocationID, Agent info)
//   - Input user Content
//   - Emission / resumption coordination channels
//   - Backing services (session, artifact, memory) for persistence concerns
//   - A working Session snapshot and pending StateDelta / Artifacts to commit
//   - Branch label for hierarchical flows
//
// State mutations performed via SetState accumulate in StateDelta until
// CommitStateDelta or EmitEvent applies them. Cloning produces an isolated
// delta/artifact buffer while keeping references to underlying services.
type InvocationContext struct {
	Context                 context.Context
	SessionID, InvocationID string
	Agent                   AgentInfo
	UserContent             Content
	Emit                    chan<- Event
	Resume                  <-chan struct{}
	SessionService          SessionStore
	ArtifactService         ArtifactStore
	MemoryService           MemoryStore
	Session                 *Session
	StateDelta              map[string]any
	Artifacts               []string
	Branch                  string
	Logger                  logging.Logger
}

// NewInvocationContext constructs an InvocationContext with empty state and
// artifact deltas.
func NewInvocationContext(
	ctx context.Context,
	sessionID, invocationID string,
	agent AgentInfo,
	userContent Content,
	emit chan<- Event,
	resume <-chan struct{},
	sess *Session,
	sessionService SessionStore,
	artifactService ArtifactStore,
	memoryService MemoryStore,
	logger logging.Logger,
) *InvocationContext {
	return &InvocationContext{
		Context:         ctx,
		SessionID:       sessionID,
		InvocationID:    invocationID,
		Agent:           agent,
		UserContent:     userContent,
		Emit:            emit,
		Resume:          resume,
		Session:         sess,
		SessionService:  sessionService,
		ArtifactService: artifactService,
		MemoryService:   memoryService,
		StateDelta:      map[string]any{},
		Artifacts:       []string{},
		Logger:          logger,
	}
}

// Done returns a channel closed when the underlying context is cancelled.
// Done mirrors context.Context's Done.
func (ic *InvocationContext) Done() <-chan struct{} { return ic.Context.Done() }

// Err returns the cancellation error (if any) from the underlying context.
func (ic *InvocationContext) Err() error { return ic.Context.Err() }

// GetState returns a staged (delta) value if present, else the persisted
// session value. The boolean reports whether a value was found.
func (ic *InvocationContext) GetState(k string) (any, bool) {
	if v, ok := ic.StateDelta[k]; ok {
		return v, true
	}
	if ic.Session != nil {
		return ic.Session.GetState(k)
	}
	return nil, false
}

// SetState stages a state mutation in the in-memory delta buffer. The change
// is persisted when CommitStateDelta is called or an emitted event merges it.
func (ic *InvocationContext) SetState(k string, v any) { ic.StateDelta[k] = v }

// ApplyStateDelta merges all pairs from d into the staged StateDelta.
func (ic *InvocationContext) ApplyStateDelta(d map[string]any) {
	for k, v := range d {
		ic.StateDelta[k] = v
	}
}

// AddArtifact stages an artifact id to be attached to the next emitted event.
func (ic *InvocationContext) AddArtifact(id string) { ic.Artifacts = append(ic.Artifacts, id) }

// SaveArtifact stores bytes in the ArtifactStore and stages the id for the
// next emitted event.
func (ic *InvocationContext) SaveArtifact(id string, data []byte) error {
	if ic.ArtifactService == nil {
		return fmt.Errorf("artifact service not configured")
	}
	if err := ic.ArtifactService.Save(ic.SessionID, id, data); err != nil {
		return err
	}
	ic.AddArtifact(id)
	return nil
}

// GetArtifact retrieves previously saved artifact bytes.
func (ic *InvocationContext) GetArtifact(id string) ([]byte, error) {
	if ic.ArtifactService == nil {
		return nil, fmt.Errorf("artifact service not configured")
	}
	return ic.ArtifactService.Get(ic.SessionID, id)
}

// ListArtifacts returns artifact IDs stored for the session.
func (ic *InvocationContext) ListArtifacts() ([]string, error) {
	if ic.ArtifactService == nil {
		return []string{}, nil
	}
	return ic.ArtifactService.List(ic.SessionID)
}

// SearchMemory queries the MemoryStore for relevant content.
func (ic *InvocationContext) SearchMemory(q string, limit int) ([]SearchResult, error) {
	if ic.MemoryService == nil {
		return []SearchResult{}, nil
	}
	return ic.MemoryService.Search(ic.SessionID, q, limit)
}

// StoreMemory appends content plus metadata to the MemoryStore.
func (ic *InvocationContext) StoreMemory(content string, md map[string]any) error {
	if ic.MemoryService == nil {
		return fmt.Errorf("memory service not configured")
	}
	return ic.MemoryService.Store(ic.SessionID, content, md)
}

// RefreshSession reloads the session snapshot from the SessionStore.
func (ic *InvocationContext) RefreshSession() error {
	if ic.SessionService == nil {
		return fmt.Errorf("session service not configured")
	}
	s, err := ic.SessionService.Get(ic.SessionID)
	if err != nil {
		return err
	}
	ic.Session = s
	return nil
}

// CommitStateDelta persists the accumulated StateDelta via the SessionStore
// then clears the in-memory delta. It is a no-op when there are no staged
// mutations.
func (ic *InvocationContext) CommitStateDelta() error {
	if len(ic.StateDelta) == 0 {
		return nil
	}
	if ic.SessionService == nil {
		return fmt.Errorf("session service not configured")
	}
	if err := ic.SessionService.ApplyDelta(ic.SessionID, ic.StateDelta); err != nil {
		return err
	}
	ic.StateDelta = map[string]any{}
	return nil
}

// GetSessionHistory returns all historical events for the session.
func (ic *InvocationContext) GetSessionHistory() []Event {
	if ic.Session == nil {
		return []Event{}
	}
	return ic.Session.GetEvents()
}

// GetAgentName returns the logical agent name for this invocation.
func (ic *InvocationContext) GetAgentName() string { return ic.Agent.Name }

// GetAgentType returns a categorization label for the agent.
func (ic *InvocationContext) GetAgentType() string { return ic.Agent.Type }

// Clone returns a shallow copy with deep-copied delta & artifact slices. It
// shares service pointers and is safe for speculative processing.
func (ic *InvocationContext) Clone() *InvocationContext {
	c := &InvocationContext{Context: ic.Context, SessionID: ic.SessionID, InvocationID: ic.InvocationID, Agent: ic.Agent, UserContent: ic.UserContent, Emit: ic.Emit, Resume: ic.Resume, SessionService: ic.SessionService, ArtifactService: ic.ArtifactService, MemoryService: ic.MemoryService, Session: ic.Session, StateDelta: map[string]any{}, Artifacts: []string{}, Branch: ic.Branch, Logger: ic.Logger}
	for k, v := range ic.StateDelta {
		c.StateDelta[k] = v
	}
	c.Artifacts = append(c.Artifacts, ic.Artifacts...)
	return c
}

// WithBranch clones the context and sets the Branch label to b.
func (ic *InvocationContext) WithBranch(b string) *InvocationContext {
	c := ic.Clone()
	c.Branch = b
	return c
}

// NewChildInvocationContext derives a context for a nested / child execution
// path. It clones the receiver, replaces the Emit & Resume channels, resets
// pending StateDelta & Artifacts buffers, and optionally sets a branch label
// if non-empty. Use in composite agents to intercept or isolate child output
// without mutating the parent's transient buffers.
func (ic *InvocationContext) NewChildInvocationContext(emit chan<- Event, resume <-chan struct{}, branch string) *InvocationContext {
	// Avoid Clone() to prevent wasted copying of StateDelta / Artifacts that we
	// immediately discard. Construct a lightweight derived context directly.
	finalBranch := ic.Branch
	if branch != "" {
		finalBranch = branch
	}
	return &InvocationContext{
		Context:         ic.Context,
		SessionID:       ic.SessionID,
		InvocationID:    ic.InvocationID,
		Agent:           ic.Agent,
		UserContent:     ic.UserContent,
		Emit:            emit,
		Resume:          resume,
		SessionService:  ic.SessionService,
		ArtifactService: ic.ArtifactService,
		MemoryService:   ic.MemoryService,
		Session:         ic.Session,
		StateDelta:      map[string]any{}, // fresh buffers
		Artifacts:       []string{},
		Branch:          finalBranch,
		Logger:          ic.Logger,
	}
}

// EmitEvent merges pending StateDelta / Artifacts into ev.Actions, sends it on
// the Emit channel, then resets those buffers. If the context is cancelled
// before emission it returns the cancellation error.
func (ic *InvocationContext) EmitEvent(ev Event) error {
	if len(ic.StateDelta) > 0 {
		if ev.Actions.StateDelta == nil {
			ev.Actions.StateDelta = map[string]any{}
		}
		for k, v := range ic.StateDelta {
			ev.Actions.StateDelta[k] = v
		}
	}
	if len(ic.Artifacts) > 0 {
		if ev.Actions.ArtifactDelta == nil {
			ev.Actions.ArtifactDelta = map[string]int{}
		}
		for _, id := range ic.Artifacts {
			ev.Actions.ArtifactDelta[id] = 1
		}
	}
	select {
	case <-ic.Context.Done():
		return ic.Context.Err()
	case ic.Emit <- ev:
	}
	ic.StateDelta = map[string]any{}
	ic.Artifacts = []string{}
	return nil
}

// WaitForResume blocks until the Resume channel signals or the context is
// cancelled. If Resume is nil it returns immediately.
func (ic *InvocationContext) WaitForResume() error {
	if ic.Resume == nil {
		return nil
	}
	select {
	case <-ic.Resume:
		return nil
	case <-ic.Context.Done():
		return ic.Context.Err()
	}
}
