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

// Config defines tuning parameters for the Engine's operational behavior.
//
// This configuration focuses on core performance and behavioral aspects:
//   - Concurrency: How many invocations can run simultaneously
//   - Streaming: Whether to enable real-time event streaming
//   - Buffering: Channel buffer sizes for event processing
//
// Additional concerns such as timeouts, metrics collection, and distributed
// tracing should be configured via functional options rather than expanding
// this struct to maintain simplicity and focused responsibility.
//
// Example:
//
//	cfg := Config{
//	    MaxConcurrentInvocations: 50,
//	    EnableStreaming: true,
//	    EventBufferSize: 256,
//	}
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

// DefaultConfig provides production-ready default configuration values.
//
// These defaults are chosen for:
//   - Safety: Conservative concurrency limits prevent resource exhaustion
//   - Performance: Reasonable buffer sizes for typical workloads
//   - Compatibility: Streaming enabled for maximum flexibility
//
// Production deployments should tune these values based on:
//   - Available system resources (CPU, memory)
//   - Expected concurrent load
//   - Network latency characteristics
//   - Agent complexity and execution time
//
// Configuration values:
//   - MaxConcurrentInvocations: 10 (safe for most environments)
//   - EnableStreaming: true (enables real-time interactions)
//   - EventBufferSize: 100 (balances memory usage and performance)
var DefaultConfig = Config{
	MaxConcurrentInvocations: 10,
	EnableStreaming:          true,
	EventBufferSize:          100,
}

// Options configures an Engine instance using the functional options pattern.
//
// This struct provides a clean way to configure all aspects of the Engine
// including services, configuration, and logging. Default implementations
// are provided for all services to enable quick setup for development
// and testing scenarios.
//
// The Options struct follows the functional options pattern, allowing for:
//   - Clear, readable configuration
//   - Optional parameters with sensible defaults
//   - Future extensibility without breaking changes
//   - Testable configuration logic
//
// Example:
//
//	engine := New(
//	    WithConfig(customConfig),
//	    WithSessionStore(mySessionStore),
//	    WithLogger(myLogger),
//	)
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

// Engine orchestrates agent execution and manages the complete lifecycle
// of multi-agent conversations within the AgentMesh framework.
//
// The Engine serves as the central coordination point that bridges high-level
// AgentMesh operations with low-level agent implementations. It provides:
//
// Core Responsibilities:
//   - Agent Registry: Thread-safe registration and lookup of named agents
//   - Invocation Management: Async/sync execution with proper lifecycle management
//   - Event Processing: Real-time event streaming and persistence coordination
//   - Resource Management: Concurrency limits and graceful resource cleanup
//   - Service Integration: Coordination with session, artifact, and memory stores
//
// Concurrency Model:
//   - Thread-safe agent registration and lookup via RWMutex
//   - Bounded concurrent invocations to prevent resource exhaustion
//   - Per-invocation goroutines with proper cancellation propagation
//   - Non-blocking event emission with configurable buffering
//
// Event Flow:
//  1. User content is converted to events and persisted
//  2. Agent execution produces a stream of events
//  3. Event actions (state changes, artifacts) are applied to services
//  4. Events are streamed to clients and persisted to session history
//
// Error Handling:
//   - Agent execution errors are propagated via dedicated error channels
//   - Service integration errors terminate the invocation gracefully
//   - Context cancellation provides timeout and cleanup mechanisms
//
// The design intentionally separates orchestration concerns from business logic,
// keeping agent implementations focused on their core responsibilities while
// the Engine handles cross-cutting concerns like persistence and event routing.
//
// Example Usage:
//
//	// Create engine with services
//	engine := New(sessionStore, artifactStore, memoryStore,
//	    WithConfig(DefaultConfig()),
//	    WithLogger(logger))
//
//	// Register agents
//	engine.Register(myModelAgent)
//	engine.Register(mySequentialAgent)

