package graph

import "context"

// streamWriterKey is the context key for the stream writer function.
type streamWriterKey struct{}

// StreamWriter is a function that emits intermediate updates during node execution.
// Nodes can use this to stream progress updates before they complete.
// The updates are published as events and yielded to the graph output iterator.
type StreamWriter func(Updates)

// WithStreamWriter attaches a stream writer to the context.
// This is called automatically by the executor to enable intermediate streaming.
func WithStreamWriter(ctx context.Context, writer StreamWriter) context.Context {
	return context.WithValue(ctx, streamWriterKey{}, writer)
}

// GetStreamWriter retrieves the stream writer from context.
// Returns nil if streaming is not available (e.g., outside of graph execution).
//
// Example usage in a node:
//
//	func myNode(ctx context.Context, view graph.View) (*graph.Command, error) {
//	    streamWriter := graph.GetStreamWriter(ctx)
//
//	    for i, chunk := range chunks {
//	        // Process chunk...
//
//	        // Stream intermediate progress
//	        if streamWriter != nil {
//	            streamWriter(graph.Updates{
//	                "progress": fmt.Sprintf("%d/%d", i+1, len(chunks)),
//	                "current_chunk": chunk,
//	            })
//	        }
//	    }
//
//	    return graph.Set(statusKey, "done").End()
//	}
func GetStreamWriter(ctx context.Context) StreamWriter {
	if writer, ok := ctx.Value(streamWriterKey{}).(StreamWriter); ok {
		return writer
	}
	return nil
}
