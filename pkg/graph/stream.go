package graph

import "context"

// streamWriterKey is the context key for the stream writer function.
type streamWriterKey struct{}

// StreamWriter is a function that emits intermediate updates during node execution.
// This remains for backward compatibility with tools that need context-based streaming.
//
// Deprecated: Use Scope.Stream() instead for typed streaming.
type StreamWriter func(Updates)

// WithStreamWriter attaches a stream writer to the context.
// This is kept for tools that need to access streaming via context.
//
// Deprecated: Node functions now receive Scope[O] with typed Stream() method.
func WithStreamWriter(ctx context.Context, writer StreamWriter) context.Context {
	return context.WithValue(ctx, streamWriterKey{}, writer)
}

// GetStreamWriter retrieves the stream writer from context.
// This is kept for tools that need to access streaming via context.
//
// Deprecated: Use GetScope[O](ctx).Stream() instead for typed streaming.
//
// For typed streaming in nodes, use Scope.Stream() directly:
//
//	func myNode(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
//	    scope.Stream(partialMessage) // Type-safe!
//	    return graph.Append(key, finalMessage).End()
//	}
//
// For tools that need context-based streaming (non-typed):
//
//	func (t *MyTool) Run(ctx context.Context, input string) (string, error) {
//	    if sw := graph.GetStreamWriter(ctx); sw != nil {
//	        sw(graph.Updates{"progress": "50%"})
//	    }
//	    return result, nil
//	}
func GetStreamWriter(ctx context.Context) StreamWriter {
	if writer, ok := ctx.Value(streamWriterKey{}).(StreamWriter); ok {
		return writer
	}
	return nil
}
