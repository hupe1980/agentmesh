// Package graph provides a simplified API for building executable workflows.
package graph

import (
	"context"
	"errors"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// END is the terminal node constant.
const END = "__end__"

// ApprovalsKey is the reserved state key for storing node approvals.
const ApprovalsKey = "__approvals__"

// Sentinel errors for graph validation.
var (
	ErrNoEntryPoint  = errors.New("graph: no entry point defined")
	ErrNodeNotFound  = errors.New("graph: node not found")
	ErrDuplicateNode = errors.New("graph: duplicate node name")
	ErrDuplicateKey  = errors.New("graph: duplicate key")
	ErrInvalidTarget = errors.New("graph: invalid target node")
)

// node represents an internal node in the graph.
type node struct {
	name    string
	fn      NodeFunc
	targets []string
}

// interruptPoint represents a configured interrupt.
type interruptPoint struct {
	nodeName string
	before   bool // true = before, false = after
	config   *interruptConfig
}

// Builder is a fluent workflow builder.
// Create with New(), add nodes, then Build() to get an executable Graph.
// Input is []Message (conversation history), output is Message (response).
type Builder struct {
	keys        []StateKey
	nodes       map[string]*node
	entryPoints []string
	interrupts  []interruptPoint

	// Configuration (set via With* methods)
	store          Store
	checkpointer   checkpoint.Checkpointer
	runID          string
	nodeMiddleware []NodeMiddleware // Node-level middleware (wraps each node)
	runMiddleware  []RunMiddleware  // Run-level middleware (wraps Run/Resume)
	executor       Executor
}

// Graph is an executable workflow with immutable structure.
// Created by calling Build() on a Builder.
type Graph struct {
	keys        []StateKey
	nodes       map[string]*node
	entryPoints []string
	interrupts  []interruptPoint

	store          Store
	checkpointer   checkpoint.Checkpointer
	runID          string
	nodeMiddleware []NodeMiddleware // Node-level middleware (wraps each node)
	runMiddleware  []RunMiddleware  // Run-level middleware (wraps Run/Resume)
	executor       Executor
}

// New creates a graph builder with the given state keys.
// Keys are automatically registered.
// messagesKey is always included and used as the output key.
// Duplicate keys will cause Build() to fail.
//
// By default, messagesKey is included. Additional keys can be provided.
func New(keys ...StateKey) *Builder {
	// Ensure messagesKey is always included
	allKeys := append([]StateKey{messagesKey}, keys...)

	return &Builder{
		keys:  allKeys,
		nodes: make(map[string]*node),
	}
}

// Node adds a node to the graph.
// Targets are the possible next nodes (use END for terminal).
func (b *Builder) Node(name string, fn NodeFunc, targets ...string) *Builder {
	b.nodes[name] = &node{name: name, fn: fn, targets: targets}
	return b
}

// Subgraph creates a NodeFunc that executes a compiled subgraph.
// Use with Node() to add a subgraph while maintaining the fluent builder pattern:
//
//	child, _ := graph.New(...).Build()
//
//	parent.Node("validate", graph.Subgraph(child,
//	    func(ctx context.Context, view graph.ReadOnlyScope) ([]graph.Message, error) {
//	        return graph.GetMessages(view), nil
//	    },
//	    func(ctx context.Context, out graph.Message) (graph.Updates, error) {
//	        return graph.Updates{graph.MessagesKeyName: []graph.Message{out}}, nil
//	    },
//	), "next")
//
// The InputMapper transforms parent state into subgraph input.
// The OutputMapper transforms subgraph output into parent state updates.
func Subgraph(
	sub *Graph,
	inputMapper InputMapper,
	outputMapper OutputMapper,
) NodeFunc {
	return func(ctx context.Context, scope Scope) (*Command, error) {
		// Map parent state to subgraph input (use scope as view)
		input, err := inputMapper(ctx, scopeAsView{scope})
		if err != nil {
			return Fail(err)
		}

		// Execute subgraph (collect final output)
		var lastOutput message.Message
		for output, err := range sub.Run(ctx, input) {
			if err != nil {
				return Fail(err)
			}
			lastOutput = output
		}

		// Map subgraph output to parent state updates
		updates, err := outputMapper(ctx, lastOutput)
		if err != nil {
			return Fail(err)
		}

		return &Command{Updates: updates}, nil
	}
}

// scopeAsView adapts a Scope to the View interface for read-only operations.
type scopeAsView struct {
	scope Scope
}

func (s scopeAsView) NodeName() string {
	return s.scope.NodeName()
}

func (s scopeAsView) GetValue(name string) (any, bool) {
	return s.scope.GetValue(name)
}

func (s scopeAsView) Messages() []message.Message {
	return s.scope.Messages()
}

func (s scopeAsView) LastMessage() message.Message {
	return s.scope.LastMessage()
}

func (s scopeAsView) ManagedValues() *ManagedValueRegistry {
	return s.scope.ManagedValues()
}

func (s scopeAsView) ToMap() map[string]any {
	return s.scope.ToMap()
}

// Start sets the entry point node(s).
// Multiple entry points run in parallel.
func (b *Builder) Start(names ...string) *Builder {
	b.entryPoints = names
	return b
}

// InterruptBefore adds an interrupt before the specified node.
// When execution reaches this node, it will pause and yield an interrupt event.
func (b *Builder) InterruptBefore(nodeName string, opts ...InterruptOption) *Builder {
	cfg := &interruptConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	b.interrupts = append(b.interrupts, interruptPoint{
		nodeName: nodeName,
		before:   true,
		config:   cfg,
	})
	return b
}

// InterruptAfter adds an interrupt after the specified node.
// When execution completes this node, it will pause and yield an interrupt event.
func (b *Builder) InterruptAfter(nodeName string, opts ...InterruptOption) *Builder {
	cfg := &interruptConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	b.interrupts = append(b.interrupts, interruptPoint{
		nodeName: nodeName,
		before:   false,
		config:   cfg,
	})
	return b
}

// WithStore sets a custom state store.
func (b *Builder) WithStore(store Store) *Builder {
	b.store = store
	return b
}

// WithCheckpointer sets the checkpointer and run ID.
func (b *Builder) WithCheckpointer(cp checkpoint.Checkpointer, runID string) *Builder {
	b.checkpointer = cp
	b.runID = runID
	return b
}

// WithNodeMiddleware adds node-level middleware to the builder.
// Node middleware wraps each node execution and runs for every node.
// For middleware that should wrap the entire Run/Resume operation, use WithRunMiddleware.
func (b *Builder) WithNodeMiddleware(mw ...NodeMiddleware) *Builder {
	b.nodeMiddleware = append(b.nodeMiddleware, mw...)
	return b
}

// WithRunMiddleware adds run-level middleware to the builder.
// Run middleware wraps the entire Run/Resume operation, intercepting:
//   - Input before execution starts
//   - Output after execution completes
//
// This is useful for:
//   - Input validation/guardrails (check user input once at start)
//   - Output validation/guardrails (check final output once at end)
//   - Logging/observability at the run level
//   - Request/response transformation
//
// Middleware is applied in order: first added = outermost wrapper.
func (b *Builder) WithRunMiddleware(mw ...RunMiddleware) *Builder {
	b.runMiddleware = append(b.runMiddleware, mw...)
	return b
}

// WithExecutor sets a custom executor.
func (b *Builder) WithExecutor(exec Executor) *Builder {
	b.executor = exec
	return b
}

// buildConfig holds build-time configuration.
type buildConfig struct {
	validationOpts ValidationOptions
}

// BuildOption configures a Build call.
type BuildOption func(*buildConfig)

// WithValidation sets custom validation options.
func WithValidation(opts ValidationOptions) BuildOption {
	return func(c *buildConfig) {
		c.validationOpts = opts
	}
}

// WithStrictValidation enables strict validation mode.
// This includes cycle detection and disconnected node detection.
func WithStrictValidation() BuildOption {
	return func(c *buildConfig) {
		c.validationOpts = StrictValidationOptions()
	}
}

// WithoutValidation disables validation (use with caution).
// Only use this for trusted graphs or when validation overhead is unacceptable.
func WithoutValidation() BuildOption {
	return func(c *buildConfig) {
		c.validationOpts = ValidationOptions{
			Level: ValidationLevelNone,
		}
	}
}

// Build compiles and validates the builder.
// Returns an executable Graph or an error.
func (b *Builder) Build(opts ...BuildOption) (*Graph, error) {
	// Apply build options
	cfg := &buildConfig{
		validationOpts: DefaultValidationOptions(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Use the comprehensive validator
	if errs := b.Validate(cfg.validationOpts); len(errs) > 0 {
		// Return the first validation error
		return nil, errs[0]
	}

	// Set defaults
	store := b.store
	if store == nil {
		store = newMemoryStore()
	}

	return &Graph{
		keys:           b.keys,
		nodes:          b.nodes,
		entryPoints:    b.entryPoints,
		interrupts:     b.interrupts,
		store:          store,
		checkpointer:   b.checkpointer,
		runID:          b.runID,
		nodeMiddleware: b.nodeMiddleware,
		runMiddleware:  b.runMiddleware,
		executor:       b.executor,
	}, nil
}

// applyRunMiddleware wraps a core run function with the configured run middleware.
// Middleware is applied in reverse order so first added = outermost wrapper.
func (g *Graph) applyRunMiddleware(coreRun RunFunc) RunFunc {
	wrapped := coreRun
	for i := len(g.runMiddleware) - 1; i >= 0; i-- {
		wrapped = g.runMiddleware[i](wrapped)
	}
	return wrapped
}

// executeWithOptions runs the executor with the given input and options.
// This is the shared execution logic for both Run and Resume.
func (g *Graph) executeWithOptions(ctx context.Context, input []message.Message, runOpts []runOption) iter.Seq2[message.Message, error] {
	return func(yield func(message.Message, error) bool) {
		// Build executor config
		cfg := g.buildExecutorConfig()

		// Use configured executor or default Pregel executor
		exec := g.executor
		if exec == nil {
			exec = NewPregelExecutor()
		}

		// Delegate to executor
		for output, err := range exec.Run(ctx, cfg, input, runOpts...) {
			if !yield(output, err) {
				return
			}
		}
	}
}

// Run executes the compiled graph with input.
// For resuming from a checkpoint without providing new input, use [Resume] instead.
func (g *Graph) Run(ctx context.Context, input []message.Message, opts ...RunOption) iter.Seq2[message.Message, error] {
	// Build the core run function
	coreRun := func(ctx context.Context, input []message.Message) iter.Seq2[message.Message, error] {
		// Convert RunOptions to internal runOptions
		runOpts := make([]runOption, len(opts))
		for i, opt := range opts {
			opt := opt // Capture loop variable
			runOpts[i] = func(cfg *runConfig) { opt.applyRun(cfg) }
		}
		return g.executeWithOptions(ctx, input, runOpts)
	}

	return g.applyRunMiddleware(coreRun)(ctx, input)
}

// Resume continues execution from a checkpoint without providing new input.
// This is the correct way to resume a paused graph - the checkpoint state is
// restored without being overwritten by a zero-value input.
//
// Parameters:
//   - runID: The run ID for checkpointing (required)
//   - opts: Optional resume options for human-in-the-loop workflows
//
// Example:
//
//	// Resume from the latest checkpoint (auto-restore)
//	for output, err := range compiled.Resume(ctx, runID) {
//	    // process output
//	}
//
//	// Resume from a specific checkpoint
//	savedCp, _ := checkpointer.Load(ctx, runID)
//	for output, err := range compiled.Resume(ctx, runID, graph.WithCheckpoint(savedCp)) {
//	    // process output
//	}
//
//	// Resume with human input (human-in-the-loop)
//	for output, err := range compiled.Resume(ctx, runID,
//	    graph.WithCheckpoint(savedCp),
//	    graph.WithResumeValue("wait_node", answerKey.Name(), "user input"),
//	) {
//	    // process output
//	}
func (g *Graph) Resume(ctx context.Context, runID string, opts ...ResumeOption) iter.Seq2[message.Message, error] {
	// Build the core run function (Resume uses zero input)
	coreRun := func(ctx context.Context, _ []message.Message) iter.Seq2[message.Message, error] {
		// Build runOptions: skipInputMerge + runID + autoRestore (default) + user options
		runOpts := make([]runOption, 0, len(opts)+3)
		runOpts = append(runOpts,
			func(cfg *runConfig) { cfg.skipInputMerge = true },
			func(cfg *runConfig) { cfg.runID = runID },
			func(cfg *runConfig) { cfg.autoRestore = true }, // Default to auto-restore
		)
		for _, opt := range opts {
			opt := opt // Capture loop variable
			runOpts = append(runOpts, func(cfg *runConfig) { opt.applyResume(cfg) })
		}

		return g.executeWithOptions(ctx, nil, runOpts)
	}

	// Resume always uses nil input
	return g.applyRunMiddleware(coreRun)(ctx, nil)
}

// buildExecutorConfig creates the executor configuration from the compiled graph.
func (g *Graph) buildExecutorConfig() *ExecutorConfig {
	// Build nodes map - types are already correct
	nodes := make(map[string]ExecutorNode, len(g.nodes))
	for name, n := range g.nodes {
		nodes[name] = ExecutorNode{
			Name:    n.name,
			Fn:      n.fn,
			Targets: n.targets,
		}
	}

	// Build interrupt lookup maps
	interruptsBefore := make(map[string]*interruptConfig)
	interruptsAfter := make(map[string]*interruptConfig)
	for _, ip := range g.interrupts {
		if ip.before {
			interruptsBefore[ip.nodeName] = ip.config
		} else {
			interruptsAfter[ip.nodeName] = ip.config
		}
	}

	// Build key registry for reducers
	registry := NewKeyRegistry()
	for _, key := range g.keys {
		registry.Register(key.Name(), key.ReducerFunc())
	}

	// Register internal approvals key (replace semantics)
	registry.Register(ApprovalsKey, ReducerFunc{
		ZeroFn:   func() any { return nil },
		ReduceFn: func(_, updated any) any { return updated },
	})

	return &ExecutorConfig{
		Execution: ExecutionConfig{
			Nodes:          nodes,
			EntryPoints:    g.entryPoints,
			NodeMiddleware: g.nodeMiddleware,
			Store:          g.store,
			KeyRegistry:    registry,
		},
		Checkpoint: CheckpointConfig{
			Checkpointer: g.checkpointer,
			RunID:        g.runID,
		},
		Interrupt: InterruptConfig{
			Before: interruptsBefore,
			After:  interruptsAfter,
		},
	}
}

// InterruptError signals that execution has paused for approval.
type InterruptError struct {
	NodeName string
	Before   bool
}

func (e *InterruptError) Error() string {
	if e.Before {
		return "graph: interrupt before node " + e.NodeName
	}
	return "graph: interrupt after node " + e.NodeName
}

// runConfig holds runtime configuration.
type runConfig struct {
	checkpoint          *checkpoint.Checkpoint
	approvals           map[string]*ApprovalResponse
	stateUpdates        map[string]any
	managedValues       *ManagedValueRegistry
	runID               string
	maxConcurrency      int
	maxIterations       int
	checkpointInterval  int
	autoRestore         bool
	failOnCheckpointErr bool
	skipInputMerge      bool     // true when resuming without new input
	pausedNodes         []string // Nodes to resume from (overrides entry points)
}

// runOption is the internal option type used by all option types.
type runOption func(*runConfig)

// RunOption is an option that can be passed to Run().
type RunOption interface {
	applyRun(*runConfig)
}

// ResumeOption is an option that can be passed to Resume().
type ResumeOption interface {
	applyResume(*runConfig)
}

// runOnlyOption is for options that only work with Run().
type runOnlyOption struct {
	fn runOption
}

func (o runOnlyOption) applyRun(cfg *runConfig) {
	o.fn(cfg)
}

// resumeOnlyOption is for options that only work with Resume().
type resumeOnlyOption struct {
	fn runOption
}

func (o resumeOnlyOption) applyResume(cfg *runConfig) {
	o.fn(cfg)
}

// SharedOption implements both RunOption and ResumeOption interfaces.
// This allows common options (like WithMaxConcurrency) to work with both
// Run() and Resume() without explicit conversion.
type SharedOption func(*runConfig)

// Implement RunOption interface
func (s SharedOption) applyRun(cfg *runConfig) {
	s(cfg)
}

// Implement ResumeOption interface
func (s SharedOption) applyResume(cfg *runConfig) {
	s(cfg)
}

// -----------------------------------------------------------------------------
// Resume-only options (can only be used with Resume)
// -----------------------------------------------------------------------------

// WithCheckpoint resumes from a specific checkpoint instead of auto-restoring
// from the latest checkpoint.
// This is a Resume-only option.
//
// Example:
//
//	savedCp, _ := checkpointer.Load(ctx, runID)
//	compiled.Resume(ctx, runID, graph.WithCheckpoint(savedCp))
func WithCheckpoint(cp *checkpoint.Checkpoint) ResumeOption {
	return resumeOnlyOption{fn: func(cfg *runConfig) {
		cfg.checkpoint = cp
		cfg.autoRestore = false // Disable auto-restore when explicit checkpoint is provided
	}}
}

// WithResumeValue is a convenience function that sets a state value and
// auto-approves a node for simple human-input scenarios.
// This is a Resume-only option.
//
// Example:
//
//	compiled.Resume(ctx, runID,
//	    graph.WithResumeValue("wait_for_answer", answerKey.Name(), "Paris"),
//	)
func WithResumeValue(nodeName string, key string, value any) ResumeOption {
	return resumeOnlyOption{fn: func(cfg *runConfig) {
		// Apply state update
		if cfg.stateUpdates == nil {
			cfg.stateUpdates = make(map[string]any)
		}
		cfg.stateUpdates[key] = value

		// Auto-approve the node
		if cfg.approvals == nil {
			cfg.approvals = make(map[string]*ApprovalResponse)
		}
		cfg.approvals[nodeName] = &ApprovalResponse{
			Decision: ApprovalApproved,
			Reason:   "Auto-approved via WithResumeValue",
		}
	}}
}

// WithApproval provides an approval response for a node interrupt.
// This is a Resume-only option - approvals are provided when continuing after an interrupt.
func WithApproval(nodeName string, approval *ApprovalResponse) ResumeOption {
	return resumeOnlyOption{fn: func(cfg *runConfig) {
		if cfg.approvals == nil {
			cfg.approvals = make(map[string]*ApprovalResponse)
		}
		cfg.approvals[nodeName] = approval
	}}
}

// WithStateUpdates applies state updates when resuming.
// This is a Resume-only option for human-in-the-loop workflows.
//
// Example:
//
//	savedCp, _ := checkpointer.Load(ctx, runID)
//	compiled.Resume(ctx, savedCp, runID,
//	    graph.WithStateUpdates(map[string]any{"answer": "Paris"}),
//	    graph.WithApproval("wait_node", approval),
//	)
func WithStateUpdates(updates map[string]any) ResumeOption {
	return resumeOnlyOption{fn: func(cfg *runConfig) {
		cfg.stateUpdates = updates
	}}
}

// -----------------------------------------------------------------------------
// Run-only options (can only be used with Run)
// -----------------------------------------------------------------------------

// WithRunID sets the run ID for checkpointing when starting a new execution.
// This is a Run-only option.
//
// Example:
//
//	compiled.Run(ctx, input, graph.WithRunID("workflow-123"))
func WithRunID(id string) RunOption {
	return runOnlyOption{fn: func(cfg *runConfig) {
		cfg.runID = id
	}}
}

// WithInitialValue sets an initial state value when starting graph execution.
// This is a Run-only option.
//
// Example:
//
//	compiled.Run(ctx, messages,
//	    graph.WithInitialValue(agent.SessionIDKey, "session-123"),
//	)
func WithInitialValue[T any](key Key[T], value T) RunOption {
	return runOnlyOption{fn: func(cfg *runConfig) {
		if cfg.stateUpdates == nil {
			cfg.stateUpdates = make(map[string]any)
		}
		cfg.stateUpdates[key.Name()] = value
	}}
}

// -----------------------------------------------------------------------------
// Shared options (can be used with both Run and Resume)
// -----------------------------------------------------------------------------

// WithManagedValues attaches ephemeral runtime values to the graph execution.
// This option works with both Run and Resume.
//
// Example:
//
//	compiled.Run(ctx, input, graph.WithManagedValues(apiKeyMV))
//	compiled.Resume(ctx, runID, graph.WithManagedValues(apiKeyMV))
func WithManagedValues(values ...ManagedValue) SharedOption {
	return func(cfg *runConfig) {
		if cfg.managedValues == nil {
			cfg.managedValues = NewManagedValueRegistry()
		}
		for _, v := range values {
			cfg.managedValues.register(v)
		}
	}
}

// WithMaxConcurrency sets the maximum number of nodes that can execute in parallel.
// This option works with both Run and Resume.
func WithMaxConcurrency(n int) SharedOption {
	return func(cfg *runConfig) {
		cfg.maxConcurrency = n
	}
}

// WithMaxIterations sets the maximum number of supersteps before stopping.
// This option works with both Run and Resume.
func WithMaxIterations(n int) SharedOption {
	return func(cfg *runConfig) {
		cfg.maxIterations = n
	}
}

// WithCheckpointInterval sets how often checkpoints are saved.
// This option works with both Run and Resume.
func WithCheckpointInterval(interval int) SharedOption {
	return func(cfg *runConfig) {
		if interval > 0 {
			cfg.checkpointInterval = interval
		}
	}
}

// WithFailOnCheckpointError configures checkpoint error handling.
// This option works with both Run and Resume.
func WithFailOnCheckpointError(fail bool) SharedOption {
	return func(cfg *runConfig) {
		cfg.failOnCheckpointErr = fail
	}
}
