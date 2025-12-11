package message

import "github.com/hupe1980/agentmesh/pkg/graph"

// NodeFunc is the typed signature for message-processing node logic.
// Read state via Scope, optionally stream partial messages, return a Command.
//
// Example:
//
//	func myNode(ctx context.Context, scope message.Scope) (*graph.Command, error) {
//	    messages := message.GetMessages(scope)
//	    scope.Stream(partialMessage)  // Stream partial output
//	    return graph.Set(message.MessagesKey, []message.Message{finalMessage}).End()
//	}
type NodeFunc = graph.NodeFunc[Message]

// Scope provides the execution context for a message-processing node.
// It combines state access (read) with typed message streaming (write).
//
// Scope embeds graph.Scope[Message], providing:
//   - GetValue(name string) (any, bool) - Access raw state values
//   - ManagedValues() - Access managed values registry
//   - ToMap() map[string]any - Get state as map for templates
//   - Stream(value Message) - Emit partial messages immediately
type Scope = graph.Scope[Message]

// Middleware wraps a message-processing node function with additional behavior.
// Middleware is applied in reverse order: the last middleware added runs first.
//
// Example:
//
//	loggingMiddleware := func(next message.NodeFunc) message.NodeFunc {
//	    return func(ctx context.Context, scope message.Scope) (*graph.Command, error) {
//	        log.Printf("entering node")
//	        return next(ctx, scope)
//	    }
//	}
type Middleware = graph.Middleware[Message]

// GraphBuilder is a fluent builder for message-processing workflows.
// This is the standard builder type for conversational agents.
//
// GraphBuilder provides type-safe message handling with compile-time guarantees:
//   - Input: []Message (message history)
//   - Output: Message (final response)
//   - State: Automatically includes MessagesKey
//
// Example:
//
//	b := message.NewGraphBuilder()
//	b.Node("agent", func(ctx context.Context, scope message.Scope) (*graph.Command, error) {
//	    msgs := message.GetMessages(scope)
//	    response := processMessages(msgs)
//	    return graph.Set(message.MessagesKey, []message.Message{response}).End()
//	})
type GraphBuilder = graph.Builder[[]Message, Message]

// Graph is an executable message-processing workflow with immutable structure.
// After compilation, the graph structure cannot be modified, ensuring deterministic execution.
//
// Use Run() or Stream() methods to execute the graph with message inputs.
//
// Example:
//
//	g, err := b.Build()
//	if err != nil {
//	    return err
//	}
//
//	for msg, err := range g.Run(ctx, []message.Message{userMsg}) {
//	    if err != nil {
//	        return err
//	    }
//	    fmt.Println(msg.Content())
//	}
type Graph = graph.Graph[[]Message, Message]

// NewGraphBuilder creates a message-processing graph builder for conversational agents.
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
//   - *GraphBuilder: A graph builder configured for message processing
//
// Example - Basic message builder:
//
//	b := message.NewGraphBuilder()
//	b.Node("process", processNode).
//	  Start("process")
//	g, err := b.Build()
//
// Example - With custom state keys:
//
//	CategoryKey := graph.NewKey("category", "")
//	b := message.NewGraphBuilder(CategoryKey)
//	b.Node("classify", func(ctx context.Context, scope message.Scope) (*graph.Command, error) {
//	    category := graph.Get(scope, CategoryKey)
//	    messages := message.GetMessages(scope)
//	    // Classify messages by category...
//	})
func NewGraphBuilder(additionalKeys ...graph.StateKey) *GraphBuilder {
	allKeys := append([]graph.StateKey{MessagesKey}, additionalKeys...)
	return graph.New[[]Message, Message](allKeys...)
}
