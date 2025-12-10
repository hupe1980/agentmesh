package tool

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMockWorkerGraph creates a simple graph that returns a fixed response
func createMockWorkerGraph(t *testing.T, response string) *message.Graph {
	g := message.NewGraphBuilder()

	g.Node("worker", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		msg := message.Message(message.NewAIMessageFromText(response))
		return graph.Append(message.MessagesKey, msg).End()
	}, graph.END)

	g.Start("worker")

	// Build the graph before returning
	built, err := g.Build()
	if err != nil {
		t.Fatalf("failed to build mock graph: %v", err)
	}

	return built
}

func TestHandoffToAgent(t *testing.T) {
	ctx := context.Background()

	// Create a simple worker agent graph
	workerGraph := createMockWorkerGraph(t, "Worker response")

	// Create handoff tool
	handoffTool, err := HandoffToAgent(
		"test_agent",
		"A test agent for validation",
		workerGraph,
		WithContext(true),
		WithRetries(1),
	)
	require.NoError(t, err)
	require.NotNil(t, handoffTool)

	// Verify tool name and description
	assert.Equal(t, "handoff_to_test_agent", handoffTool.Name())
	assert.Contains(t, handoffTool.Description(), "test_agent")

	// Test tool invocation
	argsJSON := `{"task": "Test task", "context": "Test context"}`
	result, err := handoffTool.Call(ctx, argsJSON)
	require.NoError(t, err)
	assert.Equal(t, "Worker response", result)
}

func TestHandoffToAgent_MissingTask(t *testing.T) {
	ctx := context.Background()
	workerGraph := createMockWorkerGraph(t, "Response")

	handoffTool, err := HandoffToAgent("test_agent", "Test", workerGraph)
	require.NoError(t, err)

	// Call without task should fail (JSON schema validation)
	argsJSON := `{"context": "only context"}`
	_, err = handoffTool.Call(ctx, argsJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid arguments")
}

func TestHandoffToAgent_NilGraph(t *testing.T) {
	_, err := HandoffToAgent("test_agent", "Test", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestHandoffToAgent_WithoutContext(t *testing.T) {
	ctx := context.Background()
	workerGraph := createMockWorkerGraph(t, "Response")

	handoffTool, err := HandoffToAgent(
		"test_agent",
		"Test",
		workerGraph,
		WithContext(false), // Disable context passing
	)
	require.NoError(t, err)

	argsJSON := `{"task": "Test task", "context": "Should be ignored"}`
	result, err := handoffTool.Call(ctx, argsJSON)
	require.NoError(t, err)
	assert.Equal(t, "Response", result)
}

func TestHandoffToAgent_Retry(t *testing.T) {
	ctx := context.Background()

	// Create a graph that fails on first call, succeeds on second
	failOnce := true
	g := message.NewGraphBuilder()

	g.Node("worker", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		if failOnce {
			failOnce = false
			return graph.Fail(errors.New("temporary error"))
		}
		msg := message.Message(message.NewAIMessageFromText("Success after retry"))
		return graph.Append(message.MessagesKey, msg).End()
	}, graph.END)

	g.Start("worker")

	// Build the graph before passing to the tool
	built, err := g.Build()
	require.NoError(t, err)

	handoffTool, err := HandoffToAgent(
		"test_agent",
		"Test",
		built,
		WithRetries(2), // Allow retry
	)
	require.NoError(t, err)

	argsJSON := `{"task": "Test task"}`
	result, err := handoffTool.Call(ctx, argsJSON)
	require.NoError(t, err)
	assert.Equal(t, "Success after retry", result)
}

func TestMessageString(t *testing.T) {
	tests := []struct {
		name     string
		msg      message.Message
		expected string
	}{
		{
			name:     "AI message with text",
			msg:      message.NewAIMessageFromText("Hello world"),
			expected: "Hello world",
		},
		{
			name:     "Human message",
			msg:      message.NewHumanMessageFromText("Question"),
			expected: "Question",
		},
		{
			name:     "System message",
			msg:      message.NewSystemMessageFromText("System prompt"),
			expected: "System prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.msg.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidResult(t *testing.T) {
	tests := []struct {
		name     string
		result   string
		expected bool
	}{
		{"Valid result", "Success", true},
		{"Empty string", "", false},
		{"Error keyword", "error", false},
		{"Failed keyword", "failed", false},
		{"Valid with error in text", "This is an error message", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidResult(tt.result))
		})
	}
}
