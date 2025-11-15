package graph

import (
	"context"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Runnable represents any executable component that can process input
// and stream results. This includes compiled graphs, agents, tools,
// and subgraphs.
//
// Type Parameters:
//   - I: Input type (e.g., []message.Message, map[string]any, string)
//   - O: Output type (e.g., state.ExecutionResult, message.Message)
//
// The iterator pattern enables:
//   - Lazy evaluation (start before all results are ready)
//   - Streaming (process results as they arrive)
//   - Cancellation (via context)
//   - Error handling (each element can be success or error)
//
// Example usage:
//
//	agent, err := agent.NewReActAgent(model)
//	if err != nil {
//	    return err
//	}
//
//	for result, err := range agent.Run(ctx, messages) {
//	    if err != nil {
//	        return err
//	    }
//	    // Process result
//	}
type Runnable[I, O any] interface {
	// Run executes the component with the given input and returns
	// an iterator of output events.
	//
	// The iterator yields (output, error) pairs. On error, iteration
	// should stop (error != nil signals termination).
	//
	// Options can be used to configure execution (max iterations,
	// concurrency, checkpointing, etc.).
	Run(ctx context.Context, input I, opts ...RunOption) iter.Seq2[O, error]
}

// MessageRunnable is the most common Runnable type, processing message
// sequences and streaming execution results. This is the interface returned
// by all built-in agent constructors (ReAct, Supervisor, RAG).
//
// Example:
//
//	var agent graph.MessageRunnable
//	agent, err := agent.NewReActAgent(model)
type MessageRunnable = Runnable[[]message.Message, state.ExecutionResult]

// StateRunnable processes arbitrary state maps and streams execution results.
// Useful for graphs that work with structured state rather than message sequences.
//
// Example:
//
//	var processor graph.StateRunnable
//	processor = myCustomGraph
//	for result, err := range processor.Run(ctx, initialState) {
//	    // ...
//	}
type StateRunnable = Runnable[map[string]any, state.ExecutionResult]

// StringRunnable processes text input and returns text output.
// Useful for simple text-to-text transformations.
type StringRunnable = Runnable[string, string]

// StatefulRunnable extends Runnable with state access capabilities.
// This interface is useful for tests and debugging that need to inspect graph state.
type StatefulRunnable[I, O any] interface {
	Runnable[I, O]

	// State returns the state manager for inspection and testing.
	State() StateManager

	// CurrentSuperstep returns the current execution superstep.
	CurrentSuperstep() int64
}
