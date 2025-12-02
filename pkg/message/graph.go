package message

import "github.com/hupe1980/agentmesh/pkg/graph"

// MessageGraph is a graph builder that processes message sequences.
// This is the standard type for conversational agents.
//
// MessageGraph provides type-safe message handling with compile-time guarantees:
//   - Input: []Message (message history)
//   - Output: Message (final response)
//   - State: Automatically includes MessagesKey
//
// Example:
//
//	g := message.NewGraph()
//	g.Node("agent", func(ctx context.Context, view graph.View) (*graph.Command, error) {
//	    msgs := message.GetMessages(view)
//	    response := processMessages(msgs)
//	    return graph.Append(message.MessagesKey, response).End()
//	})
//
//nolint:revive // MessageGraph is the conventional name for message-based graphs
type MessageGraph = graph.Graph[[]Message, Message]

// CompiledMessageGraph is an executable message graph with immutable structure.
// After compilation, the graph structure cannot be modified, ensuring deterministic execution.
//
// Use Run() or Stream() methods to execute the graph with message inputs.
//
// Example:
//
//	compiled, err := g.Build()
//	if err != nil {
//	    return err
//	}
//
//	for msg, err := range compiled.Run(ctx, []message.Message{userMsg}) {
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Println(msg.Content())
//	}
type CompiledMessageGraph = graph.CompiledGraph[[]Message, Message]

// NewGraph creates a message-processing graph for conversational agents.
// Automatically includes MessagesKey in state. Additional keys can be provided.
//
// This is a convenience wrapper that:
//   - Sets up proper input/output types for messages
//   - Includes MessagesKey by default for message history
//   - Allows custom state keys for agent-specific data
//
// Parameters:
//   - additionalKeys: Optional state keys for custom data (e.g., DocumentsKey, StatusKey)
//
// Returns:
//   - *MessageGraph: A graph builder configured for message processing
//
// Example - Basic message graph:
//
//	g := message.NewGraph()
//	g.Node("process", processNode).
//	  Start("process")
//	compiled, err := g.Build()
//
// Example - With custom state keys:
//
//	CategoryKey := graph.NewKey("category", "")
//	g := message.NewGraph(CategoryKey)
//	g.Node("classify", func(ctx context.Context, view graph.View) (*graph.Command, error) {
//	    category := graph.Get(view, CategoryKey)
//	    messages := message.GetMessages(view)
//	    // Classify messages by category...
//	})
func NewGraph(additionalKeys ...graph.StateKey) *MessageGraph {
	allKeys := append([]graph.StateKey{MessagesKey}, additionalKeys...)
	return graph.New[[]Message, Message](allKeys...)
}
