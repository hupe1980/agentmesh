package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/artifact"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/memory"
	"github.com/hupe1980/agentmesh/session"
)

// Config captures core runtime tuning knobs for the Engine.
// Only fundamental dimensions (concurrency + buffering) are exposed here;
// cross‑cutting concerns (timeouts, metrics, tracing) belong in higher‑level
// wrappers or decorators to keep the core minimal.
type Config struct {
	// MaxConcurrentInvocations limits the number of agent invocations that
	// can execute simultaneously. This prevents resource exhaustion and
	// provides backpressure. Set to 0 for unlimited (not recommended).
	MaxConcurrentInvocations int

	// EnableStreaming determines whether events are streamed in real-time
	// or buffered until completion. Streaming enables interactive experiences
	// but may increase overhead for simple request-response patterns.
	EnableStreaming bool

	// EventBufferSize sets the channel buffer size for event processing.
	// Larger buffers reduce blocking but increase memory usage. Should be
	// tuned based on expected event volume and processing latency.
	EventBufferSize int
}

// DefaultConfig provides conservative, generally safe defaults suitable for
// development and small/medium production loads. Tune based on throughput and
// event volume characteristics of your deployment.
var DefaultConfig = Config{
	MaxConcurrentInvocations: 10,
	EnableStreaming:          true,
	EventBufferSize:          100,
}

// Options holds dependency + configuration overrides passed to New(). Missing
// services fall back to in‑memory implementations; Logger defaults to no‑op.
type Options struct {
	// Config contains operational parameters for the engine behavior.
	// Defaults to DefaultConfig if not specified.
	Config Config

	// Service Dependencies - all have in-memory defaults for development

	// SessionStore manages session persistence and state.
	// Defaults to in-memory implementation if not provided.
	SessionStore core.SessionStore

	// ArtifactStore handles binary/blob artifact storage.
	// Defaults to in-memory implementation if not provided.
	ArtifactStore core.ArtifactStore

	// MemoryStore provides searchable memory and recall capabilities.
	// Defaults to in-memory implementation if not provided.
	MemoryStore core.MemoryStore

	// Logger provides structured logging for debugging and monitoring.
	// Defaults to NoOp logger if nil to ensure no logging dependencies.
	Logger logging.Logger
}

// Engine coordinates agent execution: it resolves agents, creates invocation
// contexts, streams events, applies side‑effects, and persists history. Public
// methods are safe for concurrent use. The design keeps orchestration separate
// from agent logic. Future layers (metrics, policies) can wrap the core.
type Engine struct {
	// Core stores - immutable after construction
	sessionStore  core.SessionStore  // Session persistence and state management
	artifactStore core.ArtifactStore // Binary/blob artifact storage
	memoryStore   core.MemoryStore   // Searchable memory and recall storage
	logger        logging.Logger     // Structured logging interface

	// Configuration - immutable after construction
	config Config // Operational parameters (concurrency, buffering, etc.)

	// Agent registry - protected by mutex for thread-safe access
	agents map[string]core.Agent // Registered agents by name
	mu     sync.RWMutex          // Protects agents map

	// Active invocation tracking - protected by separate mutex
	activeInvocations map[string]context.CancelFunc // Cancellation functions by invocation ID
	invocationsMu     sync.RWMutex                  // Protects activeInvocations map
}

