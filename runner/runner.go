package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/artifact"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/internal/util"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/memory"
	"github.com/hupe1980/agentmesh/metrics"
	"github.com/hupe1980/agentmesh/plugin"
	"github.com/hupe1980/agentmesh/session"
	"github.com/hupe1980/agentmesh/trace"
)

// Options holds dependency + configuration overrides passed to New().
type Options struct {
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

	// Plugin management services.
	PluginManager core.PluginManager

	// Logging services.
	Logger logging.Logger

	// Metrics services.
	Metrics metrics.Provider

	// Tracing services.
	Tracer trace.Provider
}

// Runner coordinates agent execution: resolves the root agent, creates
// invocation contexts, streams events, applies side-effects, and persists
// history. Public methods are safe for concurrent use.
type Runner struct {
	appName string
	agent   core.Agent

	enableStreaming bool
	eventBufferSize int

	sessionStore  core.SessionStore
	artifactStore core.ArtifactStore
	memoryStore   core.MemoryStore
	pluginManager core.PluginManager
	logger        logging.Logger
	metrics       metrics.Provider
	tracer        trace.Provider

	activeRuns map[string]context.CancelFunc
	mu         sync.RWMutex

	wg sync.WaitGroup
}

// New constructs a Runner with optional overrides.
func New(appName string, agent core.Agent, optFns ...func(o *Options)) *Runner {
	opts := Options{
		EnableStreaming: true,
		EventBufferSize: 100,
		SessionStore:    session.NewInMemoryStore(),
		ArtifactStore:   artifact.NewInMemoryStore(),
		MemoryStore:     memory.NewInMemoryStore(),
		PluginManager:   plugin.NewManager(),
		Logger:          logging.NoopLogger{},
		Metrics:         metrics.Noop(),
		Tracer:          trace.Noop(),
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	return &Runner{
		appName:         appName,
		agent:           agent,
		enableStreaming: opts.EnableStreaming,
		eventBufferSize: opts.EventBufferSize,
		sessionStore:    opts.SessionStore,
		artifactStore:   opts.ArtifactStore,
		memoryStore:     opts.MemoryStore,
		pluginManager:   opts.PluginManager,
		logger:          opts.Logger,
		metrics:         opts.Metrics,
		tracer:          opts.Tracer,
		activeRuns:      make(map[string]context.CancelFunc),
	}
}

// sessionQueue implements core.EventWriter and is responsible for persisting
// non-partial events to the session store and forwarding all events to results.
type sessionQueue struct {
	runID   string
	session *core.Session
	store   core.SessionStore
	results chan<- core.RunResult
}

func (q *sessionQueue) Write(ctx context.Context, ev *core.Event) error {
	if err := q.store.AppendEvent(ctx, q.session, ev); err != nil {
		return fmt.Errorf("failed to append event to session: %w", err)
	}

	// Forward the event to consumers
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.results <- core.RunResult{RunID: q.runID, Event: ev}:
		logging.FromContext(ctx).
			Debug("runner delivered event", "session_id", q.session.ID, "event_id", ev.ID)
		return nil
	}
}

// buildRequestContext constructs a RequestContext for this run with the given inputs.
func (r *Runner) buildRequestContext(
	runID string,
	agentInfo core.AgentInfo,
	session *core.Session,
	userParts []core.Part,
	opts core.RunOptions,
) core.RequestContext {
	return core.NewRequestContext(core.RequestContextParams{
		RunID:         runID,
		Agent:         agentInfo,
		UserParts:     userParts,
		MaxModelCalls: opts.MaxModelCalls,
		Session:       session,
		SessionStore:  r.sessionStore,
		ArtifactStore: r.artifactStore,
		MemoryStore:   r.memoryStore,
		PluginManager: r.pluginManager,
	})
}

// runOnUserPartsHook lets plugins observe or replace the incoming user parts.
func (r *Runner) runOnUserPartsHook(
	ctx context.Context,
	reqCtx core.RequestContext,
	parts []core.Part,
) ([]core.Part, error) {
	if r.pluginManager == nil {
		return nil, nil
	}

	replaced, err := r.pluginManager.RunOnUserParts(ctx, reqCtx, parts)
	if err != nil {
		return nil, fmt.Errorf("plugin: on_user_parts: %w", err)
	}

	return replaced, nil
}

// rewriteBlobsAsArtifacts scans a user content for blob-like parts, saves them as
// artifacts, and returns content with those parts replaced by FileURI references.
func (r *Runner) rewriteBlobsAsArtifacts(
	ctx context.Context,
	appName, userID, sessionID string,
	parts []core.Part,
) ([]core.Part, error) {
	filtered := make([]core.Part, 0, len(parts))
	for i, p := range parts {
		switch fp := p.(type) {
		case *core.FilePart:
			// Only treat raw bytes and base64 as blob-like that should be saved/stripped.
			switch fp.File.(type) {
			case *core.FileRawBytes, *core.FileBase64:
				name := fp.Name
				if name == "" {
					name = fmt.Sprintf("upload-%s-%d", util.NewID(), i)
				}

				if err := r.artifactStore.Save(ctx, appName, userID, sessionID, name, fp); err != nil {
					return nil, fmt.Errorf("artifact: failed to save input blob '%s': %w", name, err)
				}

				filtered = append(filtered, &core.FilePart{
					File:     &core.FileURI{URI: "artifact:" + name},
					MimeType: fp.MimeType,
					Name:     fp.Name,
				})
			default:
				filtered = append(filtered, p)
			}
		default:
			filtered = append(filtered, p)
		}
	}

	return filtered, nil
}

