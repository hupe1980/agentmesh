package testutil

import (
	"context"
	"errors"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

// Common test errors.
var (
	ErrMockAPI     = errors.New("mock API error")
	ErrMockTimeout = errors.New("mock timeout error")
	ErrMockInvalid = errors.New("mock invalid input error")
)

// Scenario represents a pre-built test scenario.
type Scenario struct {
	Name        string
	Model       *MockModel
	Tools       []*MockTool
	Recorder    *ConversationRecorder
	Description string
}

// SimpleResponseScenario creates a scenario with a single response.
func SimpleResponseScenario(response string) *Scenario {
	recorder := NewConversationRecorder()
	return &Scenario{
		Name:        "simple_response",
		Description: "Model returns a single text response",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithResponse(response).
			Build(),
		Recorder: recorder,
	}
}

// MultiTurnScenario creates a scenario with multiple sequential responses.
func MultiTurnScenario(responses ...string) *Scenario {
	recorder := NewConversationRecorder()
	return &Scenario{
		Name:        "multi_turn",
		Description: "Model returns different responses for each turn",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithResponses(responses...).
			Build(),
		Recorder: recorder,
	}
}

// ToolCallingScenario creates a scenario where the model calls a tool.
func ToolCallingScenario(toolName, toolResult, finalResponse string) *Scenario {
	recorder := NewConversationRecorder()

	mockTool := NewToolBuilder(toolName).
		WithResult(toolResult).
		Build()

	return &Scenario{
		Name:        "tool_calling",
		Description: "Model calls a tool and then provides final response",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithToolCalls(message.ToolCall{
				ID:        "call_1",
				Name:      toolName,
				Type:      "function",
				Arguments: "{}",
			}).
			WithResponse(finalResponse).
			Build(),
		Tools:    []*MockTool{mockTool},
		Recorder: recorder,
	}
}

// ErrorScenario creates a scenario where the model returns an error.
func ErrorScenario(err error) *Scenario {
	recorder := NewConversationRecorder()
	return &Scenario{
		Name:        "error",
		Description: "Model returns an error",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithError(err).
			Build(),
		Recorder: recorder,
	}
}

// TimeoutScenario creates a scenario for testing timeout handling.
func TimeoutScenario(delay time.Duration) *Scenario {
	recorder := NewConversationRecorder()
	return &Scenario{
		Name:        "timeout",
		Description: "Model response is delayed for timeout testing",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithDelay(delay).
			WithResponse("delayed response").
			Build(),
		Recorder: recorder,
	}
}

// StreamingScenario creates a scenario for testing streaming responses.
func StreamingScenario(fullResponse string) *Scenario {
	recorder := NewConversationRecorder()
	return &Scenario{
		Name:        "streaming",
		Description: "Model streams response in chunks",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithStreaming(true).
			WithResponse(fullResponse).
			Build(),
		Recorder: recorder,
	}
}

// RetryScenario creates a scenario where first N calls fail, then succeeds.
func RetryScenario(failCount int, successResponse string) *Scenario {
	recorder := NewConversationRecorder()
	callCount := 0

	customGen := func(ctx context.Context, messages []message.Message) (message.Message, error) {
		callCount++
		if callCount <= failCount {
			return nil, ErrMockAPI
		}
		return message.NewAIMessageFromText(successResponse), nil
	}

	return &Scenario{
		Name:        "retry",
		Description: "Model fails N times then succeeds",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithGenerator(WrapSimpleGenerate(customGen)).
			Build(),
		Recorder: recorder,
	}
}

// StructuredOutputScenario creates a scenario for testing structured output.
func StructuredOutputScenario(supportsStructured bool, response string) *Scenario {
	recorder := NewConversationRecorder()
	caps := model.Capabilities{
		Streaming:        true,
		Tools:            true,
		StructuredOutput: supportsStructured,
	}

	return &Scenario{
		Name:        "structured_output",
		Description: "Model with configurable structured output support",
		Model: NewModelBuilder().
			WithRecorder(recorder).
			WithCapabilities(caps).
			WithResponse(response).
			Build(),
		Recorder: recorder,
	}
}

// ChainedToolCallsScenario creates a scenario with multiple sequential tool calls.
func ChainedToolCallsScenario(toolCalls []message.ToolCall, toolResults []string, finalResponse string) *Scenario {
	recorder := NewConversationRecorder()

	tools := make([]*MockTool, 0, len(toolCalls))
	builder := NewModelBuilder().WithRecorder(recorder)

	for i, tc := range toolCalls {
		builder = builder.WithToolCalls(tc)
		tool := NewToolBuilder(tc.Name).
			WithResult(toolResults[i]).
			Build()
		tools = append(tools, tool)
	}

	builder = builder.WithResponse(finalResponse)

	return &Scenario{
		Name:        "chained_tool_calls",
		Description: "Model makes multiple sequential tool calls",
		Model:       builder.Build(),
		Tools:       tools,
		Recorder:    recorder,
	}
}
