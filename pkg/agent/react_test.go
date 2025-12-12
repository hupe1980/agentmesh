package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/guardrail"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests - Nil Checking

func TestNewModelNodeFunc_NilExecutor(t *testing.T) {
	_, err := NewModelNodeFunc(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "executor must not be nil")
}

func TestNewToolNodeFunc_NoExecutorOrToolset(t *testing.T) {
	_, err := NewToolNodeFunc()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "either Executor or Toolset must be provided")
}

func TestNewModelNodeFunc_ValidExecutor(t *testing.T) {
	executor := model.NewExecutor(testutil.NewModelBuilder().Build())
	node, err := NewModelNodeFunc(executor)
	require.NoError(t, err)
	assert.NotNil(t, node)
}

func TestNewToolNodeFunc_ValidExecutor(t *testing.T) {
	registry := make(map[string]tool.Tool)
	executor := tool.NewSequentialExecutor(registry)
	node, err := NewToolNodeFunc(WithToolExecutor(executor))
	require.NoError(t, err)
	assert.NotNil(t, node)
}

// Tests

func TestNew_BasicAgent(t *testing.T) {
	mdl := testutil.NewModelBuilder().Build()
	compiled, err := NewReAct(mdl)

	require.NoError(t, err)
	require.NotNil(t, compiled)
	// Verify it returns *graph.MessageGraph
	_ = compiled
}

func TestNew_WithTools(t *testing.T) {
	mdl := testutil.NewModelBuilder().Build()
	weatherTool := testutil.NewToolBuilder("weather").
		WithDescription("Get weather").
		Build()

	compiled, err := NewReAct(mdl, WithTools(weatherTool))

	require.NoError(t, err)
	require.NotNil(t, compiled)
}

func TestNew_NilToolsIgnored(t *testing.T) {
	mdl := testutil.NewModelBuilder().Build()

	compiled, err := NewReAct(mdl, WithTools(nil, nil))

	require.NoError(t, err)
	require.NotNil(t, compiled)
}

func TestNew_ModelSupportsTools(t *testing.T) {
	mdl := testutil.NewModelBuilder().
		WithCapabilities(model.Capabilities{
			Tools:               true,
			MaxContextTokens:    4096,
			MaxOutputTokens:     2048,
			SupportedModalities: []string{"text"},
		}).
		Build()
	weatherTool := testutil.NewToolBuilder("weather").Build()

	agent, err := NewReAct(mdl, WithTools(weatherTool))

	require.NoError(t, err)
	require.NotNil(t, agent)
}

// Note: With dynamic toolset support, tool capability validation now happens at runtime
// when tools are discovered from toolsets. This allows for more flexible agent composition
// where toolsets may return different tools based on state.
// The old tests TestNew_ModelDoesNotSupportTools and TestNew_ModelDoesNotSupportToolsViaCapabilities
// were removed as they tested compile-time validation that no longer applies.

func TestAgent_BasicExecution(t *testing.T) {
	mdl := testutil.NewModelBuilder().
		WithResponse("Hello! I'm here to help.").
		Build()

	compiled, err := NewReAct(mdl)
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
	mdl := testutil.NewModelBuilder().
		WithToolCalls(message.ToolCall{
			ID:        "call_1",
			Name:      "weather",
			Arguments: `{"location":"Berlin"}`,
		}).
		WithResponse("The weather is sunny!").
		Build()

	weatherTool := testutil.NewToolBuilder("weather").
		WithResult(`{"temperature": 21, "conditions": "sunny"}`).
		Build()

	compiled, err := NewReAct(mdl, WithTools(weatherTool))
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
}

func TestAgent_UnregisteredTool(t *testing.T) {
	mdl := testutil.NewModelBuilder().
		WithToolCalls(message.ToolCall{
			ID:   "call_1",
			Name: "unknown_tool",
		}).
		Build()

	compiled, err := NewReAct(mdl)
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
	mdl := testutil.NewModelBuilder().
		WithToolCalls(message.ToolCall{ID: "call_1", Name: "failing_tool"}).
		Build()

	failingTool := testutil.NewToolBuilder("failing_tool").
		WithError(errors.New("tool execution failed")).
		Build()

	compiled, err := NewReAct(mdl, WithTools(failingTool))
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

	compiled, err := NewReAct(mdl, WithTools(testTool))
	require.NoError(t, err)

	ctx := context.Background()
	_, err = graph.Last(compiled.Run(ctx, []message.Message{
		message.NewHumanMessageFromText("Test"),
	}))

	require.NoError(t, err)
	assert.True(t, finalResponseSeen, "Should route to model after tool execution")
}

