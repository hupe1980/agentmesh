package graph

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/hupe1980/agentmesh/pkg/state"
)

// ============================================================================
// Constants and Errors
// ============================================================================

// Reserved node names for graph entry and exit points.
// All graphs implicitly start from StartNode and end at EndNode.
const (
	// StartNode is the reserved node name for graph entry points.
	// All graphs implicitly start from this node.
	StartNode = "__start__"

	// EndNode is the reserved node name for graph exit points.
	// All graphs implicitly end at this node.
	EndNode = "__end__"
)

// MessagesKeyName is the standard key name for storing conversation messages.
// This is used by both the graph executor and agent package to ensure consistency.
const MessagesKeyName = "__messages__"

// ErrHumanInterrupt signals that a node requires human input before continuing.
// When a node returns this error, graph execution pauses at that node,
// allowing external systems to provide input and resume execution.
//
// Example usage:
//
//	func approvalNode(ctx context.Context, s state.Writer) (*NodeResult, error) {
//	    if s.Get("approved") == nil {
//	        return nil, graph.ErrHumanInterrupt
//	    }
//	    // Process with approval
//	    return &NodeResult{...}, nil
//	}
var ErrHumanInterrupt = errors.New("human interrupt: execution paused for user input")

// ErrNodeExecution is a sentinel error that wraps node execution failures.
// Used to distinguish node-level errors from system-level errors in execution.
var ErrNodeExecution = errors.New("node execution failed")

// NodeExecutionError wraps node execution failures with context about which node failed.
// This type preserves error unwrapping capabilities while providing structured error information.
type NodeExecutionError struct {
	NodeName string
	Err      error
}

// Error implements the error interface.
func (e *NodeExecutionError) Error() string {
	return fmt.Sprintf("node %q: %v", e.NodeName, e.Err)
}

// Unwrap returns the wrapped error for errors.Is/As compatibility.
func (e *NodeExecutionError) Unwrap() error {
	return e.Err
}

// Is checks if the target error is ErrNodeExecution.
func (e *NodeExecutionError) Is(target error) bool {
	return target == ErrNodeExecution
}

// ============================================================================
// Executor Interface
// ============================================================================

// Executor defines a strategy for executing a compiled graph.
// Generic over input and output types to provide type-safe execution.
//
// Implementers:
//   - PregelExecutor: BSP-based parallel execution (default)
//   - SequentialExecutor: Simple sequential execution for debugging
//
// Type parameters:
//   - I: Input type accepted by the executor
//   - O: Output type produced by the executor
type Executor[I, O any] interface {
	// Run executes the compiled graph with the given input.
	// Returns an iterator that yields outputs as execution progresses.
	//
	// The iterator approach allows:
	//   - Streaming results as they become available
	//   - Early termination on errors
	//   - Resource cleanup via context cancellation
	//
	// Uses RunOptions from runnable.go for configuration.
	Run(ctx context.Context, compiled *Compiled[I, O], input I, opts ...RunOption) iter.Seq2[O, error]
}

// ============================================================================
// Context Utilities
// ============================================================================

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	// resumeValueKey is the context key for resume values
	resumeValueKey contextKey = "resumeValue"
)

// withResumeValueContext attaches a resume value map to the context.
// This is an internal function used by the executor to inject resume values.
// Use WithResumeValue(map[string]any) RunOption instead for public API.
//
// Resume values are injected when resuming from a checkpoint and allow
// nodes to receive external input (e.g., human approval, edits) when continuing execution.
func withResumeValueContext(ctx context.Context, value map[string]any) context.Context {
	return context.WithValue(ctx, resumeValueKey, value)
}

// ResumeValueFromContext retrieves the resume value map from the context.
// Returns nil if no resume value was set (normal execution, not resumed).
//
// Nodes can check for resume values to handle human-in-the-loop scenarios:
//
//	func (n *MyNode) Execute(ctx context.Context, view state.ReadView) (state.Updates, error) {
//	    if resume := graph.ResumeValueFromContext(ctx); resume != nil {
//	        // Resuming with human input
//	        if approval, ok := resume["approval"]; ok {
//	            if approval == "APPROVED" {
//	                // Execute approved action
//	                return executeAction(view)
//	            }
//	            // Handle rejection
//	            return state.Updates{"status": "rejected"}, nil
//	        }
//	    }
//
//	    // Normal execution (not resumed)
//	    return n.normalExecution(view)
//	}
func ResumeValueFromContext(ctx context.Context) map[string]any {
	if value := ctx.Value(resumeValueKey); value != nil {
		if resumeMap, ok := value.(map[string]any); ok {
			return resumeMap
		}
	}
	return nil
}

// ============================================================================
// Stream Writer
// ============================================================================

// StreamWriter is a function that can emit node updates during execution.
// This is used for streaming node outputs in real-time rather than
// waiting for the entire graph execution to complete.
//
// StreamWriter allows nodes to emit intermediate progress updates that
// are forwarded to the execution result stream. These intermediate updates
// are not applied to the graph state - they are purely for observation
// and user feedback.
type StreamWriter func(state.Updates)

// streamWriterContextKey is the key for storing StreamWriter in context.
var streamWriterContextKey = &struct{}{}

// WithStreamWriter attaches a StreamWriter to a context.
// This allows nodes to emit results during execution via GetStreamWriter.
func WithStreamWriter(ctx context.Context, writer StreamWriter) context.Context {
	return context.WithValue(ctx, streamWriterContextKey, writer)
}

// GetStreamWriter retrieves the StreamWriter from a context if present.
// Nodes can use this to emit updates in real-time during execution.
// Returns nil if no StreamWriter is attached to the context.
//
// Example usage in a node function:
//
//	func myNode(ctx context.Context, view state.ReadView) (state.Updates, error) {
//	    streamWriter := graph.GetStreamWriter(ctx)
//
//	    // Emit intermediate progress
//	    if streamWriter != nil {
//	        streamWriter(state.Updates{"progress": "50%"})
//	    }
//
//	    // Return final result
//	    return state.Updates{"done": true}, nil
//	}
func GetStreamWriter(ctx context.Context) StreamWriter {
	writer, _ := ctx.Value(streamWriterContextKey).(StreamWriter)
	return writer
}
