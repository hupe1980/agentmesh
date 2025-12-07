// Package graph provides a simplified API for building executable workflows.
package graph

import (
	"context"
	"errors"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
)

// END is the terminal node constant.
const END = "__end__"

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
// I = input type, O = output type.
type Builder[I, O any] struct {
	keys         []StateKey
	outputKey    string // Name of the key that produces outputs
	outputIsList bool   // True if output key is a ListKey
	nodes        map[string]*node
	entryPoints  []string
	interrupts   []interruptPoint

	// Configuration (set via With* methods)
	store        Store
	checkpointer checkpoint.Checkpointer
	runID        string
	middleware   []Middleware
	executor     Executor[I, O]
}

// Graph is an executable workflow with immutable structure.
// Created by calling Build() on a Builder.
type Graph[I, O any] struct {
	keys         []StateKey
	outputKey    string
	outputIsList bool
	nodes        map[string]*node
	entryPoints  []string
	interrupts   []interruptPoint

	store        Store
	checkpointer checkpoint.Checkpointer
	runID        string
	middleware   []Middleware
	executor     Executor[I, O]
}

// New creates a graph builder with the given state keys.
// Keys are automatically registered.
// The first key (if provided) is used as the output key.
// For ListKey[O], new items are yielded as outputs.
// For Key[O], the value is yielded when set.
// Duplicate keys will cause Build() to fail.
func New[I, O any](keys ...StateKey) *Builder[I, O] {
	var outputKey string
	var outputIsList bool
	if len(keys) > 0 {
		outputKey = keys[0].Name()
		outputIsList = keys[0].IsList()
	}

	return &Builder[I, O]{
		keys:         keys,
		outputKey:    outputKey,
		outputIsList: outputIsList,
		nodes:        make(map[string]*node),
	}
}

// Node adds a node to the graph.
// Targets are the possible next nodes (use END for terminal).
func (b *Builder[I, O]) Node(name string, fn NodeFunc, targets ...string) *Builder[I, O] {
	b.nodes[name] = &node{name: name, fn: fn, targets: targets}
	return b
}

