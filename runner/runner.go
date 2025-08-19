package runner

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

// Config captures core runtime tuning knobs for the Runner.
type Config struct {
	// MaxConcurrentInvocations limits concurrent agent invocations.
	MaxConcurrentInvocations int
	// EnableStreaming toggles real-time event streaming vs buffered.
	EnableStreaming bool
	// EventBufferSize sets channel buffering for events.
	EventBufferSize int
}

// DefaultConfig provides conservative defaults.
var DefaultConfig = Config{
	MaxConcurrentInvocations: 10,
	EnableStreaming:          true,
	EventBufferSize:          100,
}

// Options holds dependency + configuration overrides passed to New().
type Options struct {
	Config        Config
	SessionStore  core.SessionStore
	ArtifactStore core.ArtifactStore
	MemoryStore   core.MemoryStore
	Logger        logging.Logger
}

// Runner coordinates agent execution: resolves the root agent, creates
// invocation contexts, streams events, applies side‑effects, and persists
// history. Public methods are safe for concurrent use.
type Runner struct {
	agent core.Agent

	sessionStore  core.SessionStore
	artifactStore core.ArtifactStore
	memoryStore   core.MemoryStore
	logger        logging.Logger

	config Config

	mu sync.RWMutex

	activeInvocations map[string]context.CancelFunc
	invocationsMu     sync.RWMutex
}

// New constructs a Runner with optional overrides.
func New(agent core.Agent, optFns ...func(o *Options)) *Runner {
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
	return &Runner{
		agent:             agent,
		sessionStore:      opts.SessionStore,
		artifactStore:     opts.ArtifactStore,
		memoryStore:       opts.MemoryStore,
		logger:            opts.Logger,
		config:            opts.Config,
		activeInvocations: make(map[string]context.CancelFunc),
	}
}

// Run starts an asynchronous invocation.
func (r *Runner) Run(ctx context.Context, sessionID string, userContent core.Content) (string, <-chan core.Event, <-chan error, error) {
	session, err := r.sessionStore.Get(sessionID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to get session: %w", err)
	}

	invocationID := uuid.NewString()
	eventsCh := make(chan core.Event, r.config.EventBufferSize)
	errorsCh := make(chan error, 1)
	agentEmit := make(chan core.Event, r.config.EventBufferSize)
	resumeCh := make(chan struct{}, 1)

	invocationCtx, cancel := context.WithCancel(ctx)
	r.invocationsMu.Lock()
	r.activeInvocations[invocationID] = cancel
	r.invocationsMu.Unlock()

	agentInfo := core.AgentInfo{Name: r.agent.Name(), Type: "unknown"}

	invCtx := core.NewInvocationContext(
		invocationCtx,
		sessionID,
		invocationID,
		agentInfo,
		userContent,
		agentEmit,
		resumeCh,
		session,
		r.sessionStore,
		r.artifactStore,
		r.memoryStore,
		r.logger,
	)

	userEvent := core.NewUserContentEvent(invocationID, &userContent)
	if err := r.sessionStore.AppendEvent(sessionID, userEvent); err != nil {
		return "", nil, nil, fmt.Errorf("failed to append user event: %w", err)
	}

	go func() {
		defer func() {
			close(agentEmit)
			r.invocationsMu.Lock()
			delete(r.activeInvocations, invocationID)
			r.invocationsMu.Unlock()
		}()
		if err := r.runAgent(invCtx); err != nil {
			select {
			case <-invocationCtx.Done():
				return
			case errorsCh <- fmt.Errorf("agent execution failed: %w", err):
			}
		}
	}()

	go func() {
		defer func() { close(eventsCh); close(errorsCh) }()
		r.processEvents(invocationCtx, sessionID, agentEmit, resumeCh, eventsCh, errorsCh)
	}()

	return invocationID, eventsCh, errorsCh, nil
}

// Cancel cancels a running invocation by ID.
func (r *Runner) Cancel(invocationID string) error {
	r.invocationsMu.Lock()
	cancel, exists := r.activeInvocations[invocationID]
	r.invocationsMu.Unlock()
	if !exists {
		return fmt.Errorf("invocation %s not found", invocationID)
	}
	cancel()
	return nil
}

func (r *Runner) runAgent(invocationCtx *core.InvocationContext) error {
	if err := r.agent.Start(invocationCtx); err != nil {
		return err
	}
	defer func() {
		if err := r.agent.Stop(invocationCtx); err != nil {
			r.logger.Warn("error stopping agent %s: %v", r.agent.Name(), err)
		}
	}()
	return r.agent.Run(invocationCtx)
}

func (r *Runner) processEvents(
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
			return
		case ev, ok := <-agentEmit:
			if !ok {
				return
			}
			if err := r.applyEventActions(sessionID, ev); err != nil {
				select {
				case <-ctx.Done():
					return
				case errorsCh <- fmt.Errorf("failed to process event actions: %w", err):
				}
				return
			}
			if !ev.IsPartial() {
				if err := r.sessionStore.AppendEvent(sessionID, ev); err != nil {
					select {
					case <-ctx.Done():
						return
					case errorsCh <- fmt.Errorf("failed to append event to session: %w", err):
					}
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			case eventsCh <- ev:
				r.logger.Debug("runner delivered event event_id=%s session_id=%s", ev.ID, sessionID)
			}
			if !ev.IsPartial() {
				select {
				case <-ctx.Done():
					return
				case resumeCh <- struct{}{}:
				default:
				}
			}
		}
	}
}

func (r *Runner) applyEventActions(sessionID string, ev core.Event) error {
	if len(ev.Actions.StateDelta) > 0 {
		if err := r.sessionStore.ApplyDelta(sessionID, ev.Actions.StateDelta); err != nil {
			return fmt.Errorf("failed to apply state delta: %w", err)
		}
	}
	if len(ev.Actions.ArtifactDelta) > 0 {
		_ = r.artifactStore
	}
	if ev.Actions.TransferToAgent != nil && *ev.Actions.TransferToAgent != "" {
		r.logger.Debug("runner.event.transfer_to_agent target=%s session_id=%s", *ev.Actions.TransferToAgent, sessionID)
	}
	if ev.Actions.Escalate != nil && *ev.Actions.Escalate {
		r.logger.Debug("runner.event.escalate session_id=%s", sessionID)
	}
	return nil
}

// GetSession returns a snapshot of a session.
func (r *Runner) GetSession(sessionID string) (*core.Session, error) {
	return r.sessionStore.Get(sessionID)
}
