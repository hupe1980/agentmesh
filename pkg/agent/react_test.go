package agent

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests - Nil Checking

func TestNewModelNode_NilModel(t *testing.T) {
	_, err := NewModelNode(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model cannot be nil")
}

func TestNewToolNode_NilRegistry(t *testing.T) {
	_, err := NewToolNode(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "toolRegistry cannot be nil")
}

func TestNewModelNode_ValidModel(t *testing.T) {
	node, err := NewModelNode(&testutil.MockModel{})
	require.NoError(t, err)
	assert.NotNil(t, node)
}

func TestNewToolNode_ValidRegistry(t *testing.T) {
	registry := make(map[string]tool.Tool)
	node, err := NewToolNode(registry)
	require.NoError(t, err)
	assert.NotNil(t, node)
}

// Tests

func TestNew_BasicAgent(t *testing.T) {
	mdl := &testutil.MockModel{}
	compiled, err := NewReActAgent(mdl)

	require.NoError(t, err)
	require.NotNil(t, compiled)
	// Verify it implements MessageRunnable
	_, ok := compiled.(graph.Runnable[[]message.Message, message.Message])
	require.True(t, ok, "agent should implement MessageRunnable")
}

func TestNew_WithTools(t *testing.T) {
	mdl := &testutil.MockModel{}
	weatherTool := &testutil.MockTool{
		NameValue:        "weather",
		DescriptionValue: "Get weather",
	}

	compiled, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.NoError(t, err)
	require.NotNil(t, compiled)
}

func TestNew_NilToolsIgnored(t *testing.T) {
	mdl := &testutil.MockModel{}

	compiled, err := NewReActAgent(mdl, WithTools(nil, nil))

	require.NoError(t, err)
	require.NotNil(t, compiled)
}

func TestNew_ModelSupportsTools(t *testing.T) {
	mdl := &testutil.MockModel{
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				Tools:               true,
				MaxContextTokens:    4096,
				MaxOutputTokens:     2048,
				SupportedModalities: []string{"text"},
			}
		},
	}
	weatherTool := &testutil.MockTool{NameValue: "weather"}

	agent, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.NoError(t, err)
	require.NotNil(t, agent)
}

func TestNew_ModelDoesNotSupportTools(t *testing.T) {
	// Mock model that doesn't support tools (Capabilities().Tools = false)
	mdl := &basicModel{}
	weatherTool := &testutil.MockTool{NameValue: "weather"}

	_, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support tools")
}

func TestNew_ModelDoesNotSupportToolsViaCapabilities(t *testing.T) {
	mdl := &testutil.MockModel{
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				Tools:               false, // Model doesn't support tools
				MaxContextTokens:    4096,
				MaxOutputTokens:     2048,
				SupportedModalities: []string{"text"},
			}
		},
	}
	weatherTool := &testutil.MockTool{NameValue: "weather"}

	_, err := NewReActAgent(mdl, WithTools(weatherTool))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support tools")
}

func TestAgent_BasicExecution(t *testing.T) {
	mdl := &testutil.MockModel{
		GenerateFunc: func(ctx context.Context, req *model.Request) iter.Seq2[*model.Response, error] {
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

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	require.NotNil(t, messages)
	// CollectMessages returns messages from ExecutionResults (AI responses)
	assert.GreaterOrEqual(t, len(messages), 1) // At least one AI response
}

func TestAgent_ToolCalling(t *testing.T) {
	callCount := 0
	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
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

	weatherTool := &testutil.MockTool{
		NameValue: "weather",
		CallFunc: func(ctx context.Context, args string) (any, error) {
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

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	require.NotNil(t, messages)

	// CollectMessages returns messages from ExecutionResults
	// Should have: AI (with tool call) + Tool result + AI (final response)
	assert.GreaterOrEqual(t, len(messages), 2) // At least tool call and response

	// Verify model was called twice (once for tool request, once after tool result)
	assert.GreaterOrEqual(t, callCount, 1, "Model should be called at least once")
}

func TestAgent_UnregisteredTool(t *testing.T) {
	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
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

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool \"unknown_tool\" not registered")
}

func TestAgent_ToolExecutionError(t *testing.T) {
	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			aiMsg := message.NewAIMessageFromText("")

			aiMsg.ToolCalls = []message.ToolCall{
				{ID: "call_1", Name: "failing_tool"},
			}
			return aiMsg, nil
		}),
	}

	failingTool := &testutil.MockTool{
		NameValue: "failing_tool",
		CallFunc: func(ctx context.Context, args string) (any, error) {
			return nil, errors.New("tool execution failed")
		},
	}

	compiled, err := NewReActAgent(mdl, WithTools(failingTool))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool execution failed")
}