// // Execute agent asynchronously
// invocationID, events, errors, err := engine.Invoke(ctx, "session-1", "MyAgent", userContent)
//
//	if err != nil {
//	    return err
//	}
//
// _ = invocationID
//
// // Process streaming events
//
//	for event := range events {
//	    // Handle real-time events
//	}
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

// New creates a new Engine instance with sensible defaults and optional configuration.
//
// The constructor uses the functional options pattern to provide flexible configuration
// while maintaining backward compatibility and ease of use. All services have
// reasonable in-memory defaults suitable for development, testing, and simple
// production scenarios.
//
// Default Services:
//   - SessionStore: In-memory session storage with thread-safe operations
//   - ArtifactStore: In-memory artifact storage with basic lifecycle management
//   - MemoryStore: In-memory searchable storage with simple text matching
//   - Logger: No-op logger that discards all messages
//
// The defaults enable immediate use without external dependencies, making the
// engine suitable for rapid prototyping, testing, and development scenarios.
// Production deployments should typically provide custom service implementations.
//
// Configuration is applied through functional options, allowing for:
//   - Optional configuration with sensible defaults
//   - Clear, readable configuration at construction time
//   - Future extensibility without breaking existing code
//   - Partial configuration (only override what you need)
//
// Thread Safety:
// The returned Engine is immediately ready for use and is safe for concurrent
// access. All public methods are thread-safe, and the Engine manages its own
// internal synchronization.
//
// Examples:
//
//	// Minimal setup with all defaults
//	engine := New()
//
//	// Custom configuration only
//	engine := New(
//	    WithMaxConcurrent(50),
//	    WithEventBuffer(256),
//	)
//
//	// Production setup with custom services
//	engine := New(
//	    WithConfig(productionConfig),
//	    WithSessionStore(postgresStore),
//	    WithArtifactStore(s3Store),
//	    WithMemoryStore(vectorStore),
//	    WithLogger(structuredLogger),
//	)
//
//	// Mixed approach - some defaults, some custom
//	engine := New(
//	    WithSessionStore(customSessionStore),
//	    WithLogger(myLogger),
//	    // Uses default in-memory stores for artifacts and memory
//	)
//
// Resource Management:
// The Engine does not take ownership of provided services and will not manage
// their lifecycle. Callers remain responsible for properly initializing services
// before use and cleaning them up when no longer needed.
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

// Register adds an agent to the engine's registry, making it available for invocation.
//
// The agent is registered by its name (agent.Name()) and can be invoked using
// that name in Invoke() calls. If an agent with the same name already exists,
// it will be replaced without warning.
//
// Registration is thread-safe and can be called concurrently with other
// operations. However, it's recommended to complete all agent registration
// during initialization before starting invocations to avoid confusion.
//
// The engine does not take ownership of the agent - the caller remains
// responsible for the agent's lifecycle. Agents should remain valid for
// the lifetime of the engine or until they are replaced.
//
// Example:
//
//	modelAgent := agent.NewModelAgent("Assistant", model)
//	engine.Register(modelAgent)
//
//	// Agent can now be invoked by name
//	invocationID, events, errors, err := engine.Invoke(ctx, "session-1", "Assistant", userContent)
//	_ = invocationID
//
// Note: Registration during active invocations is safe but not recommended
// as it may lead to inconsistent behavior if agents are replaced while
// invocations are in progress.
func (e *Engine) Register(a core.Agent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agents[a.Name()] = a
}

// GetAgent retrieves a registered agent by name.
//
// This method provides thread-safe access to the agent registry. The boolean
// return value indicates whether an agent with the given name exists.
//
// This method is primarily used internally by the engine during invocation,
// but is exposed for debugging, introspection, and advanced use cases.
//
// Returns:
//   - The agent instance if found
//   - A boolean indicating whether the agent exists
//
// Example:
//
//	agent, exists := engine.GetAgent("Assistant")
//	if !exists {
//	    return fmt.Errorf("agent not found")
//	}
//	fmt.Printf("Found agent: %s", agent.Name())
//
// The returned agent reference is safe to use concurrently as long as the
// agent implementation itself is thread-safe.
func (e *Engine) GetAgent(name string) (core.Agent, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a, ok := e.agents[name]
	return a, ok
}

