package agent

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing
type mockModel struct {
	generateFunc     func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error]
	capabilitiesFunc func() model.Capabilities
}

func (m *mockModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	// Default implementation returns a single message
	return func(yield func(*model.Response, error) bool) {
		yield(&model.Response{
			Message: message.NewAIMessageFromText("mock response"),
			Partial: false, // Single complete response
		}, nil)
	}
}

func (m *mockModel) Capabilities() model.Capabilities {
	if m.capabilitiesFunc != nil {
		return m.capabilitiesFunc()
	}
	// Default capabilities
	return model.Capabilities{
		Streaming:           true,
		Tools:               true,
		MaxContextTokens:    4096,
		MaxOutputTokens:     2048,
		SupportedModalities: []string{"text"},
	}
}

// Helper to wrap simple generate functions into iterators
func wrapGenerate(fn func(ctx context.Context, messages []message.Message) (message.Message, error)) func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
		return func(yield func(*model.Response, error) bool) {
			msg, err := fn(ctx, req.Messages)
			yield(&model.Response{Message: msg, Partial: false}, err)
		}
	}
}

type mockTool struct {
	name        string
	description string
	callFunc    func(ctx context.Context, args string) (any, error)
}

func (t *mockTool) Name() string {
	return t.name
}

func (t *mockTool) Description() string {
	return t.description
}

func (t *mockTool) Definition() *tool.Definition {
	return &tool.Definition{
		Type: "function",
		Function: tool.FunctionDefinition{
			Name:        t.name,
			Description: t.description,
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

func (t *mockTool) Call(ctx context.Context, args string) (any, error) {
	if t.callFunc != nil {
		return t.callFunc(ctx, args)
	}
	return "mock result", nil
}

// Tests

func TestNew_BasicAgent(t *testing.T) {
	mdl := &mockModel{}
	compiled, err := NewReActAgent(mdl)

	require.NoError(t, err)
	require.NotNil(t, compiled)
	assert.NotNil(t, compiled.State())
}

func TestNew_WithTools(t *testing.T) {
	mdl := &mockModel{}
	weatherTool := &mockTool{
		name:        "weather",
		description: "Get weather",
	}

	compiled, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.NoError(t, err)
	require.NotNil(t, compiled)
}

func TestNew_NilToolsIgnored(t *testing.T) {
	mdl := &mockModel{}

	compiled, err := NewReActAgent(mdl, WithTools(nil, nil))

	require.NoError(t, err)
	require.NotNil(t, compiled)
}

func TestNew_ModelSupportsTools(t *testing.T) {
	mdl := &mockModel{
		capabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				Tools:               true,
				MaxContextTokens:    4096,
				MaxOutputTokens:     2048,
				SupportedModalities: []string{"text"},
			}
		},
	}
	weatherTool := &mockTool{name: "weather"}

	agent, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.NoError(t, err)
	require.NotNil(t, agent)
}

func TestNew_ModelDoesNotSupportTools(t *testing.T) {
	// Mock model that doesn't support tools (Capabilities().Tools = false)
	mdl := &basicModel{}
	weatherTool := &mockTool{name: "weather"}

	_, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support tools")
}

func TestNew_ModelDoesNotSupportToolsViaCapabilities(t *testing.T) {
	mdl := &mockModel{
		capabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				Tools:               false, // Model doesn't support tools
				MaxContextTokens:    4096,
				MaxOutputTokens:     2048,
				SupportedModalities: []string{"text"},
			}
		},
	}
	weatherTool := &mockTool{name: "weather"}

	_, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support tools")
}

func TestAgent_BasicExecution(t *testing.T) {
	mdl := &mockModel{
		generateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
			return func(yield func(*model.Response, error) bool) {
				yield(&model.Response{
					Message: message.NewAIMessageFromText("Hello! I'm here to help."),
				}, nil)
			}
		},
	}

	compiled, err := NewReActAgent(mdl)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewSystemMessageFromText("You are a helpful assistant."),
		message.NewHumanMessageFromText("Hello!"),
	}

	result, err := compiled.Invoke(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.GreaterOrEqual(t, len(result), 3) // System + Human + AI
}

func TestAgent_ToolCalling(t *testing.T) {
	callCount := 0
	mdl := &mockModel{
		generateFunc: wrapGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			// First call: model requests tool
			if callCount == 0 {
				callCount++
				aiMsg := message.NewAIMessageFromText("I'll check the weather")

				aiMsg.ToolCalls = []message.ToolCall{
					{
						ID:        "call_1",
						Name:      "weather",
						Arguments: map[string]any{"location": "Berlin"},
					},
				}
				return aiMsg, nil
			}
			// Second call: model responds after tool result
			return message.NewAIMessageFromText("The weather is sunny!"), nil
		}),
	}

	weatherTool := &mockTool{
		name: "weather",
		callFunc: func(ctx context.Context, args string) (any, error) {
			return map[string]any{
				"temperature": 21,
				"conditions":  "sunny",
			}, nil
		},
	}

	compiled, err := NewReActAgent(mdl, WithTools(weatherTool))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("What's the weather in Berlin?"),
	}

	result, err := compiled.Invoke(ctx, input)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have: Human + AI (with tool call) + Tool result + AI (final response)
	assert.GreaterOrEqual(t, len(result), 4)

	// Verify model was called twice (once for tool request, once after tool result)
	assert.GreaterOrEqual(t, callCount, 1, "Model should be called at least once")
}

