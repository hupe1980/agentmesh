package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hupe1980/agentmesh/agent"
	"github.com/hupe1980/agentmesh/artifact"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/memory"
	"github.com/hupe1980/agentmesh/metrics"
	"github.com/hupe1980/agentmesh/session"
	"github.com/hupe1980/agentmesh/trace"
)

// Options for configuring the Runner.
type Options struct {
	// EnableStreaming toggles real-time event streaming vs buffered.
	EnableStreaming bool
	// AgentExecutor to use for running agents.
	AgentExecutor core.AgentExecutor
	// Session management services.
	SessionStore core.SessionStore
	// Artifact management services.
	ArtifactStore core.ArtifactStore
	// Memory management services.
	MemoryStore core.MemoryStore
	// Logging services.
	Logger logging.Logger
	// Metrics services.
	Metrics metrics.Provider
	// Tracing services.
	Tracer trace.Provider
}

// DefaultOptions provides the default configuration for Runner.
var DefaultOptions = Options{
	EnableStreaming: true,
	AgentExecutor:   agent.DefaultAgentExecutor,
	SessionStore:    session.NewInMemoryStore(),
	ArtifactStore:   artifact.NewInMemoryStore(),
	MemoryStore:     memory.NewInMemoryStore(),
	Logger:          logging.NoopLogger{},
	Metrics:         metrics.Noop(),
	Tracer:          trace.Noop(),
}

// services groups the various stores and services used by the runner.
type services struct {
	sessionStore  core.SessionStore
	artifactStore core.ArtifactStore
	memoryStore   core.MemoryStore
	pluginManager core.PluginManager
	logger        logging.Logger
	metrics       metrics.Provider
	tracer        trace.Provider
}

// Runner coordinates agent execution: resolves the root agent, creates
// invocation contexts, streams events, applies side-effects, and persists
// history. Public methods are safe for concurrent use.
type Runner struct {
	app   core.App
	agent core.Agent

	enableStreaming bool

	agentExecutor core.AgentExecutor
	svc           services

	activeRuns map[string]context.CancelFunc
	mu         sync.RWMutex

	wg sync.WaitGroup
}

// New constructs a Runner with optional overrides.
func New(application core.App, optFns ...func(o *Options)) *Runner {
	opts := DefaultOptions

	for _, fn := range optFns {
		fn(&opts)
	}

	rootAgent := application.RootAgent()
	plugins := application.Plugins()

	return &Runner{
		app:             application,
		agent:           rootAgent,
		enableStreaming: opts.EnableStreaming,
		agentExecutor:   opts.AgentExecutor,
		svc: services{
			sessionStore:  opts.SessionStore,
			artifactStore: opts.ArtifactStore,
			memoryStore:   opts.MemoryStore,
			pluginManager: core.NewPluginManager(plugins...),
			logger:        opts.Logger,
			metrics:       opts.Metrics,
			tracer:        opts.Tracer,
		},
		activeRuns: make(map[string]context.CancelFunc),
	}
}

// DefaultRunOptions provides the default configuration for Run invocations.
var DefaultRunOptions = core.RunOptions{
	MaxModelCalls:   100,
	EventBufferSize: 100,
	RunIDKey:        "run_id",
}

