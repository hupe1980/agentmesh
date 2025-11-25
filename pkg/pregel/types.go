package pregel

import "context"

// Graph defines the minimal contract the runtime needs to traverse a graph.
// Generic over global state `S` and message payload `M`.
type Graph[S any, M any] interface {
	RootVertices() []string
	Outgoing(vertex string) []string
	VertexByName(name string) Vertex[S, M]
	State() S
}

// Aggregator combines per-vertex contributions into a single value that becomes
// visible to all vertices at the start of the next superstep. Implementations
// should be deterministic and treat Zero as an identity element for Aggregate.
type Aggregator interface {
	Zero() any
	Aggregate(current, value any) any
}

// Combiner collapses multiple in-flight messages for the same recipient during a
// superstep into a single value, reducing mailbox pressure. It is invoked
// synchronously while messages are recorded and can safely mutate the returned
// Message instance.
type Combiner[M any] func(existing, incoming Message[M]) Message[M]

// VertexContext exposes runtime services to a vertex implementation during Run.
// Aggregate is nil when no aggregators are configured, and Aggregates contains
// the completed reductions from the previous superstep. Send may be called zero
// or more times to enqueue messages for the next superstep.
type VertexContext[S any, M any] struct {
	State      S
	Send       func(Message[M])
	Aggregate  func(name string, value any) error
	Aggregates map[string]any
}

// Vertex represents a unit of computation in the BSP model.
// Each vertex receives messages, performs computation, and can send messages to other vertices.
type Vertex[S any, M any] interface {
	Name() string
	Run(ctx context.Context, vertex VertexContext[S, M], incoming []Message[M]) error
}

// Message represents a typed, directed message sent between vertices.
type Message[M any] struct {
	From string
	To   string
	Data M
}

// Event can be used to observe runtime progress during BSP execution.
//
// ERROR HANDLING CONTRACT:
//
// When iterating over runtime events:
//
//	for evt, err := range runtime.Run(ctx) {
//	    if err != nil {
//	        // Fatal error - BSP execution terminated
//	        // Examples: context canceled, max iterations exceeded, quota exceeded
//	        return err
//	    }
//	    // Process event (superstep progress, vertex output, diagnostics)
//	}
//
// For non-fatal vertex-level errors (where execution should continue),
// use evt.Output to pass error information rather than the iterator's error return.
type Event[M any] struct {
	Vertex      string
	Superstep   int64
	Output      any // Vertex output (can include error information for non-fatal failures)
	Diagnostics any // Debug/diagnostic information
}