// New constructs an Engine. All dependencies optional (defaults are in‑memory
// implementations + no‑op logger). Use functional options to override.
// Returned instance is ready and thread‑safe.
func New(
	optFns ...func(o *Options),
) *Engine {
	opts := Options{
		Config:        DefaultConfig,
		SessionStore:  session.NewInMemoryStore(),
		ArtifactStore: artifact.NewInMemoryStore(),
		MemoryStore:   memory.NewInMemoryStore(),
		Logger:        logging.NoOpLogger{},
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	// Initialize engine with required services and default configuration
	return &Engine{
		// Core services - required and immutable after construction
		sessionStore:  opts.SessionStore,
		artifactStore: opts.ArtifactStore,
		memoryStore:   opts.MemoryStore,

		// Default configuration - may be overridden by options
		config: opts.Config,

		// Runtime state - initialized empty
		agents:            make(map[string]core.Agent),
		activeInvocations: make(map[string]context.CancelFunc),

		// Logger - may be overridden by options
		logger: opts.Logger,
	}
}

// Register makes / replaces an agent under its Name(). Safe for concurrent
// use; prefer registering all agents during startup to avoid mid‑flight swaps.
func (e *Engine) Register(a core.Agent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agents[a.Name()] = a
}

// GetAgent returns the registered agent and a bool indicating presence.
func (e *Engine) GetAgent(name string) (core.Agent, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a, ok := e.agents[name]
	return a, ok
}

// Invoke starts an asynchronous invocation and returns streaming channels.
// eventsCh is closed on completion; errorsCh yields a single terminal error
// (if any). Returned invocationID can be used with StopInvocation().
func (e *Engine) Invoke(
	ctx context.Context,
	sessionID string,
	agentName string,
	userContent core.Content,
) (string, <-chan core.Event, <-chan error, error) {
	// Validate agent exists
	agent, ok := e.GetAgent(agentName)
	if !ok {
		return "", nil, nil, fmt.Errorf("agent %s not found", agentName)
	}

	// Retrieve or create session
	session, err := e.sessionStore.Get(sessionID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Generate unique invocation identifier
	invocationID := uuid.NewString()

	// Create channels for event streaming and error reporting
	eventsCh := make(chan core.Event, e.config.EventBufferSize)
	errorsCh := make(chan error, 1)
	agentEmit := make(chan core.Event, e.config.EventBufferSize)
	resumeCh := make(chan struct{}, 1)

	// Create cancellable context for this invocation
	invocationCtx, cancel := context.WithCancel(ctx)

	// Track active invocation for management and cleanup
	e.invocationsMu.Lock()
	e.activeInvocations[invocationID] = cancel
	e.invocationsMu.Unlock()

	// Create invocation context with all necessary services and metadata
	agentInfo := core.AgentInfo{Name: agent.Name(), Type: "unknown"}

	invCtx := core.NewInvocationContext(
		invocationCtx,
		sessionID,
		invocationID,
		agentInfo,
		userContent,
		agentEmit,
		resumeCh,
		session,
		e.sessionStore,
		e.artifactStore,
		e.memoryStore,
		e.logger,
	)

	// Persist user input as the starting event for this invocation
	userEvent := core.NewUserContentEvent(invocationID, &userContent)

	if err := e.sessionStore.AppendEvent(sessionID, userEvent); err != nil {
		return "", nil, nil, fmt.Errorf("failed to append user event: %w", err)
	}

	// Start agent execution in a separate goroutine
	go func() {
		// Unified defer handles agent stop and invocation cleanup
		defer func() {
			close(agentEmit)
			e.invocationsMu.Lock()
			delete(e.activeInvocations, invocationID)
			e.invocationsMu.Unlock()
		}()

		if err := e.runAgent(invCtx, agent); err != nil {
			select {
			case <-invocationCtx.Done():
				return
			case errorsCh <- fmt.Errorf("agent execution failed: %w", err):
			}
		}
	}()

	// Start event processing in a separate goroutine
	go func() {
		defer func() {
			// Ensure channels are closed when processing completes
			close(eventsCh)
			close(errorsCh)
		}()

		// Process events from agent and forward to client
		e.processEvents(invocationCtx, sessionID, agentEmit, resumeCh, eventsCh, errorsCh)
	}()

	return invocationID, eventsCh, errorsCh, nil
}

// StopInvocation cancels a running invocation by ID. Safe to call multiple
// times; calling after completion is a no‑op with an error return.
func (e *Engine) StopInvocation(invocationID string) error {
	e.invocationsMu.Lock()
	cancel, exists := e.activeInvocations[invocationID]
	e.invocationsMu.Unlock()

	if !exists {
		return fmt.Errorf("invocation %s not found", invocationID)
	}

	cancel()
	return nil
}

func (e *Engine) runAgent(invocationCtx *core.InvocationContext, agent core.Agent) error {
	if err := agent.Start(invocationCtx); err != nil {
		return err
	}
	defer func() {
		if err := agent.Stop(invocationCtx); err != nil {
			e.logger.Warn("error stopping agent %s: %v", agent.Name(), err)
		}
	}()

	return agent.Run(invocationCtx)
}

// processEvents pulls agent events, applies side‑effects, persists, forwards
// them, and emits resume signals for non‑partial events. Exits on channel close
// or context cancellation.
func (e *Engine) processEvents(
	ctx context.Context,
	sessionID string,
	agentEmit <-chan core.Event,
	resumeCh chan<- struct{},
	eventsCh chan<- core.Event,
	errorsCh chan<- error,
) {
	for {
		select {
		case <-ctx.Done():
			// Context cancelled - terminate processing
			return

		case ev, ok := <-agentEmit:
			if !ok {
				// Agent closed emit channel - normal completion
				return
			}

			// Apply event actions to underlying services
			if err := e.applyEventActions(sessionID, ev); err != nil {
				select {
				case <-ctx.Done():
					return
				case errorsCh <- fmt.Errorf("failed to process event actions: %w", err):
				}
				return
			}

			// Persist non-partial events to session history
			if !ev.IsPartial() {
				if err := e.sessionStore.AppendEvent(sessionID, ev); err != nil {
					select {
					case <-ctx.Done():
						return
					case errorsCh <- fmt.Errorf("failed to append event to session: %w", err):
					}
					return
				}
			}

			// Forward event to client
			select {
			case <-ctx.Done():
				return
			case eventsCh <- ev:
				e.logger.Debug("engine delivered event event_id=%s session_id=%s", ev.ID, sessionID)
			}

			// Signal resumption for non-partial events
			if !ev.IsPartial() {
				select {
				case <-ctx.Done():
					return
				case resumeCh <- struct{}{}:
					// Resumption signal sent
				default:
					// Channel full, skip signal (non-blocking)
				}
			}
		}
	}
}

// applyEventActions applies state / artifact / routing side‑effects embedded in
// an event. Currently transfer + escalate are logged only (future orchestration).
func (e *Engine) applyEventActions(sessionID string, ev core.Event) error {
	// Apply session state changes
	if len(ev.Actions.StateDelta) > 0 {
		if err := e.sessionStore.ApplyDelta(sessionID, ev.Actions.StateDelta); err != nil {
			return fmt.Errorf("failed to apply state delta: %w", err)
		}
	}

	// Apply artifact changes (reserved for future implementation)
	if len(ev.Actions.ArtifactDelta) > 0 {
		// Currently unused - reserved for future artifact lifecycle management
		// This might include operations like:
		// - Marking artifacts as referenced or unreferenced
		// - Triggering cleanup of temporary artifacts
		// - Updating artifact metadata or permissions
		_ = e.artifactStore
	}

	// Handle agent transfer requests (reserved for future implementation)
	// TransferToAgent placeholder: log intent (actual orchestration TBD)
	if ev.Actions.TransferToAgent != nil && *ev.Actions.TransferToAgent != "" {
		// Future: validate target agent exists, queue handoff
		e.logger.Debug("engine.event.transfer_to_agent target=%s session_id=%s", *ev.Actions.TransferToAgent, sessionID)
	}

	// Handle escalation requests (reserved for future implementation)
	// Escalate placeholder: log intent (actual escalation workflow TBD)
	if ev.Actions.Escalate != nil && *ev.Actions.Escalate {
		e.logger.Debug("engine.event.escalate session_id=%s", sessionID)
	}

	return nil
}

// GetSession returns a point‑in‑time snapshot of a session (for debugging /
// inspection). Writes still go through invocation flows.
func (e *Engine) GetSession(sessionID string) (*core.Session, error) {
	return e.sessionStore.Get(sessionID)
}