// Run starts an asynchronous invocation. (Refactored wiring + small helpers)
func (r *Runner) Run(
	ctx context.Context,
	userID, sessionID string,
	userParts []core.Part,
	optFns ...func(o *core.RunOptions),
) (string, <-chan core.RunResult, error) {
	// Generate a new run ID.
	runID := uuid.NewString()

	opts := DefaultRunOptions

	for _, fn := range optFns {
		fn(&opts)
	}

	// If an external run ID is provided, use it instead of the generated one.
	if opts.ExternalRunID != "" {
		runID = opts.ExternalRunID
	}

	// Prepare context & observability (logger, metrics, tracing)
	ctx, cancel, runLogger, sp, start := r.prepareRunContext(ctx, opts.RunIDKey, runID, sessionID)

	// Register active run cancel func
	r.registerRun(runID, cancel)

	// Ensure duration recorded & span ended when Run returns.
	defer func() {
		// record duration
		r.recordRunDuration(ctx, sp, start)
	}()

	// Increment metric for run count
	r.svc.metrics.Counter("agentmesh_runs_total").
		Add(ctx, 1, metrics.Attr{Key: "agent.name", Value: r.agent.Name()})

	// Load or create session
	session, err := r.svc.sessionStore.GetOrCreate(ctx, r.app.Name(), userID, sessionID)
	if err != nil {
		r.unregisterRunAndCancel(runID)
		return "", nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Build initial request context
	reqCtx := r.buildRequestContext(runID, session, userParts, opts)

	// BeforeRun hook: allows global setup / early short-circuit.
	if resultsChan, err := func() (<-chan core.RunResult, error) {
		var parts []core.Part
		if pm := reqCtx.PluginManager(); pm != nil {
			out, err := pm.RunBeforeRun(ctx, reqCtx)
			if err != nil {
				return nil, fmt.Errorf("plugin: before_run: %w", err)
			}
			parts = out
		}

		if parts == nil {
			return nil, nil
		}

		results := make(chan core.RunResult, 1)
		writer := &sessionWriter{
			runID:   runID,
			session: session,
			store:   r.svc.sessionStore,
			results: results,
			onEvent: func(ctx context.Context, ev *core.Event) (*core.Event, error) {
				if pm := reqCtx.PluginManager(); pm != nil {
					return pm.RunOnEvent(ctx, reqCtx, ev)
				}
				return nil, nil
			},
		}
		beforeEvent := core.NewFullAssistantEvent(runID, r.agent.Name(), parts...)
		if err := writer.Write(ctx, beforeEvent); err != nil {
			close(results)
			return nil, fmt.Errorf("failed to write before_run event: %w", err)
		}

		// AfterRun still invoked for short-circuited runs.
		if pm := reqCtx.PluginManager(); pm != nil {
			if err := pm.RunAfterRun(ctx, reqCtx); err != nil {
				close(results)
				return nil, fmt.Errorf("plugin: after_run: %w", err)
			}
		}

		results <- core.RunResult{RunID: runID, Event: beforeEvent}
		close(results)
		return results, nil
	}(); err != nil {
		r.unregisterRunAndCancel(runID)
		return "", nil, err
	} else if resultsChan != nil {
		runLogger.Info("run finished (before_run short-circuit)", "session_id", sessionID)
		return runID, resultsChan, nil
	}

	// Allow plugins to observe/modify the incoming user parts.
	if replaced, err := r.onUserParts(ctx, reqCtx, userParts); err != nil {
		r.unregisterRunAndCancel(runID)
		return "", nil, err
	} else if replaced != nil {
		userParts = replaced
		reqCtx = r.buildRequestContext(runID, session, userParts, opts)
	}

	if len(userParts) == 0 {
		r.unregisterRunAndCancel(runID)
		return "", nil, fmt.Errorf("no user parts provided")
	}

	// Record the initial user content
	userEvent := core.NewUserContentEvent(runID, userParts...)
	if opts.StateDelta != nil {
		userEvent.Actions.StateDelta = core.Map(opts.StateDelta)
	}

	results := make(chan core.RunResult, opts.EventBufferSize)
	writer := &sessionWriter{
		runID:   runID,
		session: session,
		store:   r.svc.sessionStore,
		results: results,
		onEvent: func(ctx context.Context, ev *core.Event) (*core.Event, error) {
			if pm := reqCtx.PluginManager(); pm != nil {
				return pm.RunOnEvent(ctx, reqCtx, ev)
			}
			return nil, nil
		},
	}
	if err := writer.Write(ctx, userEvent); err != nil {
		r.unregisterRunAndCancel(runID)
		return "", nil, fmt.Errorf("failed to write user event: %w", err)
	}

	runLogger.Info("run started", "session_id", sessionID)

	// Launch asynchronous execution goroutine (keeps same semantics as before).
	r.launchRun(ctx, runID, reqCtx, writer, results, runLogger, sessionID)

	return runID, results, nil
}

// Cancel cancels a running run by ID.
func (r *Runner) Cancel(runID string) error {
	r.mu.Lock()
	cancel, exists := r.activeRuns[runID]
	if exists {
		delete(r.activeRuns, runID)
	}
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("%w: id=%s", core.ErrRunNotFound, runID)
	}

	if cancel != nil {
		cancel()
	}

	return nil
}

