package core

import (
	"context"
	"fmt"

	"maps"

	"github.com/hupe1980/agentmesh/logging"
)

// RunContext carries execution state & helpers for an agent run.
// It encapsulates the mutable, per-invocation execution scope passed to an
// Agent's Run method. It aggregates:
//   - The ambient cancellation Context
//   - Identifiers (SessionID, RunID, Agent info)
//   - Input user Content
//   - Emission / resumption coordination channels
//   - Backing services (session, artifact, memory) for persistence concerns
//   - A working Session snapshot and pending StateDelta / Artifacts to commit
//   - Branch label for hierarchical flows
//
// State mutations performed via SetState accumulate in StateDelta until
// CommitStateDelta or EmitEvent applies them. Cloning produces an isolated
// delta/artifact buffer while keeping references to underlying services.
type RunContext struct {
	Context          context.Context
	SessionID, RunID string
	Agent            AgentInfo
	UserContent      Content
	MaxModelCalls    int
	Emit             chan<- Event
	Resume           <-chan struct{}
	SessionStore     SessionStore
	ArtifactStore    ArtifactStore
	MemoryStore      MemoryStore
	Limiter          *ModelLimiter
	Session          *Session
	StateDelta       map[string]any
	Artifacts        []string
	Branch           string

	*loggerAdapter
}

// NewRunContext constructs a RunContext with empty state and
// artifact deltas.
func NewRunContext(
	ctx context.Context,
	sessionID, runID string,
	agent AgentInfo,
	userContent Content,
	maxModelCalls int,
	emit chan<- Event,
	resume <-chan struct{},
	sess *Session,
	sessionStore SessionStore,
	artifactStore ArtifactStore,
	memoryStore MemoryStore,
	logger logging.Logger,
) *RunContext {
	return &RunContext{
		Context:       ctx,
		SessionID:     sessionID,
		RunID:         runID,
		Agent:         agent,
		UserContent:   userContent,
		MaxModelCalls: maxModelCalls,
		Emit:          emit,
		Resume:        resume,
		Session:       sess,
		SessionStore:  sessionStore,
		ArtifactStore: artifactStore,
		MemoryStore:   memoryStore,
		Limiter:       NewModelLimiter(maxModelCalls),
		StateDelta:    map[string]any{},
		Artifacts:     []string{},
		loggerAdapter: newLoggerAdapter(logger),
	}
}

// Done returns a channel closed when the underlying context is cancelled.
func (ic *RunContext) Done() <-chan struct{} { return ic.Context.Done() }

// Err returns the cancellation error (if any) from the underlying context.
func (ic *RunContext) Err() error { return ic.Context.Err() }

// GetState returns a staged (delta) value if present, else the persisted session value.
func (ic *RunContext) GetState(k string) (any, bool) {
	if v, ok := ic.StateDelta[k]; ok {
		return v, true
	}

	if ic.Session != nil {
		return ic.Session.GetState(k)
	}

	return nil, false
}

// SetState stages a state mutation in the in-memory delta buffer.
func (ic *RunContext) SetState(k string, v any) { ic.StateDelta[k] = v }

// ApplyStateDelta merges all pairs from d into the staged StateDelta.
func (ic *RunContext) ApplyStateDelta(d map[string]any) {
	maps.Copy(ic.StateDelta, d)
}

// AddArtifact stages an artifact id to be attached to the next emitted event.
func (ic *RunContext) AddArtifact(id string) { ic.Artifacts = append(ic.Artifacts, id) }

// SaveArtifact stores bytes in the ArtifactStore and stages the id for the next emitted event.
func (ic *RunContext) SaveArtifact(id string, data []byte) error {
	if ic.ArtifactStore == nil {
		return fmt.Errorf("artifact store not configured")
	}

	if err := ic.ArtifactStore.Save(ic.SessionID, id, data); err != nil {
		return err
	}

	ic.AddArtifact(id)

	return nil
}

// GetArtifact retrieves previously saved artifact bytes.
func (ic *RunContext) GetArtifact(id string) ([]byte, error) {
	if ic.ArtifactStore == nil {
		return nil, fmt.Errorf("artifact store not configured")
	}

	return ic.ArtifactStore.Get(ic.SessionID, id)
}

// ListArtifacts returns artifact IDs stored for the session.
func (ic *RunContext) ListArtifacts() ([]string, error) {
	if ic.ArtifactStore == nil {
		return []string{}, nil
	}

	return ic.ArtifactStore.List(ic.SessionID)
}