// Subgraph creates a NodeFunc that executes a compiled subgraph.
// Use with Node() to add a subgraph while maintaining the fluent builder pattern:
//
//	child, _ := graph.New[ChildIn, ChildOut](...).Build()
//
//	parent.Node("validate", graph.Subgraph(child,
//	    func(ctx context.Context, view graph.View) (ChildIn, error) {
//	        return ChildIn{Data: graph.Get(view, dataKey)}, nil
//	    },
//	    func(ctx context.Context, out ChildOut) (graph.Updates, error) {
//	        return graph.Set(resultKey, out.Result), nil
//	    },
//	), "next")
//
// The InputMapper transforms parent state into subgraph input.
// The OutputMapper transforms subgraph output into parent state updates.
func Subgraph[SI, SO any](
	sub *Graph[SI, SO],
	inputMapper InputMapper[SI],
	outputMapper OutputMapper[SO],
) NodeFunc {
	return func(ctx context.Context, view View) (*Command, error) {
		// Map parent state to subgraph input
		input, err := inputMapper(ctx, view)
		if err != nil {
			return Fail(err)
		}

		// Execute subgraph (collect final output)
		var lastOutput SO
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

// Start sets the entry point node(s).
// Multiple entry points run in parallel.
func (b *Builder[I, O]) Start(names ...string) *Builder[I, O] {
	b.entryPoints = names
	return b
}

// InterruptBefore adds an interrupt before the specified node.
// When execution reaches this node, it will pause and yield an interrupt event.
func (b *Builder[I, O]) InterruptBefore(nodeName string, opts ...InterruptOption) *Builder[I, O] {
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
func (b *Builder[I, O]) InterruptAfter(nodeName string, opts ...InterruptOption) *Builder[I, O] {
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
func (b *Builder[I, O]) WithStore(store Store) *Builder[I, O] {
	b.store = store
	return b
}

// WithCheckpointer sets the checkpointer and run ID.
func (b *Builder[I, O]) WithCheckpointer(cp checkpoint.Checkpointer, runID string) *Builder[I, O] {
	b.checkpointer = cp
	b.runID = runID
	return b
}

// WithMiddleware adds middleware to the builder.
func (b *Builder[I, O]) WithMiddleware(mw ...Middleware) *Builder[I, O] {
	b.middleware = append(b.middleware, mw...)
	return b
}

// WithExecutor sets a custom executor.
func (b *Builder[I, O]) WithExecutor(exec Executor[I, O]) *Builder[I, O] {
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
func (b *Builder[I, O]) Build(opts ...BuildOption) (*Graph[I, O], error) {
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

	return &Graph[I, O]{
		keys:         b.keys,
		outputKey:    b.outputKey,
		outputIsList: b.outputIsList,
		nodes:        b.nodes,
		entryPoints:  b.entryPoints,
		interrupts:   b.interrupts,
		store:        store,
		checkpointer: b.checkpointer,
		runID:        b.runID,
		middleware:   b.middleware,
		executor:     b.executor,
	}, nil
}

// Run executes the compiled graph with input.
func (g *Graph[I, O]) Run(ctx context.Context, input I, opts ...RunOption) iter.Seq2[O, error] {
	return func(yield func(O, error) bool) {
		// Build executor config
		cfg := g.buildExecutorConfig()

		// Use configured executor or default Pregel executor
		exec := g.executor
		if exec == nil {
			exec = NewPregelExecutor[I, O]()
		}

		// Delegate to executor
		for output, err := range exec.Run(ctx, cfg, input, opts...) {
			if !yield(output, err) {
				return
			}
		}
	}
}

// buildExecutorConfig creates the executor configuration from the compiled graph.
func (g *Graph[I, O]) buildExecutorConfig() *ExecutorConfig[I, O] {
	// Build nodes map
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

	return &ExecutorConfig[I, O]{
		Nodes:            nodes,
		EntryPoints:      g.entryPoints,
		InterruptsBefore: interruptsBefore,
		InterruptsAfter:  interruptsAfter,
		Middleware:       g.middleware,
		Store:            g.store,
		Checkpointer:     g.checkpointer,
		RunID:            g.runID,
		OutputKey:        g.outputKey,
		OutputIsList:     g.outputIsList,
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
	managedValues       *managedValueRegistry
	runID               string
	maxConcurrency      int
	maxIterations       int
	checkpointInterval  int
	autoRestore         bool
	failOnCheckpointErr bool
}

// RunOption configures a Run call.
type RunOption func(*runConfig)

// WithCheckpoint resumes from a checkpoint.
func WithCheckpoint(cp *checkpoint.Checkpoint) RunOption {
	return func(cfg *runConfig) {
		cfg.checkpoint = cp
	}
}

// WithStateUpdates applies state updates to the graph execution.
// This works for both fresh runs (to set initial values) and checkpoint resumes
// (for human-in-the-loop workflows where you need to inject human input).
//
// For type-safe initial values, prefer [WithInitialValue] instead.
//
// Example (checkpoint resume):
//
//	compiled.Run(ctx, nil,
//	    graph.WithCheckpoint(savedCheckpoint),
//	    graph.WithStateUpdates(map[string]any{
//	        "answer": "Paris",
//	        "approved": true,
//	    }),
//	    graph.WithApproval("wait_for_input", approval),
//	)
func WithStateUpdates(updates map[string]any) RunOption {
	return func(cfg *runConfig) {
		cfg.stateUpdates = updates
	}
}

// WithResumeValue is a convenience function that combines WithStateUpdates and
// WithApproval for simple human-input scenarios. It sets a single state value
// and auto-approves the specified node to bypass its interrupt.
//
// This is ideal for "pause for input" workflows where you just need to inject
// a value and continue execution.
//
// Example:
//
//	compiled.Run(ctx, nil,
//	    graph.WithCheckpoint(savedCheckpoint),
//	    graph.WithResumeValue("wait_for_answer", answerKey.Name(), "Paris"),
//	)
func WithResumeValue(nodeName string, key string, value any) RunOption {
	return func(cfg *runConfig) {
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
	}
}

// WithApproval provides an approval response for a node.
func WithApproval(nodeName string, approval *ApprovalResponse) RunOption {
	return func(cfg *runConfig) {
		if cfg.approvals == nil {
			cfg.approvals = make(map[string]*ApprovalResponse)
		}
		cfg.approvals[nodeName] = approval
	}
}

// WithInitialValue sets an initial state value when starting graph execution.
// This is useful for passing runtime-specific values like session IDs or
// configuration that varies per execution.
//
// Unlike WithStateUpdates, this provides type safety through the Key type.
//
// Example:
//
//	compiled.Run(ctx, messages,
//	    graph.WithInitialValue(agent.SessionIDKey, "session-123"),
//	)
func WithInitialValue[T any](key Key[T], value T) RunOption {
	return func(cfg *runConfig) {
		if cfg.stateUpdates == nil {
			cfg.stateUpdates = make(map[string]any)
		}
		cfg.stateUpdates[key.Name()] = value
	}
}

// WithManagedValues attaches ephemeral runtime values to the graph execution.
// Managed values are NOT persisted in checkpoints and are ideal for:
//   - API keys and authentication tokens
//   - Session state
//   - Runtime metrics collectors
//   - Cached computed values
//
// Access managed values in nodes using graph.GetManaged(ctx, view, managedValue).
//
// Example:
//
//	apiKeyMV := graph.NewManagedValueWithDefault("api_key", os.Getenv("API_KEY"))
//	timeoutMV := graph.NewManagedValueWithDefault("timeout", 30*time.Second)
//
//	compiled.Run(ctx, input, graph.WithManagedValues(apiKeyMV, timeoutMV))
func WithManagedValues(values ...ManagedValue) RunOption {
	return func(cfg *runConfig) {
		if cfg.managedValues == nil {
			cfg.managedValues = newManagedValueRegistry()
		}
		for _, v := range values {
			cfg.managedValues.register(v)
		}
	}
}

// WithRunID sets the run ID for checkpointing.
func WithRunID(id string) RunOption {
	return func(cfg *runConfig) {
		cfg.runID = id
	}
}

// WithMaxConcurrency sets the maximum number of nodes that can execute in parallel.
// Default is 0 (unlimited). Higher values may improve throughput for I/O-bound nodes.
//
// Example:
//
//	graph.Run(ctx, input, graph.WithMaxConcurrency(8))
func WithMaxConcurrency(n int) RunOption {
	return func(cfg *runConfig) {
		cfg.maxConcurrency = n
	}
}

// WithMaxIterations sets the maximum number of supersteps before stopping execution.
// Prevents infinite loops in cyclic graphs. Default is 100.
//
// Example:
//
//	graph.Run(ctx, input, graph.WithMaxIterations(1000))
func WithMaxIterations(n int) RunOption {
	return func(cfg *runConfig) {
		cfg.maxIterations = n
	}
}

// WithCheckpointInterval sets how often checkpoints are saved (in supersteps).
// Default is 1 (every superstep). Higher values reduce I/O but increase
// potential data loss on failure.
//
// Example:
//
//	graph.Run(ctx, input, graph.WithCheckpointInterval(5))
func WithCheckpointInterval(interval int) RunOption {
	return func(cfg *runConfig) {
		if interval > 0 {
			cfg.checkpointInterval = interval
		}
	}
}

// WithAutoRestore enables automatic restoration from the latest checkpoint
// when a checkpointer is configured.
//
// Example:
//
//	graph.Run(ctx, input, graph.WithAutoRestore(true))
func WithAutoRestore(enabled bool) RunOption {
	return func(cfg *runConfig) {
		cfg.autoRestore = enabled
	}
}

// WithFailOnCheckpointError configures whether checkpoint save errors should
// fail the entire graph execution or just be logged as warnings.
//
// By default (false), checkpoint errors are logged but don't stop execution.
// Set to true for critical workflows where checkpoint integrity is required.
//
// Example:
//
//	graph.Run(ctx, input, graph.WithFailOnCheckpointError(true))
func WithFailOnCheckpointError(fail bool) RunOption {
	return func(cfg *runConfig) {
		cfg.failOnCheckpointErr = fail
	}
}
