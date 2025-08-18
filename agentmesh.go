// Package agentmesh provides a high-level façade over the core Engine and
// service abstractions (sessions, artifacts, memory & logging) enabling rapid
// construction of multi‑agent reasoning systems. Most applications interact
// with this package by:
//  1. Creating an AgentMesh via New() (optionally overriding default in‑memory services)
//  2. Registering one or more agents (model, sequential, parallel, loop, custom)
//  3. Invoking agents asynchronously (Invoke) or synchronously (InvokeSync)
//
// The façade delegates orchestration to engine.Engine while keeping setup and
// usage ergonomics concise. All defaults are safe for local development and
// testing; production deployments typically supply durable store implementations
// and a structured logger.
package agentmesh

import (
	"context"

	"github.com/hupe1980/agentmesh/artifact"
	"github.com/hupe1980/agentmesh/core"
	"github.com/hupe1980/agentmesh/engine"
	"github.com/hupe1980/agentmesh/logging"
	"github.com/hupe1980/agentmesh/memory"
	"github.com/hupe1980/agentmesh/session"
)

// Options configures the AgentMesh instance.
type Options struct {
	// Engine configuration (concurrency, streaming, buffers)
	EngineConfig engine.Config
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

	// Stores (defaults to in-memory implementations if not provided)
	SessionStore    core.SessionStore
	ArtifactService core.ArtifactStore
	MemoryStore     core.MemoryStore

	// Logger (defaults to NoOp logger if nil)
	Logger logging.Logger
}

// AgentMesh is the high-level façade aggregating the underlying engine and services.
type AgentMesh struct {
	opts   Options
	engine core.Engine
}

// New creates a new AgentMesh instance with optional overrides. Any unset service is
// initialized with an in-memory implementation.
func New(optFns ...func(o *Options)) *AgentMesh {
	opts := Options{
		EngineConfig:    engine.DefaultConfig,
		SessionStore:    session.NewInMemoryStore(),
		ArtifactService: artifact.NewInMemoryStore(),
		MemoryStore:     memory.NewInMemoryStore(),
		Logger:          logging.NoOpLogger{},
	}

	for _, fn := range optFns {
		fn(&opts)
	}

	r := engine.New(func(o *engine.Options) {
		o.Config = opts.EngineConfig
		o.SessionStore = opts.SessionStore
		o.ArtifactStore = opts.ArtifactService
		o.MemoryStore = opts.MemoryStore
		o.Logger = opts.Logger
	})

	return &AgentMesh{opts: opts, engine: r}
}

// RegisterAgent adds an agent to the underlying runner.
func (m *AgentMesh) RegisterAgent(a core.Agent) { m.engine.Register(a) }

// Invoke starts an asynchronous invocation returning event & error channels.
func (m *AgentMesh) Invoke(
	ctx context.Context,
	sessionID string,
	agentName string,
	userContent core.Content,
) (string, <-chan core.Event, <-chan error, error) {
	return m.engine.Invoke(ctx, sessionID, agentName, userContent)
}

// InvokeSync is a synchronous helper that drains the async channels, accumulates
// events and returns the invocationID.
func (m *AgentMesh) InvokeSync(
	ctx context.Context,
	sessionID string,
	agentName string,
	userContent core.Content,
) (string, []core.Event, error) {
	invocationID, eventsCh, errorsCh, err := m.engine.Invoke(ctx, sessionID, agentName, userContent)
	if err != nil {
		return "", nil, err
	}

	// Collect all events until completion
	var events []core.Event
	for {
		select {
		case <-ctx.Done():
			// Context cancelled - return events collected so far
			return invocationID, events, ctx.Err()

		case event, ok := <-eventsCh:
			if !ok {
				// Events channel closed - check for terminal error
				select {
				case err := <-errorsCh:
					return invocationID, events, err
				default:
					return invocationID, events, nil // Successful completion
				}
			}
			// Collect event
			events = append(events, event)

		case err := <-errorsCh:
			// Terminal error occurred
			if err != nil {
				return invocationID, events, err
			}
		}
	}
}