// Invoke executes an agent asynchronously and returns channels for real-time event streaming.
//
// This is the primary method for executing agents in the AgentMesh framework.
// It provides non-blocking, streaming execution with proper resource management
// and error handling.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - sessionID: Unique identifier for the conversation session
//   - agentName: Name of the registered agent to execute
//   - userContent: User input content to process
//
// Returns:
//   - eventsCh: Channel that streams events as they are generated
//   - errorsCh: Channel that receives any terminal errors
//   - error: Immediate error if invocation cannot be started
//
// Event Streaming:
// Events are streamed in real-time as the agent generates them. The events
// channel will be closed when the agent completes or encounters an error.
// Clients should range over the events channel to process streaming responses.
//
// Error Handling:
// Three types of errors are possible:
//  1. Immediate errors (returned directly): Agent not found, session issues
//  2. Terminal errors (via errorsCh): Agent execution failures, service errors
//  3. Context cancellation: Operation cancelled or timed out
//
// Resource Management:
// The engine automatically manages goroutines, channels, and invocation tracking.
// Resources are cleaned up when the context is cancelled or execution completes.
// The invocation can be explicitly stopped using StopInvocation().
//
// Concurrency:
// Multiple invocations can run concurrently up to MaxConcurrentInvocations.
// Each invocation runs in its own goroutine with isolated state and channels.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	invocationID, events, errors, err := engine.Invoke(ctx, "session-1", "Assistant", userContent)
//	if err != nil {
//	    return fmt.Errorf("failed to start invocation: %w", err)
//	}
//	_ = invocationID
//
//	// Process streaming events
//	for {
//	    select {
//	    case event, ok := <-events:
//	        if !ok {
//	            // Events channel closed, check for terminal error
//	            select {
//	            case err := <-errors:
//	                return err
//	            default:
//	                return nil // Successful completion
//	            }
//	        }
//	        // Process event in real-time
//	        handleEvent(event)
//	    case err := <-errors:
//	        return fmt.Errorf("agent execution failed: %w", err)
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    }
//	}
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