// Close waits for all active runs to finish and closes stores if supported.
func (r *Runner) Close() error {
	r.wg.Wait()

	// storeClosers holds the list of stores that need to be closed.
	type storeCloser struct {
		name  string
		store interface{ Close() error }
	}

	stores := []storeCloser{
		{"session", r.svc.sessionStore},
		{"artifact", r.svc.artifactStore},
		{"memory", r.svc.memoryStore},
	}

	var errs []error
	for _, s := range stores {
		if s.store != nil {
			if err := s.store.Close(); err != nil {
				// Log each failure for observability at shutdown.
				r.svc.logger.Error("store close failed", "app", r.app.Name(), "store", s.name, "error", err)

				errs = append(errs, fmt.Errorf("%s: close: %w", s.name, err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// buildRequestContext constructs a RequestContext for this run with the given inputs.
func (r *Runner) buildRequestContext(
	runID string,
	session *core.Session,
	userParts []core.Part,
	opts core.RunOptions,
) core.RequestContext {
	return core.NewRequestContext(core.RequestContextParams{
		RunID:         runID,
		Agent:         r.agent,
		UserParts:     userParts,
		MaxModelCalls: opts.MaxModelCalls,
		Session:       session,
		SessionStore:  r.svc.sessionStore,
		ArtifactStore: r.svc.artifactStore,
		MemoryStore:   r.svc.memoryStore,
		PluginManager: r.svc.pluginManager,
	})
}

// rewriteBlobsAsArtifacts scans user content for blob-like parts, saves them as
// artifacts, and replaces them with FileURI references.
// prepareRunContext wires logger/metrics/tracer into ctx and returns ctx, cancel,
// the run-scoped logger, the span and the start time.
func (r *Runner) prepareRunContext(
	ctx context.Context,
	runIDKey, runID, sessionID string,
) (
	context.Context,
	context.CancelFunc,
	logging.Logger,
	trace.Span,
	time.Time,
) {
	ctx, cancel := context.WithCancel(ctx)

	runLogger := r.svc.logger.With(runIDKey, runID)

	// attach logger/metrics/tracer to context
	ctx = logging.WithLogger(ctx, runLogger)
	ctx = metrics.WithProvider(ctx, r.svc.metrics)
	ctx = trace.WithProvider(ctx, r.svc.tracer)

	// start span
	tr := r.svc.tracer.Tracer("agentmesh/runner")
	ctx, sp := tr.Start(ctx, "Runner.Run", trace.Attr{Key: "session.id", Value: sessionID})

	return ctx, cancel, runLogger, sp, time.Now()
}

// recordRunDuration records histogram and ends the span.
func (r *Runner) recordRunDuration(ctx context.Context, sp trace.Span, start time.Time) {
	r.svc.metrics.
		Histogram("agentmesh_run_duration_seconds").
		Record(ctx, time.Since(start).Seconds(), metrics.Attr{Key: "agent.name", Value: r.agent.Name()})
	sp.End(nil)
}

// launchRun starts the agent execution goroutine and returns immediately.
// Keeps semantics identical to your previous code but centralized here.
func (r *Runner) launchRun(
	ctx context.Context,
	runID string,
	reqCtx core.RequestContext,
	writer *sessionWriter,
	results chan core.RunResult,
	runLogger logging.Logger,
	sessionID string,
) {
	r.wg.Add(1)
	go func() {
		defer func() {
			r.unregisterRun(runID)

			// Run AfterRun before closing results so errors can propagate.
			if pm := reqCtx.PluginManager(); pm != nil {
				if err := pm.RunAfterRun(ctx, reqCtx); err != nil {
					// best-effort deliver the error unless context canceled
					select {
					case <-ctx.Done():
					default:
						results <- core.RunResult{RunID: runID, Err: fmt.Errorf("plugin: after_run: %w", err)}
					}
				}
			}
			close(results)
			runLogger.Info("run finished", "session_id", sessionID)
			r.wg.Done()
		}()

		if err := r.agentExecutor.Execute(ctx, reqCtx, r.agent, writer); err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				results <- core.RunResult{RunID: runID, Err: fmt.Errorf("agent execution failed: %w", err)}
			}
		}
	}()
}

func (r *Runner) onUserParts(
	ctx context.Context,
	reqCtx core.RequestContext,
	parts []core.Part,
) ([]core.Part, error) {
	var (
		replaced []core.Part
		err      error
	)

	if pm := reqCtx.PluginManager(); pm != nil {
		replaced, err = pm.RunOnUserParts(ctx, reqCtx, parts)
	}

	if err != nil {
		return nil, fmt.Errorf("plugin: on_user_parts: %w", err)
	}

	return replaced, nil
}

// registerRun stores the cancel func for a run in a threadsafe way.
func (r *Runner) registerRun(runID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activeRuns[runID] = cancel
}

// unregisterRun safely removes a run from activeRuns.
func (r *Runner) unregisterRun(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.activeRuns, runID)
}

// unregisterRunAndCancel retrieves the cancel func (if any), calls it and removes the run.
//
// Use this when you want to ensure any associated goroutine/context is cancelled and
// the run entry is removed from the registry.
func (r *Runner) unregisterRunAndCancel(runID string) {
	r.mu.Lock()
	cancel, ok := r.activeRuns[runID]
	if ok {
		delete(r.activeRuns, runID)
	}
	r.mu.Unlock()

	if ok && cancel != nil {
		cancel()
	}
}