// SearchMemory queries the MemoryStore for relevant content.
func (ic *RunContext) SearchMemory(q string, limit int) ([]SearchResult, error) {
	if ic.MemoryStore == nil {
		return []SearchResult{}, nil
	}

	return ic.MemoryStore.Search(ic.SessionID, q, limit)
}

// StoreMemory appends content plus metadata to the MemoryStore.
func (ic *RunContext) StoreMemory(content string, md map[string]any) error {
	if ic.MemoryStore == nil {
		return fmt.Errorf("memory store not configured")
	}
	return ic.MemoryStore.Store(ic.SessionID, content, md)
}

// RefreshSession reloads the session snapshot from the SessionStore.
func (ic *RunContext) RefreshSession() error {
	if ic.SessionStore == nil {
		return fmt.Errorf("session store not configured")
	}

	s, err := ic.SessionStore.Get(ic.SessionID)
	if err != nil {
		return err
	}

	ic.Session = s

	return nil
}

// CommitStateDelta persists the accumulated StateDelta then clears the buffer.
func (ic *RunContext) CommitStateDelta() error {
	if len(ic.StateDelta) == 0 {
		return nil
	}

	if ic.SessionStore == nil {
		return fmt.Errorf("session store not configured")
	}

	if err := ic.SessionStore.ApplyDelta(ic.SessionID, ic.StateDelta); err != nil {
		return err
	}

	ic.StateDelta = map[string]any{}

	return nil
}

// GetSessionHistory returns all historical events for the session.
func (ic *RunContext) GetSessionHistory() []Event {
	if ic.Session == nil {
		return []Event{}
	}

	return ic.Session.GetEvents()
}

// GetAgentName returns the logical agent name for this invocation.
func (ic *RunContext) GetAgentName() string { return ic.Agent.Name }

// GetAgentType returns a categorization label for the agent.
func (ic *RunContext) GetAgentType() string { return ic.Agent.Type }

// Clone returns a shallow copy with deep-copied delta & artifact slices.
func (ic *RunContext) Clone() *RunContext {
	c := &RunContext{
		Context:       ic.Context,
		SessionID:     ic.SessionID,
		RunID:         ic.RunID,
		Agent:         ic.Agent,
		UserContent:   ic.UserContent,
		Emit:          ic.Emit,
		Resume:        ic.Resume,
		SessionStore:  ic.SessionStore,
		ArtifactStore: ic.ArtifactStore,
		MemoryStore:   ic.MemoryStore,
		Limiter:       ic.Limiter,
		Session:       ic.Session,
		StateDelta:    map[string]any{},
		Artifacts:     []string{},
		Branch:        ic.Branch,
		loggerAdapter: ic.loggerAdapter,
	}

	maps.Copy(c.StateDelta, ic.StateDelta)

	c.Artifacts = append(c.Artifacts, ic.Artifacts...)

	return c
}

// WithBranch clones the context and sets the Branch label.
func (ic *RunContext) WithBranch(b string) *RunContext {
	c := ic.Clone()
	c.Branch = b
	return c
}

// NewChildContext derives a context for a nested / child execution path.
func (ic *RunContext) NewChildContext(emit chan<- Event, resume <-chan struct{}, branch string) *RunContext {
	finalBranch := ic.Branch
	if branch != "" {
		finalBranch = branch
	}

	return &RunContext{
		Context:       ic.Context,
		SessionID:     ic.SessionID,
		RunID:         ic.RunID,
		Agent:         ic.Agent,
		UserContent:   ic.UserContent,
		Emit:          emit,
		Resume:        resume,
		SessionStore:  ic.SessionStore,
		ArtifactStore: ic.ArtifactStore,
		MemoryStore:   ic.MemoryStore,
		Limiter:       ic.Limiter,
		Session:       ic.Session,
		StateDelta:    map[string]any{}, // fresh buffers
		Artifacts:     []string{},
		Branch:        finalBranch,
		loggerAdapter: ic.loggerAdapter,
	}
}

// EmitEvent merges pending StateDelta / Artifacts into the event and emits it.
func (ic *RunContext) EmitEvent(ev Event) error {
	if len(ic.StateDelta) > 0 {
		if ev.Actions.StateDelta == nil {
			invMap := map[string]any{}
			for k, v := range ic.StateDelta {
				invMap[k] = v
			}
			ev.Actions.StateDelta = invMap
		} else {
			for k, v := range ic.StateDelta {
				ev.Actions.StateDelta[k] = v
			}
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

// WaitForResume blocks until Resume signals or context cancellation.
func (ic *RunContext) WaitForResume() error {
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
