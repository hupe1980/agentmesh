package core

import "context"

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

	// PluginManager
	RunOnUserParts(ctx context.Context, userParts []Part) ([]Part, error)
	RunBeforeAgent(ctx context.Context, agent Agent) ([]Part, error)
	RunAfterAgent(ctx context.Context, agent Agent) ([]Part, error)
	RunOnEvent(ctx context.Context, event *Event) (*Event, error)
	RunBeforeRun(ctx context.Context) ([]Part, error)
	RunAfterRun(ctx context.Context) error
	RunOnToolError(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string, err error) (any, error)
	RunBeforeTool(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string) (any, error)
	RunAfterTool(ctx context.Context, tool Tool, toolCtx ToolContext, toolArgs string, result any) (any, error)
	RunBeforeModel(ctx context.Context, req *ModelRequest) (*ModelResponse, error)
	RunAfterModel(ctx context.Context, res *ModelResponse) (*ModelResponse, error)
	RunOnModelError(ctx context.Context, req *ModelRequest, err error) (*ModelResponse, error)

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

// ToolContext extends RequestContext with tool orchestration abilities.
type ToolContext interface {
	ReadonlyContext

	// Artifact access
	ArtifactReader
	ArtifactWriter

	// Memory access
	MemoryReader

	TransferRequester
	Escalator
	SummarizationSkipper

	// FunctionCallID returns the ID of the current function call, if available.
	FunctionCallID() (string, bool)

	// EventActions returns a pointer to the mutable EventActions accumulated
	EventActions() *EventActions
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

// StateSnapshot returns a merged, read-only view of session state with staged
// delta applied (delta overrides persisted values). A nil map indicates no state.
func (rc *requestContext) StateSnapshot() map[string]any {
	return rc.session.StateSnapshot()
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

// RunOnUserParts executes the OnUserParts hook across all plugins in order.
func (rc *requestContext) RunOnUserParts(ctx context.Context, userParts []Part) ([]Part, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunOnUserParts(ctx, rc, userParts)
}

// RunOnEvent executes the OnEvent hook chain allowing mutation of events.
func (rc *requestContext) RunOnEvent(ctx context.Context, event *Event) (*Event, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunOnEvent(ctx, rc, event)
}

// RunBeforeAgent executes BeforeAgent hooks.
func (rc *requestContext) RunBeforeAgent(ctx context.Context, agent Agent) ([]Part, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunBeforeAgent(ctx, rc, agent)
}

// RunAfterAgent executes AfterAgent hooks.
func (rc *requestContext) RunAfterAgent(ctx context.Context, agent Agent) ([]Part, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunAfterAgent(ctx, rc, agent)
}

// RunBeforeRun executes the BeforeRun hook across all plugins in order.
func (rc *requestContext) RunBeforeRun(ctx context.Context) ([]Part, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunBeforeRun(ctx, rc)
}

// RunAfterRun executes the AfterRun hook across all plugins in order.
func (rc *requestContext) RunAfterRun(ctx context.Context) error {
	if rc.pluginManager == nil {
		return nil
	}
	return rc.pluginManager.RunAfterRun(ctx, rc)
}

// RunOnToolError executes the OnToolError hook across all plugins in order.
func (rc *requestContext) RunOnToolError(
	ctx context.Context,
	tool Tool,
	toolCtx ToolContext,
	toolArgs string,
	err error,
) (any, error) {
	if rc.pluginManager == nil {
		return nil, err
	}
	return rc.pluginManager.RunOnToolError(ctx, tool, toolCtx, toolArgs, err)
}

// RunBeforeTool executes the BeforeTool hook across all plugins in order.
func (rc *requestContext) RunBeforeTool(
	ctx context.Context,
	tool Tool,
	toolCtx ToolContext,
	toolArgs string,
) (any, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunBeforeTool(ctx, tool, toolCtx, toolArgs)
}

// RunAfterTool executes the AfterTool hook across all plugins in order.
func (rc *requestContext) RunAfterTool(
	ctx context.Context,
	tool Tool,
	toolCtx ToolContext,
	toolArgs string,
	result any,
) (any, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunAfterTool(ctx, tool, toolCtx, toolArgs, result)
}

// RunBeforeModel executes the BeforeModel hook chain.
func (rc *requestContext) RunBeforeModel(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunBeforeModel(ctx, rc, req)
}

// RunAfterModel executes the AfterModel hook chain.
func (rc *requestContext) RunAfterModel(ctx context.Context, res *ModelResponse) (*ModelResponse, error) {
	if rc.pluginManager == nil {
		return nil, nil
	}
	return rc.pluginManager.RunAfterModel(ctx, rc, res)
}

// RunOnModelError executes the OnModelError hook chain for model invocation errors.
func (rc *requestContext) RunOnModelError(ctx context.Context, req *ModelRequest, err error) (*ModelResponse, error) {
	if rc.pluginManager == nil {
		return nil, err
	}
	return rc.pluginManager.RunOnModelError(ctx, rc, req, err)
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
		limiter:       rc.limiter,
		session:       rc.session,
		branch:        rc.branch,
	}

	c.branch = branchName

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

// ToolContext provides a constrained, auditable surface for tool/function execution.
// It accumulates EventActions (state deltas, transfers, escalation signals, artifact diffs)
// without directly mutating the underlying session until applied.

type toolContext struct {
	RequestContext

	functionCallID Opt[string]
	eventActions   EventActions
}

// ToolContextOptions groups options for constructing a ToolContext.
type ToolContextOptions struct {
	// FunctionCallID is the ID of the function call being processed.
	FunctionCallID Opt[string]

	// EventActions accumulates actions requested during tool execution.
	EventActions EventActions
}

// NewToolContext constructs a tool context bound to a parent RequestContext
// and the provided FunctionCallID.
func NewToolContext(reqCtx RequestContext, optFns ...func(o *ToolContextOptions)) ToolContext {
	opts := &ToolContextOptions{
		EventActions: EventActions{},
	}

	for _, fn := range optFns {
		fn(opts)
	}

	return &toolContext{
		RequestContext: reqCtx,
		functionCallID: opts.FunctionCallID,
		eventActions:   opts.EventActions,
	}
}

// FunctionCallID returns the function call ID associated with the tool invocation,
// and a boolean indicating whether it was set.
func (tc *toolContext) FunctionCallID() (string, bool) { return tc.functionCallID.Get() }

// EventActions returns the event actions accumulated in the tool context.
func (tc *toolContext) EventActions() *EventActions { return &tc.eventActions }

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
	_ ToolContext       = (*toolContext)(nil)
)