func TestAgent_EmptyMessages(t *testing.T) {
	mdl := testutil.NewModelBuilder().
		WithResponse("Response").
		Build()

	compiled, err := NewReAct(mdl)
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

	compiled, err := NewReAct(mdl, WithTools(toolA, toolB))
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

// Tests for prepareStructuredOutputFallback

func TestPrepareStructuredOutputFallback_NilSchema(t *testing.T) {
	mdl := &testutil.MockModel{}
	tools := []tool.Tool{}

	resultSchema, resultTools, err := prepareStructuredOutputFallback(mdl, nil, tools)

	require.NoError(t, err)
	assert.Nil(t, resultSchema, "schema should remain nil")
	assert.Empty(t, resultTools, "tools should remain empty")
}

func TestPrepareStructuredOutputFallback_ModelSupportsStructuredOutput(t *testing.T) {
	mdl := &testutil.MockModel{
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				StructuredOutput: true,
				Tools:            true,
			}
		},
	}

	type TestOutput struct {
		Result string `json:"result" jsonschema:"required"`
	}
	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{})
	require.NoError(t, err)

	tools := []tool.Tool{}

	resultSchema, resultTools, err := prepareStructuredOutputFallback(mdl, &outputSchema, tools)

	require.NoError(t, err)
	assert.Equal(t, &outputSchema, resultSchema, "schema should be returned unchanged")
	assert.Empty(t, resultTools, "no SetModelResponseTool should be added")
}

func TestPrepareStructuredOutputFallback_FallbackToToolCalling(t *testing.T) {
	mdl := &testutil.MockModel{
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				StructuredOutput: false,
				Tools:            true, // Model supports tools but not structured output
			}
		},
	}

	type TestOutput struct {
		Result string `json:"result" jsonschema:"required"`
	}
	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{})
	require.NoError(t, err)

	existingTool := &testutil.MockTool{
		NameValue:        "existing_tool",
		DescriptionValue: "An existing tool",
	}
	tools := []tool.Tool{existingTool}

	resultSchema, resultTools, err := prepareStructuredOutputFallback(mdl, &outputSchema, tools)

	require.NoError(t, err)
	assert.Nil(t, resultSchema, "schema should be nil when using tool fallback")
	assert.Len(t, resultTools, 2, "should have original tool + SetModelResponseTool")

	// Verify SetModelResponseTool was added
	var hasSetModelResponse bool
	for _, t := range resultTools {
		if t.Name() == "set_model_response" {
			hasSetModelResponse = true
			break
		}
	}
	assert.True(t, hasSetModelResponse, "SetModelResponseTool should be added")
}

func TestPrepareStructuredOutputFallback_ModelDoesNotSupportTools(t *testing.T) {
	mdl := &testutil.MockModel{
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				StructuredOutput: false,
				Tools:            false, // Model doesn't support tools, can't use fallback
			}
		},
	}

	type TestOutput struct {
		Result string `json:"result" jsonschema:"required"`
	}
	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{})
	require.NoError(t, err)

	tools := []tool.Tool{}

	resultSchema, resultTools, err := prepareStructuredOutputFallback(mdl, &outputSchema, tools)

	require.NoError(t, err)
	assert.Equal(t, &outputSchema, resultSchema, "schema should be returned unchanged when no fallback possible")
	assert.Empty(t, resultTools, "no tools should be added")
}

func TestPrepareStructuredOutputFallback_SetModelResponseToolAlreadyExists(t *testing.T) {
	mdl := &testutil.MockModel{
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{
				StructuredOutput: false,
				Tools:            true,
			}
		},
	}

	type TestOutput struct {
		Result string `json:"result" jsonschema:"required"`
	}
	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{})
	require.NoError(t, err)

	// Create SetModelResponseTool manually
	existingSetModelResponseTool, err := tool.NewSetModelResponseTool(&outputSchema)
	require.NoError(t, err)

	tools := []tool.Tool{existingSetModelResponseTool}

	resultSchema, resultTools, err := prepareStructuredOutputFallback(mdl, &outputSchema, tools)

	require.NoError(t, err)
	assert.Nil(t, resultSchema, "schema should be nil when using tool fallback")
	assert.Len(t, resultTools, 1, "should not add duplicate SetModelResponseTool")
	assert.Equal(t, "set_model_response", resultTools[0].Name())
}

