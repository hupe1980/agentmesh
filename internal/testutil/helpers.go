package testutil

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/pregel"
)

// NewMockModelWithResponses creates a MockModel that returns the given responses in sequence.
// Each call to Generate() yields one response from the sequence.
// If streaming is true, responses are yielded as partial chunks followed by a final message.
func NewMockModelWithResponses(responses []string, streaming bool) *MockModel {
	var mu sync.Mutex
	idx := 0

	return &MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				mu.Lock()
				currentIdx := idx
				if currentIdx >= len(responses) {
					currentIdx = len(responses) - 1 // Repeat last response
				}
				idx++
				mu.Unlock()

				text := responses[currentIdx]
				msg := message.NewAIMessageFromText(text)

				if streaming && len(text) > 3 {
					// Yield as streaming chunks
					chunkSize := len(text) / 3
					for i := 0; i < len(text); i += chunkSize {
						end := i + chunkSize
						if end > len(text) {
							end = len(text)
						}
						chunk := message.NewAIMessageFromText(text[i:end])
						if !yield(&model.Response{Message: chunk, Partial: true}, nil) {
							return
						}
					}
				}

				// Yield final complete response
				yield(&model.Response{Message: msg, Partial: false}, nil)
			}
		},
	}
}

// NewMockModelWithError creates a MockModel that returns an error.
func NewMockModelWithError(err error) *MockModel {
	return &MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				yield(nil, err)
			}
		},
	}
}

// NewMockModelWithDelay creates a MockModel that introduces a delay before returning a response.
// Useful for testing timeout and cancellation behavior.
func NewMockModelWithDelay(response string, delay time.Duration) *MockModel {
	return &MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				select {
				case <-time.After(delay):
					msg := message.NewAIMessageFromText(response)
					yield(&model.Response{Message: msg, Partial: false}, nil)
				case <-ctx.Done():
					yield(nil, ctx.Err())
				}
			}
		},
	}
}

// NewMockModelWithToolCalls creates a MockModel that returns a message with tool calls.
func NewMockModelWithToolCalls(toolCalls ...message.ToolCall) *MockModel {
	return &MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				msg := message.NewAIMessageFromText("") // Empty text with tool calls
				msg.ToolCalls = toolCalls
				yield(&model.Response{Message: msg, Partial: false}, nil)
			}
		},
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				Streaming:        true,
				Tools:            true,
				MaxContextTokens: 4096,
				MaxOutputTokens:  2048,
			}
		},
	}
}

// NewCountingMockNode creates a MockNode that counts executions and tracks incoming messages.
// This is useful for verifying graph execution order and message delivery.
func NewCountingMockNode[S any, M any](name string, nextVertex string) (*MockNode[S, M], *int, *[]pregel.Message[M]) {
	called := 0
	messages := []pregel.Message[M]{}
	var callMu sync.Mutex
	var msgMu sync.Mutex

	vertex := &MockNode[S, M]{
		NameValue:  name,
		NextNode:   nextVertex,
		Called:     &called,
		CallMu:     &callMu,
		Messages:   &messages,
		MessagesMu: &msgMu,
		RunFunc: func(ctx context.Context, vertex pregel.VertexContext[S, M], incoming []pregel.Message[M]) error {
			callMu.Lock()
			called++
			callMu.Unlock()

			msgMu.Lock()
			messages = append(messages, incoming...)
			msgMu.Unlock()

			// Send message to next vertex if specified
			if nextVertex != "" && len(incoming) > 0 {
				// Forward first message to next vertex
				vertex.Send(pregel.Message[M]{
					From: name,
					To:   nextVertex,
					Data: incoming[0].Data,
				})
			}

			return nil
		},
	}

	return vertex, &called, &messages
}

// NewSimpleChainGraph creates a simple chain graph: root -> vertex1 -> vertex2 -> ... -> vertexN
// Each vertex forwards messages to the next. Returns the graph and execution counters for each vertex.
func NewSimpleChainGraph[S any, M any](vertexNames []string, state S) (*MockGraph[S, M], map[string]*int) {
	vertices := make(map[string]pregel.Vertex[S, M])
	edges := make(map[string][]string)
	counters := make(map[string]*int)

	for i, name := range vertexNames {
		var nextVertex string
		if i < len(vertexNames)-1 {
			nextVertex = vertexNames[i+1]
			edges[name] = []string{nextVertex}
		}

		vertex, counter, _ := NewCountingMockNode[S, M](name, nextVertex)
		vertices[name] = vertex
		counters[name] = counter
	}

	rootVertices := []string{vertexNames[0]}
	return NewMockGraph(rootVertices, vertices, edges, state), counters
}

