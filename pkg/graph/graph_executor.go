package graph

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// Executor is an internal interface for graph execution strategies.
// This is used internally by Compiled to delegate execution to different
// backends (Pregel BSP, Sequential, etc.).
//
// Design Goals:
//   - Clean separation between graph topology (Compiled) and execution strategy
//   - Pluggable backends without changing Compiled API
//   - No direct Pregel coupling in Compiled
//
// Implementations:
//   - SimpleGraphExecutor: Sequential topological execution for debugging
//   - Custom executors: Users can implement their own strategies
//
// Note: This is an internal interface. Users interact with Compiled.Run() directly.
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
	// Returns: Iterator yielding (ExecutionResult, error) pairs
	Run(
		ctx context.Context,
		topology *ExecutorTopology,
		stateManager StateManager,
		initialMessages []message.Message,
		options *RunOptions,
	) iter.Seq2[ExecutionResult, error]

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
// This is an internal structure matching the external ExecuteOptions.
type RunOptions struct {
	MaxIterations      int
	MaxConcurrency     int
	RunID              string
	RecursionLimit     int
	CheckpointInterval int
	Checkpointer       interface{} // checkpoint.Checkpointer
}