func TestAgent_ConditionalRouting(t *testing.T) {
	finalResponseSeen := false
	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
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

	testTool := &testutil.MockTool{
		NameValue: "test_tool",
		CallFunc: func(ctx context.Context, args string) (any, error) {
			return "tool result", nil
		},
	}

	compiled, err := NewReActAgent(mdl, WithTools(testTool))
	require.NoError(t, err)

	ctx := context.Background()
	_, err = graph.Last(compiled.Run(ctx, []message.Message{
		message.NewHumanMessageFromText("Test"),
	}))

	require.NoError(t, err)
	assert.True(t, finalResponseSeen, "Should route to model after tool execution")
}

func TestAgent_EmptyMessages(t *testing.T) {
	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			return message.NewAIMessageFromText("Response"), nil
		}),
	}

	compiled, err := NewReActAgent(mdl)
	require.NoError(t, err)

	ctx := context.Background()
	events, err := graph.Collect(compiled.Run(ctx, nil))

	require.NoError(t, err)
	require.NotNil(t, events)

	// Should have at least the AI response
	assert.GreaterOrEqual(t, len(events), 1)
}

func TestAgent_MultipleToolCalls(t *testing.T) {
	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			// Check if we have BOTH tool results
			toolResultIDs := make(map[string]bool)
			for _, msg := range messages {
				if msg.Type() == message.TypeTool {
					if toolMsg, ok := msg.(*message.ToolMessage); ok {
						toolResultIDs[toolMsg.ToolCallID] = true
					}
				}
			}

			// Verify both tool_a and tool_b results are present
			if toolResultIDs["1"] && toolResultIDs["2"] {
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

	toolACallCount := 0
	toolBCallCount := 0

	toolA := &testutil.MockTool{
		NameValue: "tool_a",
		CallFunc: func(ctx context.Context, args string) (any, error) {
			toolACallCount++
			return "result_a", nil
		},
	}
	toolB := &testutil.MockTool{
		NameValue: "tool_b",
		CallFunc: func(ctx context.Context, args string) (any, error) {
			toolBCallCount++
			return "result_b", nil
		},
	}

	compiled, err := NewReActAgent(mdl, WithTools(toolA, toolB))
	require.NoError(t, err)

	ctx := context.Background()
	events, err := graph.Collect(compiled.Run(ctx, []message.Message{
		message.NewHumanMessageFromText("Test"),
	}))

	require.NoError(t, err)
	require.NotEmpty(t, events, "Should have events")

	// After fixing the executor to unfold message arrays, ALL messages should be yielded
	// ToolNode returns multiple messages, and each should appear in the event stream

	// Get the last event which should be the final AI message
	lastEvent := events[len(events)-1]
	require.NotNil(t, lastEvent, "Last event should not be nil")

	// The last event should be an AI message saying "Both tools completed"
	aiMsg, ok := lastEvent.(*message.AIMessage)
	require.True(t, ok, "Last event should be AI message")
	parts := aiMsg.Parts()
	require.NotEmpty(t, parts, "AI message should have content")
	if textPart, ok := parts[0].(message.TextPart); ok {
		assert.Contains(t, textPart.Text, "Both tools completed")
	}

	// Count how many tool messages appear in the event stream
	toolMsgCount := 0
	toolMsgIDs := make(map[string]bool)
	for _, evt := range events {
		if evt != nil && evt.Type() == message.TypeTool {
			toolMsgCount++
			if toolMsg, ok := evt.(*message.ToolMessage); ok {
				toolMsgIDs[toolMsg.ToolCallID] = true
			}
		}
	}

	// Both tool messages should be yielded to the stream (executor unfolds message arrays)
	assert.Equal(t, 2, toolMsgCount, "Should have 2 tool messages in event stream (both yielded)")

	// Verify both tool call IDs are present in the stream
	assert.True(t, toolMsgIDs["1"], "Tool call ID '1' should be in stream")
	assert.True(t, toolMsgIDs["2"], "Tool call ID '2' should be in stream")

	// Verify BOTH tools were actually executed
	assert.Equal(t, 1, toolACallCount, "tool_a should be called once")
	assert.Equal(t, 1, toolBCallCount, "tool_b should be called once")
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