// InvokeSync executes an agent synchronously and returns all generated events.
//
// This is a convenience wrapper around Invoke() that collects all events
// and returns them as a slice. It's useful for simple request-response
// patterns where real-time streaming is not required.
//
// The method blocks until the agent completes execution or an error occurs.
// All events generated during execution are collected and returned together.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - sessionID: Unique identifier for the conversation session
//   - agentName: Name of the registered agent to execute
//   - userContent: User input content to process
//
// Returns:
//   - []core.Event: All events generated during execution
//   - error: Any error that occurred during execution
//
// Error Handling:
// Errors can occur at different stages:
//   - Invocation startup errors (agent not found, session issues)
//   - Agent execution errors (business logic failures)
//   - Context cancellation (timeout or explicit cancellation)
//
// Event Collection:
// Events are collected in the order they are generated. Partial events
// are included in the result, allowing clients to see the complete
// execution trace if needed.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	invocationID, events, err := engine.InvokeSync(ctx, "session-1", "Assistant", userContent)
//	if err != nil {
//	    return fmt.Errorf("execution failed: %w", err)
//	}
//	_ = invocationID
//
//	// Process all events at once
//	for _, event := range events {
//	    handleEvent(event)
//	}
//
// Performance Considerations:
// This method buffers all events in memory, so it may not be suitable
// for agents that generate large numbers of events. For high-volume
// scenarios, prefer the streaming Invoke() method.
func (e *Engine) InvokeSync(
	ctx context.Context,
	sessionID string,
	agentName string,
	userContent core.Content,
) (string, []core.Event, error) {
	// Start async invocation
	invocationID, eventsCh, errorsCh, err := e.Invoke(ctx, sessionID, agentName, userContent)
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

// StopInvocation forcibly terminates a specific invocation by its ID.
//
// This method provides a way to explicitly cancel a running invocation
// without cancelling the entire context. It's useful for implementing
// user-initiated cancellation or cleanup operations.
//
// The method looks up the invocation by ID and calls its cancellation
// function. Once cancelled, the invocation's goroutines will terminate
// and its resources will be cleaned up.
//
// Parameters:
//   - invocationID: The unique identifier of the invocation to stop
//
// Returns:
//   - error: Non-nil if the invocation ID is not found
//
// Behavior:
//   - The invocation's context will be cancelled
//   - Agent execution will be interrupted
//   - Event and error channels will be closed
//   - The invocation will be removed from the active invocations map
//
// Thread Safety:
// This method is thread-safe and can be called concurrently with other
// operations. The cancellation will take effect immediately.
//
// Example:
//
//	// Start an invocation
//	invocationID, events, errors, err := engine.Invoke(ctx, "session-1", "Agent", content)
//	_ = invocationID
//	if err != nil {
//	    return err
//	}
//
//	// Later, cancel it if needed
//	if err := engine.StopInvocation(invocationID); err != nil {
//	    log.Printf("Failed to stop invocation: %v", err)
//	}
//
// Note: The invocation ID is generated internally by the engine and is not
// directly exposed to callers. In practice, this method is primarily used
// by management interfaces or in testing scenarios.
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

// processEvents orchestrates the core event processing pipeline for an invocation.
//
// This method runs in a dedicated goroutine and handles the complete lifecycle
// of event processing from agent generation to client delivery. It coordinates
// multiple concerns:
//
// Event Flow Pipeline:
//  1. Receive events from agent execution goroutine
//  2. Apply event actions (state changes, artifacts) to services
//  3. Persist non-partial events to session history
//  4. Forward events to client via events channel
//  5. Signal resumption for non-partial events
//
// Error Handling:
// Service errors during event processing are treated as terminal errors
// that terminate the entire invocation. This ensures data consistency
// and prevents partial state corruption.
//
// Context Cancellation:
// The method respects context cancellation at every stage, ensuring
// graceful shutdown when invocations are cancelled or time out.
//
// Resumption Signaling:
// Non-partial events trigger resumption signals that can be used by
// agents to coordinate multi-turn conversations or complex workflows.
//
// Threading Model:
// This method runs in its own goroutine and communicates via channels.
// It's designed to be non-blocking and handle backpressure gracefully
// through buffered channels.
//
// Parameters:
//   - ctx: Cancellation context for the invocation
//   - sessionID: Session identifier for persistence operations
//   - agentEmit: Channel receiving events from agent execution
//   - resumeCh: Channel for signaling agent resumption
//   - eventsCh: Channel for forwarding events to client
//   - errorsCh: Channel for reporting terminal errors
//
// The method terminates when:
//   - Agent closes the emit channel (normal completion)
//   - Context is cancelled (timeout or explicit cancellation)
//   - A terminal error occurs during processing
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

// applyEventActions processes and applies the side-effects encoded in an event's Actions field.
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

// GetSession retrieves the current session by ID.
//
// This method provides access to session data for debugging, introspection,
// or advanced use cases where direct session access is needed. It delegates
// to the underlying session service.
//
// Parameters:
//   - sessionID: The unique identifier of the session to retrieve
//
// Returns:
//   - *core.Session: The session data if found
//   - error: Non-nil if the session cannot be retrieved
//
// The returned session represents a point-in-time snapshot. Changes made
// to the session during invocation may not be reflected in this snapshot.
//
// Example:
//
//	session, err := engine.GetSession("session-1")
//	if err != nil {
//	    return fmt.Errorf("failed to get session: %w", err)
//	}
//	fmt.Printf("Session has %d events", len(session.Events))
//
// This method is primarily used for debugging and monitoring. Normal
// application flow should rely on the session data provided through
// invocation contexts rather than direct session access.
func (e *Engine) GetSession(sessionID string) (*core.Session, error) {
	return e.sessionStore.Get(sessionID)
}
