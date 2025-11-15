package graph

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Executor is the interface for graph execution strategies.
// This allows pluggable execution backends while maintaining a consistent API.
//
// EXECUTION STRATEGIES:
//
//  1. Pregel BSP Execution (DEFAULT)
//     The default execution uses the Pregel bulk-synchronous parallel (BSP) runtime
//     for distributed, parallel execution with worker pools and message passing.
//     This is implemented internally via runWithOptions() in Compiled.
//     Benefits: High performance, distributed execution, worker pools, aggregators
//
//  2. SimpleGraphExecutor (DEBUGGING)
//     A sequential, single-threaded executor for debugging and testing.
//     Use WithExecutor(NewSimpleGraphExecutor()) when compiling to enable.
//     Benefits: Deterministic execution order, easier debugging, simpler traces
//
//  3. Custom Executors
//     Users can implement their own execution strategies by implementing this interface.
//     Examples: distributed executors with Kafka/Redis, specialized schedulers, etc.
//
// DESIGN GOALS:
//   - Clean separation between graph topology (Compiled) and execution strategy
//   - Pluggable backends without changing Compiled API
//   - Users interact with Compiled.Run() - executor choice is transparent
//
// USAGE:
//
//	// Default Pregel BSP execution (parallel, worker pools)
//	compiled, _ := builder.Compile()
//
//	// Switch to SimpleGraphExecutor for debugging (sequential)
//	compiled, _ := builder.Compile(WithExecutor(NewSimpleGraphExecutor()))
//
//	// Custom executor
//	compiled, _ := builder.Compile(WithExecutor(myCustomExecutor))
type Executor interface {
	// Run executes the graph and returns an iterator of execution events.
	// The executor is responsible for:
	//   - Coordinating node execution according to topology
	//   - Managing supersteps/synchronization barriers
	//   - Emitting events as execution progresses
	//   - Handling errors and cancellation
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - topology: Graph structure (nodes, edges, conditionals)
	//   - stateManager: State management interface
	//   - initialMessages: Starting messages for the graph
	//   - options: Execution configuration
	//
	// Returns: Iterator yielding (state.ExecutionResult, error) pairs
	Run(
		ctx context.Context,
		topology *ExecutorTopology,
		stateManager StateManager,
		initialMessages []message.Message,
		options *RunOptions,
	) iter.Seq2[state.ExecutionResult, error]

	// CurrentSuperstep returns the current superstep/iteration number.
	// Returns 0 for non-iterative executors.
	CurrentSuperstep() int64

	// Pause marks a node to pause before execution.
	Pause(nodeName string)

	// Resume clears the pause state for a node.
	Resume(nodeName string)

	// IsPaused checks if a node is currently paused.
	IsPaused(nodeName string) bool
}

// ExecutorTopology contains the immutable graph structure.
// This is passed to executors to avoid tight coupling with Compiled internals.
type ExecutorTopology struct {
	// Nodes maps node names to Node definitions
	Nodes map[string]*Node

	// Edges is the list of unconditional edges
	Edges []Edge

	// Conditionals is the list of conditional edge sets
	Conditionals []ConditionalEdges

	// Incoming tracks the number of incoming edges per node (for topological sorting)
	Incoming map[string]int

	// ConditionalGate marks nodes that have conditional incoming edges
	ConditionalGate map[string]bool

	// Outgoing maps each node to its outgoing node names
	Outgoing map[string][]string

	// ConditionalByFrom maps source nodes to their conditional edge sets
	ConditionalByFrom map[string][]ConditionalEdges

	// NodeNames is an ordered list of node names (for deterministic iteration)
	NodeNames []string

	// StartKey identifies the starting node
	StartKey string

	// EndKey identifies the ending node
	EndKey string
}

// RunOptions contains execution configuration.
// This is the public API for executor configuration.
// Contains a subset of runOptions fields plus internal reference for full config.
type RunOptions struct {
	MaxIterations      int
	MaxConcurrency     int
	RunID              string
	RecursionLimit     int
	CheckpointInterval int
	Checkpointer       interface{} // checkpoint.Checkpointer

	// internal carries the full runOptions from Compiled.Run()
	// This allows executors to access all configuration (validation limits, observability, etc.)
	// without exposing internal details in the public API
	internal *runOptions
}
