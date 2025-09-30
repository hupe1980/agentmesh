package core

import (
	"context"
	"maps"
)

// StateSnapshotter exposes a read-only snapshot of session state.
type StateSnapshotter interface {
	StateSnapshot() map[string]any
}

// ArtifactReader provides read access to artifacts scoped to the session.
type ArtifactReader interface {
	LoadArtifact(ctx context.Context, fileName string) (Part, error)
	ListArtifactKeys(ctx context.Context) ([]string, error)
}

// ArtifactWriter provides write access to artifacts scoped to the session.
type ArtifactWriter interface {
	SaveArtifact(ctx context.Context, fileName string, artifact Part) error
	DeleteArtifact(ctx context.Context, fileName string) error
}

// MemoryReader provides read access to the memory store.
type MemoryReader interface {
	SearchMemory(ctx context.Context, q string) (*SearchResult, error)
}

// MemoryWriter provides write access to the memory store.
type MemoryWriter interface {
	AddSessionToMemory(ctx context.Context, session Session) error
}

// SessionReader exposes access to the session's event history.
type SessionReader interface {
	GetSessionHistory() []*Event
}

// ReadonlyContext aggregates read-only capabilities and identity.
type ReadonlyContext interface {
	// Identity
	AppName() string
	UserID() string
	SessionID() string
	RunID() string
	AgentName() string

	// State access
	StateSnapshotter
}

// RequestContext aggregates capabilities for an agent invocation.
// It carries identities and access to stores. It does not embed context.Context.
// Cancellation and deadlines are passed explicitly in method parameters.
type RequestContext interface {
	ReadonlyContext

	// Artifact access
	ArtifactReader
	ArtifactWriter

	// Memory access
	MemoryReader
	MemoryWriter

	// Session history
	SessionReader

	// PluginManager access
	PluginManager() PluginManager

	// Branching
	NewBranchContextForSubAgent(branchName string) RequestContext
	Branch() string

	// Model call tracking
	IncrementModelCalls() error
}

// CallbackContext provides read-only access to invocation-scoped data for callback hooks.
//
// Implementations must satisfy ReadonlyContext. Use this in Callback methods
// to observe request metadata, state, logging, and history without mutation.
type CallbackContext interface {
	ReadonlyContext

	State() *State

	ArtifactReader
	ArtifactWriter

	// EventActions returns a pointer to the mutable EventActions accumulated.
	EventActions() *EventActions
}

// TransferRequester allows requesting control transfer to another agent.
type TransferRequester interface {
	TransferToAgent(name string)
}

// Escalator allows requesting escalation to a higher-order agent/human.
type Escalator interface {
	Escalate()
}

// SummarizationSkipper allows requesting that post-processing summarization be skipped.
type SummarizationSkipper interface {
	SkipSummarization()
}

// ToolContext extends CallbackContext with tool orchestration abilities.
type ToolContext interface {
	// Inherit identity/state/artifacts/actions
	CallbackContext

	// Memory access convenience
	MemoryReader

	// PluginManager access
	PluginManager() PluginManager

	TransferRequester
	Escalator
	SummarizationSkipper

	// FunctionCallID returns the ID of the current function call, if available.
	FunctionCallID() (string, bool)

	// Actions returns a pointer to the mutable EventActions accumulated (alias of EventActions).
	Actions() *EventActions
}

type requestContext struct {
	runID         string
	agent         AgentIdentity
	userParts     []Part
	sessionStore  SessionStore
	artifactStore ArtifactStore
	memoryStore   MemoryStore
	pluginManager PluginManager
	limiter       *ModelLimiter
	session       *Session
	branch        string
}

// CloneRequestContextWithAgent returns a shallow clone of the provided RequestContext
// with AgentIdentity replaced. Internal pointers (session, stores, limiter) are shared.
// If the underlying implementation is unknown, the original context is returned.
func CloneRequestContextWithAgent(rc RequestContext, agent AgentIdentity) RequestContext {
	if impl, ok := rc.(*requestContext); ok {
		clone := *impl
		clone.agent = agent
		return &clone
	}

	return rc
}

// RequestContextParams groups the inputs required to construct a RequestContext.
// Using a struct improves readability and makes call sites less error-prone.
type RequestContextParams struct {
	RunID         string
	Agent         AgentIdentity
	UserParts     []Part
	MaxModelCalls int
	Session       *Session
	SessionStore  SessionStore
	ArtifactStore ArtifactStore
	MemoryStore   MemoryStore
	PluginManager PluginManager
}

// NewRequestContext constructs a RequestContext with the provided parameters.
func NewRequestContext(p RequestContextParams) RequestContext {
	return &requestContext{
		runID:         p.RunID,
		agent:         p.Agent,
		userParts:     p.UserParts,
		session:       p.Session,
		sessionStore:  p.SessionStore,
		artifactStore: p.ArtifactStore,
		memoryStore:   p.MemoryStore,
		pluginManager: p.PluginManager,
		limiter:       NewModelLimiter(p.MaxModelCalls),
	}
}

