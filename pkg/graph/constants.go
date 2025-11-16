package graph

import "errors"

// Reserved node names for graph entry and exit points.
// These constants are re-exported from pkg/compile for convenience,
// allowing agent code to use graph.StartNode instead of compile.StartNode.
const (
	// StartNode is the reserved node name for graph entry points.
	// All graphs implicitly start from this node.
	StartNode = "__start__"

	// EndNode is the reserved node name for graph exit points.
	// All graphs implicitly end at this node.
	EndNode = "__end__"
)

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