func TestAgent_UnregisteredTool(t *testing.T) {
	mdl := &mockModel{
		generateFunc: wrapGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			aiMsg := message.NewAIMessageFromText("Calling unknown tool")

			aiMsg.ToolCalls = []message.ToolCall{
				{
					ID:   "call_1",
					Name: "unknown_tool",
				},
			}

			return aiMsg, nil
		}),
	}

	compiled, err := NewReActAgent(mdl)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Test"),
	}

	_, err = compiled.Invoke(ctx, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool \"unknown_tool\" not registered")
}

func TestAgent_ToolExecutionError(t *testing.T) {
	mdl := &mockModel{
		generateFunc: wrapGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			aiMsg := message.NewAIMessageFromText("")

			aiMsg.ToolCalls = []message.ToolCall{
				{ID: "call_1", Name: "failing_tool"},
			}
			return aiMsg, nil
		}),
	}

	failingTool := &mockTool{
		name: "failing_tool",
		callFunc: func(ctx context.Context, args string) (any, error) {
			return nil, errors.New("tool execution failed")
		},
	}

	compiled, err := NewReActAgent(mdl, WithTools(failingTool))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	_, err = compiled.Invoke(ctx, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool execution failed")
}

func TestAgent_ConditionalRouting(t *testing.T) {
	finalResponseSeen := false
	mdl := &mockModel{
		generateFunc: wrapGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			// Check if we've seen a tool result
			hasToolResult := false
			for _, msg := range messages {
				if msg.Type() == message.TypeTool {
					hasToolResult = true
					break
				}
			}

			if hasToolResult {
				finalResponseSeen = true
				return message.NewAIMessageFromText("Done"), nil
			}

			// First call: request tool
			aiMsg := message.NewAIMessageFromText("")

			aiMsg.ToolCalls = []message.ToolCall{
				{ID: "1", Name: "test_tool"},
			}
			return aiMsg, nil
		}),
	}

	testTool := &mockTool{
		name: "test_tool",
		callFunc: func(ctx context.Context, args string) (any, error) {
			return "tool result", nil
		},
	}

	compiled, err := NewReActAgent(mdl, WithTools(testTool))
	require.NoError(t, err)

	ctx := context.Background()
	_, err = compiled.Invoke(ctx, []message.Message{
		message.NewHumanMessageFromText("Test"),
	})

	require.NoError(t, err)
	assert.True(t, finalResponseSeen, "Should route to model after tool execution")
}

func TestAgent_EmptyMessages(t *testing.T) {
	mdl := &mockModel{
		generateFunc: wrapGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			return message.NewAIMessageFromText("Response"), nil
		}),
	}

	compiled, err := NewReActAgent(mdl)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := compiled.Invoke(ctx, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have at least the AI response
	assert.GreaterOrEqual(t, len(result), 1)
}

func TestAgent_MultipleToolCalls(t *testing.T) {
	mdl := &mockModel{
		generateFunc: wrapGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			// Check if we have tool results
			hasToolResults := false
			for _, msg := range messages {
				if msg.Type() == message.TypeTool {
					hasToolResults = true
					break
				}
			}

			if hasToolResults {
				return message.NewAIMessageFromText("Both tools completed"), nil
			}

			// Request multiple tools
			aiMsg := message.NewAIMessageFromText("")

			aiMsg.ToolCalls = []message.ToolCall{
				{ID: "1", Name: "tool_a"},
				{ID: "2", Name: "tool_b"},
			}
			return aiMsg, nil
		}),
	}

	toolA := &mockTool{name: "tool_a"}
	toolB := &mockTool{name: "tool_b"}

	compiled, err := NewReActAgent(mdl, WithTools(toolA, toolB))
	require.NoError(t, err)

	ctx := context.Background()
	result, err := compiled.Invoke(ctx, []message.Message{
		message.NewHumanMessageFromText("Test"),
	})

	require.NoError(t, err)

	// Count tool messages
	toolMsgCount := 0
	for _, msg := range result {
		if msg.Type() == message.TypeTool {
			toolMsgCount++
		}
	}
	assert.Equal(t, 2, toolMsgCount, "Should have 2 tool result messages")
}

// Basic model without Tools support for testing
type basicModel struct{}

func (m *basicModel) Generate(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
	return func(yield func(*model.Response, error) bool) {
		yield(&model.Response{
			Message: message.NewAIMessageFromText("response"),
			Partial: false,
		}, nil)
	}
}

func (m *basicModel) Capabilities() model.Capabilities {
	return model.Capabilities{
		Streaming:           true,
		Tools:               false, // Basic model doesn't support tools
		MaxContextTokens:    4096,
		MaxOutputTokens:     2048,
		SupportedModalities: []string{"text"},
	}
}
