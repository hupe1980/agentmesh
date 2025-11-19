package graph

import "context"

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
//	func (n *MyNode) Execute(ctx context.Context, view *state.ReadView) (state.Updates, error) {
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