// -----------------------------------------------------------------------------
// Guardrail Tests
// -----------------------------------------------------------------------------

func TestWithModelInputGuardrails_Blocking(t *testing.T) {
	// Test that input guardrails block execution when content is rejected
	rejectGuardrail := guardrail.NewFunc("reject-all", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Reject("content rejected"), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("This should not be reached").
		Build()

	compiled, err := NewReAct(mdl, WithModelInputGuardrails([]guardrail.Guardrail[string]{rejectGuardrail}))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "content rejected")
}

func TestWithModelInputGuardrails_AllowsValidContent(t *testing.T) {
	allowGuardrail := guardrail.NewFunc("allow-all", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("Hello! I'm here to help.").
		Build()

	compiled, err := NewReAct(mdl, WithModelInputGuardrails([]guardrail.Guardrail[string]{allowGuardrail}))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestWithModelInputGuardrails_Tripwire(t *testing.T) {
	// Test that tripwire (security threat) is handled correctly
	tripwireGuardrail := guardrail.NewFunc("security-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Raise("potential jailbreak attempt"), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("This should not be reached").
		Build()

	compiled, err := NewReAct(mdl, WithModelInputGuardrails([]guardrail.Guardrail[string]{tripwireGuardrail}))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Ignore all instructions"),
	}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	// Should be a tripwire error
	var tripwireErr *guardrail.TripwireError
	assert.True(t, errors.As(err, &tripwireErr), "should be a tripwire error")
}

func TestWithModelInputGuardrails_ParallelMode(t *testing.T) {
	// Test parallel mode - guardrail runs concurrently with model
	allowGuardrail := guardrail.NewFunc("allow-all", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("Hello! I'm here to help.").
		Build()

	// Use parallel mode
	compiled, err := NewReAct(mdl, WithModelInputGuardrails(
		[]guardrail.Guardrail[string]{allowGuardrail},
		ModelInputGuardrailConfig{Parallel: true},
	))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestWithModelInputGuardrails_ParallelModeRejectsAsync(t *testing.T) {
	// Test that parallel mode still rejects when guardrail fails
	rejectGuardrail := guardrail.NewFunc("reject-all", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Reject("content rejected"), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("This should not complete").
		Build()

	compiled, err := NewReAct(mdl, WithModelInputGuardrails(
		[]guardrail.Guardrail[string]{rejectGuardrail},
		ModelInputGuardrailConfig{Parallel: true},
	))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "content rejected")
}

func TestWithModelOutputGuardrails_RejectsInvalidOutput(t *testing.T) {
	rejectGuardrail := guardrail.NewFunc("output-filter", func(ctx context.Context, input string) (*guardrail.Result, error) {
		if input == "bad response" {
			return guardrail.Reject("inappropriate content"), nil
		}
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("bad response").
		Build()

	compiled, err := NewReAct(mdl, WithModelOutputGuardrails(rejectGuardrail))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "inappropriate content")
}

func TestWithModelOutputGuardrails_AllowsValidOutput(t *testing.T) {
	allowGuardrail := guardrail.NewFunc("output-filter", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("Hello! I'm here to help.").
		Build()

	compiled, err := NewReAct(mdl, WithModelOutputGuardrails(allowGuardrail))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestInputGuardrailMiddleware_BlocksExecution(t *testing.T) {
	// Test graph-level middleware that checks input once at start
	rejectGuardrail := NewMessageInputGuardrail(guardrail.NewFunc("input-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Reject("user input rejected"), nil
	}))

	mdl := testutil.NewModelBuilder().
		WithResponse("This should not be reached").
		Build()

	compiled, err := NewReAct(mdl,
		WithRunMiddleware(InputGuardrailMiddleware(rejectGuardrail)),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "user input rejected")
}

func TestInputGuardrailMiddleware_AllowsValidInput(t *testing.T) {
	allowGuardrail := NewMessageInputGuardrail(guardrail.NewFunc("input-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	}))

	mdl := testutil.NewModelBuilder().
		WithResponse("Hello! I'm here to help.").
		Build()

	compiled, err := NewReAct(mdl,
		WithRunMiddleware(InputGuardrailMiddleware(allowGuardrail)),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestOutputGuardrailMiddleware_RejectsFinalOutput(t *testing.T) {
	// Test graph-level middleware that checks final output once
	rejectGuardrail := NewMessageOutputGuardrail(guardrail.NewFunc("output-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Reject("final output rejected"), nil
	}))

	mdl := testutil.NewModelBuilder().
		WithResponse("Some response").
		Build()

	compiled, err := NewReAct(mdl,
		WithRunMiddleware(OutputGuardrailMiddleware(rejectGuardrail)),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "final output rejected")
}

func TestOutputGuardrailMiddleware_AllowsValidOutput(t *testing.T) {
	allowGuardrail := NewMessageOutputGuardrail(guardrail.NewFunc("output-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	}))

	mdl := testutil.NewModelBuilder().
		WithResponse("Hello! I'm here to help.").
		Build()

	compiled, err := NewReAct(mdl,
		WithRunMiddleware(OutputGuardrailMiddleware(allowGuardrail)),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestCombinedGuardrails_InputAndOutput(t *testing.T) {
	// Test combining both input and output guardrails
	inputGuardrail := NewMessageInputGuardrail(guardrail.NewFunc("input-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	}))

	outputGuardrail := NewMessageOutputGuardrail(guardrail.NewFunc("output-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		return guardrail.Allow(), nil
	}))

	mdl := testutil.NewModelBuilder().
		WithResponse("Hello! I'm here to help.").
		Build()

	compiled, err := NewReAct(mdl,
		WithRunMiddleware(
			InputGuardrailMiddleware(inputGuardrail),
			OutputGuardrailMiddleware(outputGuardrail),
		),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello!"),
	}

	messages, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(messages), 1)
}

func TestGuardrailMiddleware_RunsOnceNotPerNode(t *testing.T) {
	// Verify graph middleware runs once, not per LLM call
	checkCount := 0
	inputGuardrail := NewMessageInputGuardrail(guardrail.NewFunc("count-check", func(ctx context.Context, input string) (*guardrail.Result, error) {
		checkCount++
		return guardrail.Allow(), nil
	}))

	// Model that makes tool call, then responds
	mdl := testutil.NewModelBuilder().
		WithToolCalls(message.ToolCall{
			ID:        "call_1",
			Name:      "test_tool",
			Arguments: `{}`,
		}).
		WithResponse("Done!").
		Build()

	testTool := testutil.NewToolBuilder("test_tool").
		WithResult("tool result").
		Build()

	compiled, err := NewReAct(mdl,
		WithTools(testTool),
		WithRunMiddleware(InputGuardrailMiddleware(inputGuardrail)),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Use the tool"),
	}

	_, err = graph.Collect(compiled.Run(ctx, input))
	require.NoError(t, err)

	// Graph middleware should run exactly once (at start of Run)
	assert.Equal(t, 1, checkCount, "input guardrail should run exactly once")
}

// -----------------------------------------------------------------------------
// Graph-level Guardrail Option Tests
// -----------------------------------------------------------------------------

func TestWithGraphInputGuardrails_Blocking(t *testing.T) {
	rejectGuardrail := guardrail.NewFunc("reject-bad", func(ctx context.Context, input string) (*guardrail.Result, error) {
		if strings.Contains(input, "forbidden") {
			return guardrail.Reject("content blocked"), nil
		}
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("I'll help!").
		Build()

	compiled, err := NewReAct(mdl, WithGraphInputGuardrails(rejectGuardrail))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("This is forbidden content"),
	}

	_, err = graph.Collect(compiled.Run(ctx, input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "content blocked")
}

func TestWithGraphInputGuardrails_RunsOnce(t *testing.T) {
	checkCount := 0
	countingGuardrail := guardrail.NewFunc("counting", func(ctx context.Context, input string) (*guardrail.Result, error) {
		checkCount++
		return guardrail.Allow(), nil
	})

	// Model that makes a tool call, then responds
	mdl := testutil.NewModelBuilder().
		WithToolCalls(message.ToolCall{
			ID:        "call_1",
			Name:      "test_tool",
			Arguments: `{}`,
		}).
		WithResponse("Done!").
		Build()

	testTool := testutil.NewToolBuilder("test_tool").
		WithResult("tool result").
		Build()

	compiled, err := NewReAct(mdl,
		WithTools(testTool),
		WithGraphInputGuardrails(countingGuardrail),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Use the tool"),
	}

	_, err = graph.Collect(compiled.Run(ctx, input))
	require.NoError(t, err)

	// Graph guardrail should run exactly once
	assert.Equal(t, 1, checkCount, "graph input guardrail should run exactly once")
}

func TestWithGraphOutputGuardrails_Blocking(t *testing.T) {
	rejectGuardrail := guardrail.NewFunc("reject-secret", func(ctx context.Context, input string) (*guardrail.Result, error) {
		if strings.Contains(input, "secret") {
			return guardrail.Reject("secrets not allowed"), nil
		}
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithResponse("The secret is 42").
		Build()

	compiled, err := NewReAct(mdl, WithGraphOutputGuardrails(rejectGuardrail))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Tell me a secret"),
	}

	_, err = graph.Collect(compiled.Run(ctx, input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secrets not allowed")
}

// -----------------------------------------------------------------------------
// Tool-level Guardrail Option Tests
// -----------------------------------------------------------------------------

func TestWithToolInputGuardrails_Tripwire(t *testing.T) {
	// Use tripwire to cause an actual error that stops execution
	tripwireGuardrail := guardrail.NewFunc("dangerous-detector", func(ctx context.Context, input string) (*guardrail.Result, error) {
		if strings.Contains(input, "dangerous") {
			return guardrail.Raise("dangerous input detected"), nil
		}
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithToolCalls(message.ToolCall{
			ID:        "call_1",
			Name:      "test_tool",
			Arguments: `{"input": "dangerous command"}`,
		}).
		WithResponse("Done!").
		Build()

	testTool := testutil.NewToolBuilder("test_tool").
		WithResult("executed").
		Build()

	compiled, err := NewReAct(mdl,
		WithTools(testTool),
		WithToolInputGuardrails(tripwireGuardrail),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Execute the command"),
	}

	_, err = graph.Collect(compiled.Run(ctx, input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "triggered")
}

func TestWithToolOutputGuardrails_Tripwire(t *testing.T) {
	// Use tripwire to cause an actual error that stops execution
	tripwireGuardrail := guardrail.NewFunc("sensitive-detector", func(ctx context.Context, input string) (*guardrail.Result, error) {
		if strings.Contains(input, "sensitive") {
			return guardrail.Raise("sensitive data detected"), nil
		}
		return guardrail.Allow(), nil
	})

	mdl := testutil.NewModelBuilder().
		WithToolCalls(message.ToolCall{
			ID:        "call_1",
			Name:      "test_tool",
			Arguments: `{}`,
		}).
		WithResponse("Done!").
		Build()

	testTool := testutil.NewToolBuilder("test_tool").
		WithResult("sensitive data here").
		Build()

	compiled, err := NewReAct(mdl,
		WithTools(testTool),
		WithToolOutputGuardrails(tripwireGuardrail),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Get the data"),
	}

	_, err = graph.Collect(compiled.Run(ctx, input))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "triggered")
}

func TestWithToolGuardrails_RunsOnEveryToolCall(t *testing.T) {
	checkCount := 0
	countingGuardrail := guardrail.NewFunc("counting", func(ctx context.Context, input string) (*guardrail.Result, error) {
		checkCount++
		return guardrail.Allow(), nil
	})

	// Model makes two tool calls
	mdl := testutil.NewModelBuilder().
		WithToolCalls(
			message.ToolCall{ID: "call_1", Name: "test_tool", Arguments: `{}`},
			message.ToolCall{ID: "call_2", Name: "test_tool", Arguments: `{}`},
		).
		WithResponse("Done!").
		Build()

	testTool := testutil.NewToolBuilder("test_tool").
		WithResult("result").
		Build()

	compiled, err := NewReAct(mdl,
		WithTools(testTool),
		WithToolInputGuardrails(countingGuardrail),
	)
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Execute tools"),
	}

	_, err = graph.Collect(compiled.Run(ctx, input))
	require.NoError(t, err)

	// Tool guardrail should run for each tool call (2 calls)
	assert.Equal(t, 2, checkCount, "tool input guardrail should run for each tool call")
}
