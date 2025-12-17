package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
	"github.com/hupe1980/agentmesh/pkg/schema"
	"github.com/hupe1980/agentmesh/pkg/testutil"
	"github.com/hupe1980/agentmesh/pkg/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests - Nil Checking

func TestNewModelNodeFunc_NilModel(t *testing.T) {
	_, err := NewModelNodeFunc(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model must not be nil")
}

func TestNewToolNodeFunc_NoExecutorOrToolset(t *testing.T) {
	_, err := NewToolNodeFunc()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "either Executor or Toolset must be provided")
}

func TestNewModelNodeFunc_ValidModel(t *testing.T) {
	mdl := testutil.NewModelBuilder().Build()
	node, err := NewModelNodeFunc(mdl)
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

// Schema Validation Tests

func TestReAct_WithSchemaValidation_ValidOutput(t *testing.T) {
	type TestOutput struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age" jsonschema:"required"`
	}

	// Model returns valid JSON that matches the schema
	mdl := testutil.NewModelBuilder().
		WithCapabilities(model.Capabilities{StructuredOutput: true}).
		WithResponse(`{"name": "John", "age": 30}`).
		Build()

	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{},
		schema.WithValidationPolicy(schema.ValidationStrict()),
	)
	require.NoError(t, err)

	compiled, err := NewReAct(mdl, WithOutputSchema(&outputSchema))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	events, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	require.NotEmpty(t, events)

	// Verify we got a valid response
	lastEvent := events[len(events)-1]
	aiMsg, ok := lastEvent.(*message.AIMessage)
	require.True(t, ok)
	parts := aiMsg.Parts()
	require.NotEmpty(t, parts)
	if textPart, ok := parts[0].(message.TextPart); ok {
		assert.Contains(t, textPart.Text, `"name"`)
		assert.Contains(t, textPart.Text, `"age"`)
	}
}

func TestReAct_WithSchemaValidation_InvalidOutput_StrictMode(t *testing.T) {
	type TestOutput struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age" jsonschema:"required"`
	}

	// Model returns invalid JSON (missing required field "age")
	mdl := testutil.NewModelBuilder().
		WithCapabilities(model.Capabilities{StructuredOutput: true}).
		WithResponse(`{"name": "John"}`).
		Build()

	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{},
		schema.WithValidationPolicy(schema.ValidationStrict()),
	)
	require.NoError(t, err)

	compiled, err := NewReAct(mdl, WithOutputSchema(&outputSchema))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	_, err = graph.Last(compiled.Run(ctx, input))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema validation failed")
}

func TestReAct_WithSchemaValidation_RetryMode_SucceedsAfterRetry(t *testing.T) {
	type TestOutput struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age" jsonschema:"required"`
	}

	callCount := 0
	mdl := &testutil.MockModel{
		GenerateFunc: testutil.WrapSimpleGenerate(func(ctx context.Context, messages []message.Message) (message.Message, error) {
			callCount++
			if callCount == 1 {
				// First call: return invalid output
				return message.NewAIMessageFromText(`{"name": "John"}`), nil
			}
			// Second call (retry): return valid output
			return message.NewAIMessageFromText(`{"name": "John", "age": 30}`), nil
		}),
		CapabilitiesFunc: func() model.Capabilities {
			return model.Capabilities{StructuredOutput: true}
		},
	}

	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{},
		schema.WithValidationPolicy(schema.ValidationWithRetry(3)),
	)
	require.NoError(t, err)

	compiled, err := NewReAct(mdl, WithOutputSchema(&outputSchema))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	events, err := graph.Collect(compiled.Run(ctx, input))

	require.NoError(t, err)
	require.NotEmpty(t, events)
	assert.Equal(t, 2, callCount, "Should have called model twice (initial + retry)")
}

func TestReAct_WithSchemaValidation_WarnMode(t *testing.T) {
	type TestOutput struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age" jsonschema:"required"`
	}

	// Model returns invalid JSON
	mdl := testutil.NewModelBuilder().
		WithCapabilities(model.Capabilities{StructuredOutput: true}).
		WithResponse(`{"name": "John"}`).
		Build()

	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{},
		schema.WithValidationPolicy(schema.ValidationWarnOnly()),
	)
	require.NoError(t, err)

	compiled, err := NewReAct(mdl, WithOutputSchema(&outputSchema))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	events, err := graph.Collect(compiled.Run(ctx, input))

	// Should succeed despite invalid output (warn mode)
	require.NoError(t, err)
	require.NotEmpty(t, events)

	// Verify we got the invalid response back (not blocked)
	lastEvent := events[len(events)-1]
	aiMsg, ok := lastEvent.(*message.AIMessage)
	require.True(t, ok)
	parts := aiMsg.Parts()
	require.NotEmpty(t, parts)
	if textPart, ok := parts[0].(message.TextPart); ok {
		assert.Contains(t, textPart.Text, `"name"`)
		assert.Contains(t, textPart.Text, "John")
	}
}

func TestReAct_WithSchemaValidation_DisabledPolicy(t *testing.T) {
	type TestOutput struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age" jsonschema:"required"`
	}

	// Model returns invalid JSON
	mdl := testutil.NewModelBuilder().
		WithCapabilities(model.Capabilities{StructuredOutput: true}).
		WithResponse(`{"name": "John"}`).
		Build()

	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{},
		schema.WithValidationPolicy(schema.ValidationDisabled()),
	)
	require.NoError(t, err)

	compiled, err := NewReAct(mdl, WithOutputSchema(&outputSchema))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	events, err := graph.Collect(compiled.Run(ctx, input))

	// Should succeed because validation is disabled
	require.NoError(t, err)
	require.NotEmpty(t, events)
}

func TestReAct_WithSchemaValidation_NoPolicy(t *testing.T) {
	type TestOutput struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age" jsonschema:"required"`
	}

	// Model returns invalid JSON
	mdl := testutil.NewModelBuilder().
		WithCapabilities(model.Capabilities{StructuredOutput: true}).
		WithResponse(`{"name": "John"}`).
		Build()

	// No validation policy set (Validation is nil)
	outputSchema, err := schema.NewOutputSchema("test_output", TestOutput{})
	require.NoError(t, err)

	compiled, err := NewReAct(mdl, WithOutputSchema(&outputSchema))
	require.NoError(t, err)

	ctx := context.Background()
	input := []message.Message{message.NewHumanMessageFromText("Test")}

	events, err := graph.Collect(compiled.Run(ctx, input))

	// Should succeed because no validation policy means validation middleware is not added
	require.NoError(t, err)
	require.NotEmpty(t, events)
}
