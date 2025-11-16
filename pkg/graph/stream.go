package graph

import "context"

// StreamWriter is a function that can emit node results during execution.
// This is used for streaming node outputs in real-time rather than
// waiting for the entire graph execution to complete.
//
// StreamWriter allows nodes to emit intermediate progress updates that
// are forwarded to the execution result stream. These intermediate updates
// are not applied to the graph state - they are purely for observation
// and user feedback.
type StreamWriter func(*NodeResult)

// streamWriterContextKey is the key for storing StreamWriter in context.
var streamWriterContextKey = &struct{}{}

// WithStreamWriter attaches a StreamWriter to a context.
// This allows nodes to emit results during execution via GetStreamWriter.
func WithStreamWriter(ctx context.Context, writer StreamWriter) context.Context {
	return context.WithValue(ctx, streamWriterContextKey, writer)
}

// GetStreamWriter retrieves the StreamWriter from a context if present.
// Nodes can use this to emit results in real-time during execution.
// Returns nil if no StreamWriter is attached to the context.
//
// Example usage in a node function:
//
//	func myNode(ctx context.Context, s state.Writer) (*NodeResult, error) {
//	    streamWriter := graph.GetStreamWriter(ctx)
//
//	    // Emit intermediate progress
//	    if streamWriter != nil {
//	        streamWriter(&NodeResult{
//	            Updates: map[string]any{"progress": "50%"},
//	        })
//	    }
//
//	    // Return final result
//	    return &NodeResult{Updates: map[string]any{"done": true}}, nil
//	}
func GetStreamWriter(ctx context.Context) StreamWriter {
	writer, _ := ctx.Value(streamWriterContextKey).(StreamWriter)
	return writer
}