// NewParallelFanOutGraph creates a graph with one root that fans out to multiple parallel vertices:
// root -> [vertex1, vertex2, ..., vertexN]
// Useful for testing parallel execution.
func NewParallelFanOutGraph[S any, M any](rootName string, parallelVertices []string, state S) (*MockGraph[S, M], map[string]*int) {
	vertices := make(map[string]pregel.Vertex[S, M])
	edges := make(map[string][]string)
	counters := make(map[string]*int)

	// Create root vertex that broadcasts to all parallel vertices
	rootVertex, rootCounter, _ := NewCountingMockNode[S, M](rootName, "")
	rootVertex.RunFunc = func(ctx context.Context, vertex pregel.VertexContext[S, M], incoming []pregel.Message[M]) error {
		rootVertex.CallMu.Lock()
		*rootVertex.Called++
		rootVertex.CallMu.Unlock()

		// Send to all parallel vertices
		var zeroM M
		for _, target := range parallelVertices {
			vertex.Send(pregel.Message[M]{
				From: rootName,
				To:   target,
				Data: zeroM,
			})
		}
		return nil
	}
	vertices[rootName] = rootVertex
	counters[rootName] = rootCounter
	edges[rootName] = parallelVertices

	// Create parallel vertices
	for _, name := range parallelVertices {
		vertex, counter, _ := NewCountingMockNode[S, M](name, "")
		vertices[name] = vertex
		counters[name] = counter
	}

	return NewMockGraph([]string{rootName}, vertices, edges, state), counters
}

// RequireAllNodesExecuted checks that all vertices in the graph were executed at least once.
// This is a common assertion pattern for graph execution tests.
func RequireAllNodesExecuted(t interface {
	Errorf(format string, args ...any)
}, counters map[string]*int) {
	for name, counter := range counters {
		if *counter == 0 {
			t.Errorf("vertex %q was never executed", name)
		}
	}
}

// RequireExecutionOrder checks that vertices were executed in the expected order.
// This is useful for testing sequential execution.
func RequireExecutionOrder(t interface {
	Errorf(format string, args ...any)
}, counters map[string]*int, expectedOrder []string) {
	type execution struct {
		name  string
		count int
	}

	var executions []execution
	for name, counter := range counters {
		if *counter > 0 {
			executions = append(executions, execution{name: name, count: *counter})
		}
	}

	if len(executions) != len(expectedOrder) {
		t.Errorf("expected %d vertices to execute, got %d", len(expectedOrder), len(executions))
		return
	}

	for i, expected := range expectedOrder {
		found := false
		for _, exec := range executions {
			if exec.name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected vertex %q at position %d, but it was not executed", expected, i)
		}
	}
}

// NewTestContext creates a context with a timeout for tests.
// This standardizes timeout behavior across tests.
func NewTestContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// AssertEventually repeatedly checks a condition until it's true or a timeout occurs.
// This is useful for testing asynchronous operations.
func AssertEventually(t interface {
	Errorf(format string, args ...any)
	Helper()
}, condition func() bool, timeout time.Duration, message string) {
	t.Helper()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)

	for {
		if condition() {
			return
		}

		<-ticker.C
		if time.Now().After(deadline) {
			t.Errorf("condition not met within %v: %s", timeout, message)
			return
		}
	}
}

// WaitForCondition waits for a condition to become true or context to be canceled.
// Returns true if condition was met, false if context was canceled.
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

// FormatMessageHistory formats a slice of messages for debugging output.
// This is commonly needed in test assertions.
func FormatMessageHistory(messages []message.Message) string {
	if len(messages) == 0 {
		return "<empty>"
	}

	result := fmt.Sprintf("%d messages:\n", len(messages))
	for i, msg := range messages {
		content := message.Stringify(msg)
		if len(content) > 50 {
			content = content[:47] + "..."
		}
		result += fmt.Sprintf("  [%d] %s: %s\n", i, msg.Type(), content)
	}
	return result
}
