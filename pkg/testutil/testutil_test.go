package testutil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/model"
)

func TestModelBuilder_SimpleResponse(t *testing.T) {
	mdl := NewModelBuilder().
		WithResponse("Hello, world!").
		Build()

	ctx := context.Background()
	req := &model.Request{Messages: []message.Message{message.NewHumanMessageFromText("Hi")}}

	var responses []*model.Response
	for resp, err := range mdl.Generate(ctx, req) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		responses = append(responses, resp)
	}

	if len(responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(responses))
	}
	if responses[0].Message.String() != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", responses[0].Message.String())
	}
}

func TestModelBuilder_MultipleResponses(t *testing.T) {
	mdl := NewModelBuilder().
		WithResponses("First", "Second", "Third").
		Build()

	ctx := context.Background()

	// First call
	for resp, err := range mdl.Generate(ctx, &model.Request{}) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.Partial && resp.Message.String() != "First" {
			t.Errorf("expected 'First', got %q", resp.Message.String())
		}
	}

	// Second call
	for resp, err := range mdl.Generate(ctx, &model.Request{}) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.Partial && resp.Message.String() != "Second" {
			t.Errorf("expected 'Second', got %q", resp.Message.String())
		}
	}
}

func TestModelBuilder_WithError(t *testing.T) {
	expectedErr := errors.New("test error")
	mdl := NewModelBuilder().
		WithError(expectedErr).
		Build()

	ctx := context.Background()
	for _, err := range mdl.Generate(ctx, &model.Request{}) {
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	}
}

func TestModelBuilder_WithRecorder(t *testing.T) {
	recorder := NewConversationRecorder()
	mdl := NewModelBuilder().
		WithRecorder(recorder).
		WithResponse("test").
		Build()

	ctx := context.Background()
	req := &model.Request{Messages: []message.Message{message.NewHumanMessageFromText("hello")}}

	for range mdl.Generate(ctx, req) {
		// consume responses
	}

	if recorder.RequestCount() != 1 {
		t.Errorf("expected 1 request, got %d", recorder.RequestCount())
	}
}

func TestModelBuilder_Capabilities(t *testing.T) {
	mdl := NewModelBuilder().
		WithStructuredOutput(true).
		WithTools(false).
		Build()

	caps := mdl.Capabilities()
	if !caps.StructuredOutput {
		t.Error("expected StructuredOutput to be true")
	}
	if caps.Tools {
		t.Error("expected Tools to be false")
	}
}

func TestToolBuilder_SimpleResult(t *testing.T) {
	tool := NewToolBuilder("test_tool").
		WithDescription("A test tool").
		WithResult("result value").
		Build()

	if tool.Name() != "test_tool" {
		t.Errorf("expected 'test_tool', got %q", tool.Name())
	}

	result, err := tool.Call(context.Background(), "{}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "result value" {
		t.Errorf("expected 'result value', got %v", result)
	}
}

func TestToolBuilder_CallCount(t *testing.T) {
	tool := NewToolBuilder("counter").
		WithResult("ok").
		Build()

	for i := 0; i < 3; i++ {
		_, _ = tool.Call(context.Background(), "{}")
	}

	if tool.CallCount() != 3 {
		t.Errorf("expected 3 calls, got %d", tool.CallCount())
	}
}

func TestToolBuilder_CustomCall(t *testing.T) {
	tool := NewToolBuilder("custom").
		WithCall(func(ctx context.Context, args string) (any, error) {
			return "custom: " + args, nil
		}).
		Build()

	result, err := tool.Call(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "custom: test" {
		t.Errorf("expected 'custom: test', got %v", result)
	}
}

func TestConversationRecorder(t *testing.T) {
	recorder := NewConversationRecorder()

	recorder.RecordRequest(&model.Request{
		Messages: []message.Message{message.NewHumanMessageFromText("hello")},
	})
	recorder.RecordResponse(&model.Response{
		Message: message.NewAIMessageFromText("hi there"),
	})

	if recorder.RequestCount() != 1 {
		t.Errorf("expected 1 request, got %d", recorder.RequestCount())
	}
	if recorder.ResponseCount() != 1 {
		t.Errorf("expected 1 response, got %d", recorder.ResponseCount())
	}
}

func TestAssertions(t *testing.T) {
	messages := []message.Message{
		message.NewHumanMessageFromText("Hello"),
		message.NewAIMessageFromText("Hi there!"),
	}

	// Test IsHuman matcher
	humanMatcher := IsHuman("Hello")
	if !humanMatcher(messages[0]) {
		t.Error("IsHuman matcher should match")
	}

	// Test IsAI matcher
	aiMatcher := IsAI(Contains("Hi"))
	if !aiMatcher(messages[1]) {
		t.Error("IsAI matcher should match")
	}

	// Test AssertMessages
	AssertMessages(t, messages, IsHuman("Hello"), IsAI())
}

func TestAssertEventually(t *testing.T) {
	var flag atomic.Bool
	go func() {
		time.Sleep(50 * time.Millisecond)
		flag.Store(true)
	}()

	AssertEventually(t, func() bool {
		return flag.Load()
	}, 200*time.Millisecond)
}

func TestMockMemory(t *testing.T) {
	mem := NewMockMemory()
	ctx := context.Background()

	err := mem.Store(ctx, "session1", []message.Message{
		message.NewHumanMessageFromText("Hello"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mem.MessageCount("session1") != 1 {
		t.Errorf("expected 1 message, got %d", mem.MessageCount("session1"))
	}
}

func TestScenarios(t *testing.T) {
	t.Run("SimpleResponse", func(t *testing.T) {
		scenario := SimpleResponseScenario("Hello!")
		if scenario.Name != "simple_response" {
			t.Errorf("expected 'simple_response', got %q", scenario.Name)
		}
		if scenario.Model == nil {
			t.Error("Model should not be nil")
		}
		if scenario.Recorder == nil {
			t.Error("Recorder should not be nil")
		}
	})

	t.Run("ToolCalling", func(t *testing.T) {
		scenario := ToolCallingScenario("search", "results", "Based on results...")
		if len(scenario.Tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(scenario.Tools))
		}
	})
}
