package testutil

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/pregel"
)

// MockNode is a mock implementation of pregel.Vertex for testing BSP graphs.
type MockNode[S any, M any] struct {
	NameValue string
	RunFunc   func(ctx context.Context, vertex pregel.VertexContext[S, M], incoming []pregel.Message[M]) error
	callCount int
	messages  []pregel.Message[M]
	mu        sync.Mutex
}

// Name returns the vertex's identifier.
func (n *MockNode[S, M]) Name() string {
	return n.NameValue
}

// Run executes the vertex's computation.
func (n *MockNode[S, M]) Run(ctx context.Context, vertex pregel.VertexContext[S, M], incoming []pregel.Message[M]) error {
	n.mu.Lock()
	n.callCount++
	n.messages = append(n.messages, incoming...)
	n.mu.Unlock()

	if n.RunFunc != nil {
		return n.RunFunc(ctx, vertex, incoming)
	}
	return nil
}

// CallCount returns the number of times Run was called.
func (n *MockNode[S, M]) CallCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.callCount
}

// ReceivedMessages returns all messages received by this node.
func (n *MockNode[S, M]) ReceivedMessages() []pregel.Message[M] {
	n.mu.Lock()
	defer n.mu.Unlock()
	result := make([]pregel.Message[M], len(n.messages))
	copy(result, n.messages)
	return result
}

// Reset clears the call count and messages.
func (n *MockNode[S, M]) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.callCount = 0
	n.messages = nil
}

// MockGraph is a mock implementation of pregel.Graph for testing.
type MockGraph[S any, M any] struct {
	rootVertices  []string
	vertices      map[string]pregel.Vertex[S, M]
	outgoingEdges map[string][]string
	state         S
	mu            sync.Mutex
}

// NewMockGraph creates a new MockGraph with the given configuration.
func NewMockGraph[S any, M any](rootVertices []string, vertices map[string]pregel.Vertex[S, M], edges map[string][]string, state S) *MockGraph[S, M] {
	return &MockGraph[S, M]{
		rootVertices:  rootVertices,
		vertices:      vertices,
		outgoingEdges: edges,
		state:         state,
	}
}

// RootVertices returns the initial vertices to activate.
func (g *MockGraph[S, M]) RootVertices() []string {
	return g.rootVertices
}

// Outgoing returns the destination vertices for a given vertex.
func (g *MockGraph[S, M]) Outgoing(vertex string) []string {
	return g.outgoingEdges[vertex]
}

// VertexByName returns the vertex with the given name.
func (g *MockGraph[S, M]) VertexByName(name string) pregel.Vertex[S, M] {
	return g.vertices[name]
}

// State returns the current graph state.
func (g *MockGraph[S, M]) State() S {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

// GraphBuilder provides a fluent API for building test graphs.
type GraphBuilder[S any, M any] struct {
	rootVertices  []string
	vertices      map[string]pregel.Vertex[S, M]
	outgoingEdges map[string][]string
	state         S
}

// NewGraphBuilder creates a new GraphBuilder.
func NewGraphBuilder[S any, M any](state S) *GraphBuilder[S, M] {
	return &GraphBuilder[S, M]{
		vertices:      make(map[string]pregel.Vertex[S, M]),
		outgoingEdges: make(map[string][]string),
		state:         state,
	}
}

// WithRoot adds a root vertex.
func (b *GraphBuilder[S, M]) WithRoot(name string, vertex pregel.Vertex[S, M]) *GraphBuilder[S, M] {
	b.rootVertices = append(b.rootVertices, name)
	b.vertices[name] = vertex
	return b
}

// WithVertex adds a non-root vertex.
func (b *GraphBuilder[S, M]) WithVertex(name string, vertex pregel.Vertex[S, M]) *GraphBuilder[S, M] {
	b.vertices[name] = vertex
	return b
}

// WithEdge adds an edge from source to target.
func (b *GraphBuilder[S, M]) WithEdge(source, target string) *GraphBuilder[S, M] {
	b.outgoingEdges[source] = append(b.outgoingEdges[source], target)
	return b
}

// WithChain adds a chain of vertices (first is root).
func (b *GraphBuilder[S, M]) WithChain(names ...string) *GraphBuilder[S, M] {
	for i, name := range names {
		node := &MockNode[S, M]{NameValue: name}
		if i == 0 {
			b.WithRoot(name, node)
		} else {
			b.WithVertex(name, node)
			b.WithEdge(names[i-1], name)
		}
	}
	return b
}

// Build creates the MockGraph.
func (b *GraphBuilder[S, M]) Build() *MockGraph[S, M] {
	return NewMockGraph(b.rootVertices, b.vertices, b.outgoingEdges, b.state)
}

// NewTestContext creates a context with a timeout for tests.
func NewTestContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// FormatMessageHistory formats a slice of messages for debugging output.
func FormatMessageHistory(messages []message.Message) string {
	if len(messages) == 0 {
		return "<empty>"
	}

	result := fmt.Sprintf("%d messages:\n", len(messages))
	for i, msg := range messages {
		content := msg.String()
		if len(content) > 50 {
			content = content[:47] + "..."
		}
		result += fmt.Sprintf("  [%d] %s: %s\n", i, msg.Type(), content)
	}
	return result
}

// WaitForCondition waits for a condition to become true or context to be canceled.
func WaitForCondition(ctx context.Context, condition func() bool, checkInterval time.Duration) bool {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		if condition() {
			return true
		}

		if ctx.Err() != nil {
			return false
		}

		<-ticker.C
	}
}
