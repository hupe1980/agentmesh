package runner

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/artifact"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/util"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/memory"
	"github.com/hupe1980/agentmesh/session"
)

// Options holds dependency + configuration overrides passed to New().
type Options struct {
	// MaxConcurrentInvocations limits concurrent agent invocations.
	MaxConcurrentInvocations int
	// EnableStreaming toggles real-time event streaming vs buffered.
	EnableStreaming bool
	// EventBufferSize sets channel buffering for events.
	EventBufferSize int
	// Session management services.
	SessionStore core.SessionStore
	// Artifact management services.
	ArtifactStore core.ArtifactStore
	// Memory management services.
	MemoryStore core.MemoryStore
	// Logging services.
	Logger logging.Logger
}

// Runner coordinates agent execution: resolves the root agent, creates
// invocation contexts, streams events, applies side‑effects, and persists
// history. Public methods are safe for concurrent use.
type Runner struct {
	agent core.Agent

	maxConcurrentInvocations int
	enableStreaming          bool
	eventBufferSize          int

	sessionStore  core.SessionStore
	artifactStore core.ArtifactStore
	memoryStore   core.MemoryStore
	logger        logging.Logger

	activeRuns map[string]context.CancelFunc
	mu         sync.RWMutex
}

// New constructs a Runner with optional overrides.
func New(agent core.Agent, optFns ...func(o *Options)) *Runner {
	opts := Options{
		MaxConcurrentInvocations: 10,
		EnableStreaming:          true,
		EventBufferSize:          100,
		SessionStore:             session.NewInMemoryStore(),
		ArtifactStore:            artifact.NewInMemoryStore(),
		MemoryStore:              memory.NewInMemoryStore(),
		Logger:                   logging.NoOpLogger{},
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Runner{
		agent:                    agent,
		maxConcurrentInvocations: opts.MaxConcurrentInvocations,
		enableStreaming:          opts.EnableStreaming,
		eventBufferSize:          opts.EventBufferSize,
		sessionStore:             opts.SessionStore,
		artifactStore:            opts.ArtifactStore,
		memoryStore:              opts.MemoryStore,
		logger:                   opts.Logger,
		activeRuns:               make(map[string]context.CancelFunc),
	}
}

// RunOptions holds configuration options for a single run.
type RunOptions struct {
	// MaxModelCalls limits the number of model calls per run.
	MaxModelCalls int
	StateDelta    map[string]any
}

// Run starts an asynchronous invocation.
func (r *Runner) Run(
	ctx context.Context,
	sessionID string,
	userContent core.Content,
	optFns ...func(o *RunOptions),
) (string, <-chan core.Event, <-chan error, error) {
	opts := RunOptions{
		MaxModelCalls: 100,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	session, err := r.sessionStore.Get(sessionID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to get session: %w", err)
	}

	runID := util.NewID()

	eventsCh := make(chan core.Event, r.eventBufferSize)
	errorsCh := make(chan error, 1)
	agentEmit := make(chan core.Event, r.eventBufferSize)
	resumeCh := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.activeRuns[runID] = cancel
	r.mu.Unlock()

	agentInfo := core.AgentInfo{Name: r.agent.Name(), Type: "unknown"}

	runCtx := core.NewRunContext(
		ctx,
		sessionID,
		runID,
		agentInfo,
		userContent,
		opts.MaxModelCalls,
		agentEmit,
		resumeCh,
		session,
		r.sessionStore,
		r.artifactStore,
		r.memoryStore,
		r.logger,
	)

	userEvent := core.NewUserContentEvent(runID, &userContent, opts.StateDelta)
	if err := r.sessionStore.AppendEvent(sessionID, userEvent); err != nil {
		return "", nil, nil, fmt.Errorf("failed to append user event: %w", err)
	}

	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.activeRuns, runID)
			r.mu.Unlock()

			close(agentEmit)
		}()

		if err := r.runAgent(runCtx); err != nil {
			select {
			case <-runCtx.Done():
				return
			case errorsCh <- fmt.Errorf("agent execution failed: %w", err):
			}
		}
	}()

	go func() {
		defer func() { close(eventsCh); close(errorsCh) }()

		r.processEvents(runCtx, sessionID, agentEmit, resumeCh, eventsCh, errorsCh)
	}()

	return runID, eventsCh, errorsCh, nil
}

// Cancel cancels a running run by ID.
func (r *Runner) Cancel(runID string) error {
	r.mu.Lock()
	cancel, exists := r.activeRuns[runID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("run %s not found", runID)
	}

	cancel()

	return nil
}

func (r *Runner) runAgent(runCtx *core.RunContext) error {
	if err := r.agent.Start(runCtx); err != nil {
		return err
	}

	// Ensure the agent is stopped when the run context is done
	defer func() {
		if err := r.agent.Stop(runCtx); err != nil {
			r.logger.Warn("error stopping agent %s: %v", r.agent.Name(), err)
		}
	}()

	return r.agent.Run(runCtx)
}

func (r *Runner) processEvents(
	runCtx *core.RunContext,
	sessionID string,
	agentEmit <-chan core.Event,
	resumeCh chan<- struct{},
	eventsCh chan<- core.Event,
	errorsCh chan<- error,
) {
	for {
		select {
		case <-runCtx.Done():
			return
		case ev, ok := <-agentEmit:
			if !ok {
				return
			}

			if !ev.IsPartial() {
				if err := r.sessionStore.AppendEvent(sessionID, ev); err != nil {
					select {
					case <-runCtx.Done():
						return
					case errorsCh <- fmt.Errorf("failed to append event to session: %w", err):
					}

					return
				}
			}

			select {
			case <-runCtx.Done():
				return
			case eventsCh <- ev:
				r.logger.Debug("runner delivered event event_id=%s session_id=%s", ev.ID, sessionID)
			}

			if !ev.IsPartial() {
				select {
				case <-runCtx.Done():
					return
				case resumeCh <- struct{}{}:
				default:
				}
			}
		}
	}
}