// Identity accessors
func (rc *requestContext) AppName() string   { return rc.session.AppName() }
func (rc *requestContext) UserID() string    { return rc.session.UserID() }
func (rc *requestContext) SessionID() string { return rc.session.ID() }
func (rc *requestContext) RunID() string     { return rc.runID }
func (rc *requestContext) AgentName() string { return rc.agent.Name() }

// StateSnapshot returns a read-only copy of the session state map.
func (rc *requestContext) StateSnapshot() map[string]any {
	src := rc.session.State()
	snapshot := make(map[string]any, len(src))
	maps.Copy(snapshot, src)
	return snapshot
}

// SaveArtifact stores bytes in the ArtifactStore and stages the id for the next emitted event.
func (rc *requestContext) SaveArtifact(ctx context.Context, fileName string, artifact Part) error {
	return rc.artifactStore.Save(ctx, rc.AppName(), rc.UserID(), rc.SessionID(), fileName, artifact)
}

// DeleteArtifact removes an artifact from the ArtifactStore.
func (rc *requestContext) DeleteArtifact(ctx context.Context, fileName string) error {
	return rc.artifactStore.Delete(ctx, rc.AppName(), rc.UserID(), rc.SessionID(), fileName)
}

// LoadArtifact retrieves previously saved artifact bytes.
func (rc *requestContext) LoadArtifact(ctx context.Context, fileName string) (Part, error) {
	return rc.artifactStore.Load(ctx, rc.AppName(), rc.UserID(), rc.SessionID(), fileName)
}

// ListArtifacts returns artifact IDs stored for the session.
func (rc *requestContext) ListArtifactKeys(ctx context.Context) ([]string, error) {
	return rc.artifactStore.ListKeys(ctx, rc.AppName(), rc.UserID(), rc.SessionID())
}

// PluginManager returns the underlying PluginManager for direct hook execution.
func (rc *requestContext) PluginManager() PluginManager { return rc.pluginManager }

// SearchMemory queries the MemoryStore for relevant content.
func (rc *requestContext) SearchMemory(ctx context.Context, q string) (*SearchResult, error) {
	return rc.memoryStore.Search(ctx, rc.AppName(), rc.UserID(), q)
}

// AddSessionToMemory stores the session information in the MemoryStore.
func (rc *requestContext) AddSessionToMemory(ctx context.Context, session Session) error {
	return rc.memoryStore.AddSession(ctx, &session)
}

// GetSessionHistory returns all historical events for the session.
func (rc *requestContext) GetSessionHistory() []*Event {
	return rc.session.Events()
}

// NewBranchContextForSubAgent creates a branched RequestContext for a sub-agent.
// The branch shares the underlying session and stores but carries a distinct branch name.
func (rc *requestContext) NewBranchContextForSubAgent(branchName string) RequestContext {
	c := &requestContext{
		runID:         rc.runID,
		agent:         rc.agent,
		userParts:     rc.userParts,
		sessionStore:  rc.sessionStore,
		artifactStore: rc.artifactStore,
		memoryStore:   rc.memoryStore,
		pluginManager: rc.pluginManager, // <- copy plugin manager
		limiter:       rc.limiter,
		session:       rc.session,
		branch:        branchName, // set directly
	}
	return c
}

// Branch returns the branch name for the current context.
func (rc *requestContext) Branch() string {
	return rc.branch
}

// IncrementModelCalls increments the internal model call counter and enforces the max.
func (rc *requestContext) IncrementModelCalls() error {
	return rc.limiter.Increment()
}

// Compile-time assertions: requestContext satisfies capability interfaces
var (
	_ StateSnapshotter = (*requestContext)(nil)
	_ ArtifactReader   = (*requestContext)(nil)
	_ ArtifactWriter   = (*requestContext)(nil)
	_ MemoryReader     = (*requestContext)(nil)
	_ MemoryWriter     = (*requestContext)(nil)
	_ SessionReader    = (*requestContext)(nil)
	_ ReadonlyContext  = (*requestContext)(nil)
	_ RequestContext   = (*requestContext)(nil)
)

// callbackContext implements CallbackContext by layering a delta-aware State()
// over the session snapshot and delegating artifact access to the underlying
// RequestContext. Any state mutations are applied to the provided delta map,
// allowing the orchestration layer to capture and commit them later.
type callbackContext struct {
	RequestContext

	state        *State
	eventActions *EventActions
}

// CallbackContextOptions configures construction of a callbackContext.
// If StateDelta is provided, it will be used as the backing delta for State(),
// enabling mutation capture without touching the session directly.
type CallbackContextOptions struct {
	// EventActions accumulates actions (e.g., state/artifact diffs) requested during callbacks.
	EventActions *EventActions

	// StateDelta allows wiring a specific delta map (e.g., from EventActions) so that
	// ctx.State().Set(...) records into that map.
	StateDelta map[string]any
}

