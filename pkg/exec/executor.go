package exec

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Executor defines the interface for graph execution strategies.
// Executors take a compiled graph and execute it, coordinating node
// execution according to the topology.
type Executor interface {
	// Run executes the compiled graph and returns an iterator of execution results.
	// The executor coordinates node execution, manages state, and handles errors.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeouts
	//   - compiled: The compiled graph to execute
	//   - initialMessages: Starting messages for the graph
	//   - opts: Optional run options (checkpointing, concurrency, etc.)
	//
	// Returns: Iterator yielding (ExecutionResult, error) pairs
	Run(
		ctx context.Context,
		compiled *compile.CompiledGraph,
		initialMessages []message.Message,
		opts ...graph.RunOption,
	) iter.Seq2[state.ExecutionResult, error]
}