// Run starts an asynchronous invocation.
func (r *Runner) Run(
	ctx context.Context,
	userID, sessionID string,
	userParts []core.Part,
	optFns ...func(o *core.RunOptions),
) (string, <-chan core.RunResult, error) {
	runID := util.NewID()

	// Add run_id to logger context
	r.logger = r.logger.With("run_id", runID)

	// Attach logger/metrics/tracer to context for downstream propagation
	ctx = logging.WithLogger(ctx, r.logger)
	ctx = metrics.WithProvider(ctx, r.metrics)
	ctx = trace.WithProvider(ctx, r.tracer)

	tr := r.tracer.Tracer("agentmesh/runner")
	ctx, sp := tr.Start(ctx, "Runner.Run", trace.Attr{Key: "session.id", Value: sessionID})

	// record duration at end
	start := time.Now()
	defer func() {
		r.metrics.
			Histogram("agentmesh_run_duration_seconds").
			Record(
				ctx,
				time.Since(start).Seconds(),
				metrics.Attr{Key: "agent.name", Value: r.agent.Name()},
			)
		sp.End(nil)
	}()

	r.metrics.Counter("agentmesh_runs_total").Add(ctx, 1, metrics.Attr{Key: "agent.name", Value: r.agent.Name()})

	// Default options
	opts := core.RunOptions{
		MaxModelCalls:             100,
		SaveInputBlobsAsArtifacts: false,
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	session, err := r.sessionStore.GetOrCreate(ctx, r.appName, userID, sessionID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get session: %w", err)
	}

	results := make(chan core.RunResult, r.eventBufferSize)

	ctx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.activeRuns[runID] = cancel
	r.mu.Unlock()

	agentInfo := core.AgentInfo{Name: r.agent.Name(), Type: "unknown"}

	// Build initial request context from parts
	reqCtx := r.buildRequestContext(runID, agentInfo, session, userParts, opts)

	// Allow plugins to observe/modify the incoming user parts.
	if replaced, err := r.runOnUserPartsHook(ctx, reqCtx, userParts); err != nil {
		return "", nil, err
	} else if replaced != nil {
		userParts = replaced
		reqCtx = r.buildRequestContext(runID, agentInfo, session, userParts, opts)
	}

	if len(userParts) == 0 {
		return "", nil, fmt.Errorf("no user parts provided")
	}

	if opts.SaveInputBlobsAsArtifacts {
		updated, err := r.rewriteBlobsAsArtifacts(
			ctx,
			r.appName,
			userID,
			sessionID,
			userParts,
		)
		if err != nil {
			return "", nil, err
		}
		userParts = updated
		reqCtx = r.buildRequestContext(runID, agentInfo, session, userParts, opts)
	}

	// Record the initial user content
	userEvent := core.NewUserContentEvent(runID, userParts...)

	// Merge optional state delta into user event actions
	if opts.StateDelta != nil {
		userEvent.Actions.StateDelta = core.Map(opts.StateDelta)
	}

	// Create a queue that persists and forwards events
	queue := &sessionQueue{
		runID:   runID,
		session: session,
		store:   r.sessionStore,
		results: results,
	}

	// Write the user event to the queue
	if err := queue.Write(ctx, userEvent); err != nil {
		return "", nil, fmt.Errorf("failed to write user event to queue: %w", err)
	}

	r.logger.Info("run started", "session_id", sessionID)

	r.wg.Add(1)

	go func() {
		defer func() {
			r.mu.Lock()
			delete(r.activeRuns, runID)
			r.mu.Unlock()
			close(results)

			r.logger.Info("run finished", "session_id", sessionID)
			r.wg.Done()
		}()

		if err := r.agent.Run(ctx, reqCtx, queue); err != nil {
			// Propagate error unless canceled
			select {
			case <-ctx.Done():
				return
			case results <- core.RunResult{
				RunID: runID,
				Err:   fmt.Errorf("agent execution failed: %w", err),
			}:
			}
		}
	}()

	return runID, results, nil
}

// Cancel cancels a running run by ID.
func (r *Runner) Cancel(runID string) error {
	r.mu.RLock()
	cancel, exists := r.activeRuns[runID]
	r.mu.RUnlock()

	if !exists {
		return fmt.Errorf("%w: id=%s", core.ErrRunNotFound, runID)
	}

	cancel()

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
		{"session", r.sessionStore},
		{"artifact", r.artifactStore},
		{"memory", r.memoryStore},
	}

	var errs []error
	for _, s := range stores {
		if s.store != nil {
			if err := s.store.Close(); err != nil {
				// Log each failure for observability at shutdown.
				r.logger.With(
					"app", r.appName,
					"store", s.name,
					"error", err,
				).Error("store close failed")

				errs = append(errs, fmt.Errorf("%s: close: %w", s.name, err))
			}
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