// NewCallbackContext constructs a CallbackContext bound to a parent RequestContext.
// State() returns a delta-aware view backed by StateDelta if provided; otherwise, an empty delta.
func NewCallbackContext(reqCtx RequestContext, optFns ...func(o *CallbackContextOptions)) CallbackContext {
	opts := &CallbackContextOptions{}

	for _, fn := range optFns {
		fn(opts)
	}

	// Ensure EventActions is non-nil after options.
	if opts.EventActions == nil {
		opts.EventActions = &EventActions{}
	}

	// Build a delta-aware State that overlays the session snapshot with the provided delta.
	value := reqCtx.StateSnapshot()

	// Ensure StateDelta is non-nil after options.
	if opts.StateDelta == nil {
		opts.StateDelta = make(map[string]any)
	}

	return &callbackContext{
		RequestContext: reqCtx,
		state:          NewState(value, opts.StateDelta),
		eventActions:   opts.EventActions,
	}
}

// State returns the delta-aware state for the current session.
func (cc *callbackContext) State() *State { return cc.state }

// EventActions returns the event actions accumulated in the callback context.
func (cc *callbackContext) EventActions() *EventActions { return cc.eventActions }

// Compile-time assertions: callbackContext satisfies capability interfaces
var (
	_ ReadonlyContext = (*callbackContext)(nil)
	_ ArtifactReader  = (*callbackContext)(nil) // via embedded RequestContext
	_ ArtifactWriter  = (*callbackContext)(nil) // via embedded RequestContext
	_ CallbackContext = (*callbackContext)(nil)
)

type toolContext struct {
	// Embed callbackContext to provide delta-aware state, artifact access, and actions accumulation.
	*callbackContext

	functionCallID Opt[string]
}

// ToolContextOptions groups options for constructing a ToolContext.
type ToolContextOptions struct {
	// FunctionCallID is the ID of the function call being processed.
	FunctionCallID Opt[string]

	// EventActions accumulates actions requested during tool execution.
	EventActions *EventActions

	// StateDelta allows wiring a specific delta map
	StateDelta map[string]any
}

// NewToolContext constructs a tool context bound to a parent RequestContext
// and the provided FunctionCallID.
func NewToolContext(reqCtx RequestContext, optFns ...func(o *ToolContextOptions)) ToolContext {
	opts := &ToolContextOptions{}

	for _, fn := range optFns {
		fn(opts)
	}

	// Ensure EventActions is non-nil after options.
	if opts.EventActions == nil {
		opts.EventActions = &EventActions{}
	}

	// Ensure StateDelta is non-nil after options.
	if opts.StateDelta == nil {
		opts.StateDelta = make(map[string]any)
	}

	value := reqCtx.StateSnapshot()

	cb := &callbackContext{
		RequestContext: reqCtx,
		state:          NewState(value, opts.StateDelta),
		eventActions:   opts.EventActions,
	}

	return &toolContext{
		callbackContext: cb,
		functionCallID:  opts.FunctionCallID,
	}
}

// FunctionCallID returns the function call ID associated with the tool invocation,
// and a boolean indicating whether it was set.
func (tc *toolContext) FunctionCallID() (string, bool) { return tc.functionCallID.Get() }

// Actions returns the event actions accumulated in the tool context (alias of EventActions).
func (tc *toolContext) Actions() *EventActions { return tc.EventActions() }

// PluginManager exposes the underlying PluginManager from the embedded RequestContext.
func (tc *toolContext) PluginManager() PluginManager { return tc.callbackContext.PluginManager() }

// SkipSummarization requests that post-processing summarization be bypassed
// for the originating event.
func (tc *toolContext) SkipSummarization() {
	tc.eventActions.SkipSummarization = Bool(true)
}

// TransferToAgent signals orchestration to handoff control to another agent.
func (tc *toolContext) TransferToAgent(name string) {
	tc.eventActions.TransferToAgent = String(name)
}

// Escalate requests escalation (e.g., to a higher-skill agent or human).
func (tc *toolContext) Escalate() {
	tc.eventActions.Escalate = Bool(true)
}

// Compile-time assertions: ToolContext satisfies capability interfaces
var (
	_ StateSnapshotter  = (*toolContext)(nil)
	_ ArtifactReader    = (*toolContext)(nil)
	_ ArtifactWriter    = (*toolContext)(nil)
	_ MemoryReader      = (*toolContext)(nil)
	_ TransferRequester = (*toolContext)(nil)
	_ Escalator         = (*toolContext)(nil)
	_ ReadonlyContext   = (*toolContext)(nil)
	_ CallbackContext   = (*toolContext)(nil)
	_ ToolContext       = (*toolContext)(nil)
)
