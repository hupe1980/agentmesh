package callbacks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Test helpers

// mockStateWriter is a simple mock implementation of graph.StateWriter for testing
type mockStateWriter struct {
	messages   []message.Message
	state      map[string]any
	aggregates map[string]any
}

func newMockStateWriter() *mockStateWriter {
	return &mockStateWriter{
		messages:   []message.Message{message.NewHumanMessageFromText("test message")},
		state:      make(map[string]any),
		aggregates: make(map[string]any),
	}
}

func (m *mockStateWriter) Get(key string) any {
	return m.state[key]
}

func (m *mockStateWriter) GetAll() map[string]any {
	return m.state
}

func (m *mockStateWriter) Set(key string, value any) {
	m.state[key] = value
}

func (m *mockStateWriter) MessageEventsSnapshot() []graph.MessageEvent {
	events := make([]graph.MessageEvent, len(m.messages))
	for i, msg := range m.messages {
		events[i] = *graph.NewMessageEvent(msg, "", "")
	}
	return events
}

func (m *mockStateWriter) AggregatesSnapshot() map[string]any {
	return m.aggregates
}

func (m *mockStateWriter) Aggregate(name string, value any) error {
	m.aggregates[name] = value
	return nil
}

func createTestStateWriter() graph.StateWriter {
	return newMockStateWriter()
}

func createTestMessages() []message.Message {
	return []message.Message{
		message.NewHumanMessageFromText("test message"),
	}
}

func createTestToolCall() message.ToolCall {
	return message.ToolCall{
		ID:   "call_123",
		Name: "test_tool",
		Type: "function",
		Arguments: map[string]any{
			"arg1": "value1",
		},
	}
}

// BeforeModel Callback Tests

func TestBeforeModelCallback_NoShortCircuit(t *testing.T) {
	manager := NewManager()
	called := false

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		called = true
		return nil, nil // Continue to model
	})

	result, err := manager.ExecuteBeforeModel(context.Background(), createTestStateWriter())

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result (no short-circuit)")
	}
	if !called {
		t.Fatal("callback was not called")
	}
}

func TestBeforeModelCallback_ShortCircuit(t *testing.T) {
	manager := NewManager()
	cachedMsg := message.NewAIMessageFromText("cached response")

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		return cachedMsg, nil // Short-circuit with cached response
	})

	result, err := manager.ExecuteBeforeModel(context.Background(), createTestStateWriter())

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected short-circuit result")
	}
	if result != cachedMsg {
		t.Fatal("expected cached message as result")
	}
}

func TestBeforeModelCallback_Error(t *testing.T) {
	manager := NewManager()
	expectedErr := errors.New("validation failed")

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		return nil, expectedErr
	})

	result, err := manager.ExecuteBeforeModel(context.Background(), createTestStateWriter())

	if err != expectedErr {
		t.Fatalf("expected error %v, got: %v", expectedErr, err)
	}
	if result != nil {
		t.Fatal("expected nil result on error")
	}
}

func TestBeforeModelCallback_Multiple(t *testing.T) {
	manager := NewManager()
	callOrder := []int{}
	mu := sync.Mutex{}

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		mu.Lock()
		callOrder = append(callOrder, 1)
		mu.Unlock()
		return nil, nil
	})

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		mu.Lock()
		callOrder = append(callOrder, 2)
		mu.Unlock()
		return nil, nil
	})

	_, err := manager.ExecuteBeforeModel(context.Background(), createTestStateWriter())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(callOrder) != 2 || callOrder[0] != 1 || callOrder[1] != 2 {
		t.Fatalf("expected call order [1, 2], got: %v", callOrder)
	}
}

func TestBeforeModelCallback_StopsOnFirstShortCircuit(t *testing.T) {
	manager := NewManager()
	secondCalled := false

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		return message.NewAIMessageFromText("short-circuit"), nil
	})

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		secondCalled = true
		return nil, nil
	})

	_, err := manager.ExecuteBeforeModel(context.Background(), createTestStateWriter())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secondCalled {
		t.Fatal("second callback should not be called after short-circuit")
	}
}

func TestBeforeModelCallback_Panic(t *testing.T) {
	manager := NewManager()

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		panic("something went wrong")
	})

	result, err := manager.ExecuteBeforeModel(context.Background(), createTestStateWriter())

	if err == nil {
		t.Fatal("expected panic to be converted to error")
	}
	if result != nil {
		t.Fatal("expected nil result after panic")
	}
	if !errors.Is(err, err) || err.Error() == "" {
		t.Fatalf("expected panic error, got: %v", err)
	}
}

// OnModelError Callback Tests

func TestOnModelErrorCallback_Propagate(t *testing.T) {
	manager := NewManager()
	originalErr := errors.New("model failed")

	manager.RegisterOnModelError(func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		return nil, err // Propagate
	})

	result, err := manager.ExecuteOnModelError(context.Background(), createTestStateWriter(), originalErr)

	if err != originalErr {
		t.Fatalf("expected original error, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result")
	}
}

func TestOnModelErrorCallback_Fallback(t *testing.T) {
	manager := NewManager()
	fallback := message.NewAIMessageFromText("fallback response")

	manager.RegisterOnModelError(func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		return fallback, nil
	})

	result, err := manager.ExecuteOnModelError(context.Background(), createTestStateWriter(), errors.New("model failed"))

	if err != nil {
		t.Fatalf("expected no error with fallback, got: %v", err)
	}
	if result != fallback {
		t.Fatal("expected fallback result")
	}
}

func TestOnModelErrorCallback_TransformError(t *testing.T) {
	manager := NewManager()
	wrappedErr := fmt.Errorf("wrapped error")

	manager.RegisterOnModelError(func(ctx context.Context, s graph.StateWriter, err error) (message.Message, error) {
		return nil, wrappedErr
	})

	result, err := manager.ExecuteOnModelError(context.Background(), createTestStateWriter(), errors.New("original"))

	if err != wrappedErr {
		t.Fatalf("expected wrapped error, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result")
	}
}

// AfterModel Callback Tests

func TestAfterModelCallback_NoTransform(t *testing.T) {
	manager := NewManager()
	called := false

	manager.RegisterAfterModel(func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		called = true
		return nil, nil // Keep original
	})

	original := message.NewAIMessageFromText("original response")
	result, err := manager.ExecuteAfterModel(context.Background(), createTestStateWriter(), original)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != original {
		t.Fatal("expected original response")
	}
	if !called {
		t.Fatal("callback was not called")
	}
}

func TestAfterModelCallback_Transform(t *testing.T) {
	manager := NewManager()
	transformed := message.NewAIMessageFromText("filtered response")

	manager.RegisterAfterModel(func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		return transformed, nil
	})

	original := message.NewAIMessageFromText("toxic content")
	result, err := manager.ExecuteAfterModel(context.Background(), createTestStateWriter(), original)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != transformed {
		t.Fatal("expected transformed response")
	}
}

func TestAfterModelCallback_ChainedTransforms(t *testing.T) {
	manager := NewManager()

	manager.RegisterAfterModel(func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		return message.NewAIMessageFromText("step1"), nil
	})

	manager.RegisterAfterModel(func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		// Verify we receive the transformed message from step 1
		if ai, ok := response.(*message.AIMessage); ok {
			parts := ai.Parts()
			if len(parts) > 0 {
				if text, ok := parts[0].(message.TextPart); ok && text.Text == "step1" {
					return message.NewAIMessageFromText("step2"), nil
				}
			}
		}
		return nil, errors.New("unexpected response")
	})

	original := message.NewAIMessageFromText("original")
	result, err := manager.ExecuteAfterModel(context.Background(), createTestStateWriter(), original)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ai, ok := result.(*message.AIMessage); ok {
		parts := ai.Parts()
		if len(parts) > 0 {
			if text, ok := parts[0].(message.TextPart); ok && text.Text != "step2" {
				t.Fatalf("expected 'step2', got: %s", text.Text)
			}
		} else {
			t.Fatal("expected parts in result")
		}
	} else {
		t.Fatal("expected AIMessage result")
	}
}

// BeforeTool Callback Tests

func TestBeforeToolCallback_NoShortCircuit(t *testing.T) {
	manager := NewManager()
	called := false

	manager.RegisterBeforeTool(func(ctx context.Context, s graph.StateWriter, call message.ToolCall) (any, error) {
		called = true
		return nil, nil
	})

	call := createTestToolCall()
	result, err := manager.ExecuteBeforeTool(context.Background(), createTestStateWriter(), call)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result")
	}
	if !called {
		t.Fatal("callback was not called")
	}
}

func TestBeforeToolCallback_ShortCircuit(t *testing.T) {
	manager := NewManager()
	mockResult := "cached result"

	manager.RegisterBeforeTool(func(ctx context.Context, s graph.StateWriter, call message.ToolCall) (any, error) {
		return mockResult, nil
	})

	call := createTestToolCall()
	result, err := manager.ExecuteBeforeTool(context.Background(), createTestStateWriter(), call)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected mock result")
	}
	if result.(string) != mockResult {
		t.Fatalf("expected '%s', got: %v", mockResult, result)
	}
}

func TestBeforeToolCallback_Error(t *testing.T) {
	manager := NewManager()
	expectedErr := errors.New("permission denied")

	manager.RegisterBeforeTool(func(ctx context.Context, s graph.StateWriter, call message.ToolCall) (any, error) {
		return nil, expectedErr
	})

	call := createTestToolCall()
	result, err := manager.ExecuteBeforeTool(context.Background(), createTestStateWriter(), call)

	if err != expectedErr {
		t.Fatalf("expected error %v, got: %v", expectedErr, err)
	}
	if result != nil {
		t.Fatal("expected nil result on error")
	}
}

// AfterTool Callback Tests

func TestAfterToolCallback_NoTransform(t *testing.T) {
	manager := NewManager()
	called := false

	manager.RegisterAfterTool(func(ctx context.Context, s graph.StateWriter, call message.ToolCall, result any) (any, error) {
		called = true
		return nil, nil
	})

	call := createTestToolCall()
	original := "original result"
	result, err := manager.ExecuteAfterTool(context.Background(), createTestStateWriter(), call, original)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != original {
		t.Fatal("expected original result")
	}
	if !called {
		t.Fatal("callback was not called")
	}
}

func TestAfterToolCallback_Transform(t *testing.T) {
	manager := NewManager()
	transformed := "transformed result"

	manager.RegisterAfterTool(func(ctx context.Context, s graph.StateWriter, call message.ToolCall, result any) (any, error) {
		return transformed, nil
	})

	call := createTestToolCall()
	original := "original result"
	result, err := manager.ExecuteAfterTool(context.Background(), createTestStateWriter(), call, original)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != transformed {
		t.Fatal("expected transformed result")
	}
}

// OnToolError Callback Tests

func TestOnToolErrorCallback_Propagate(t *testing.T) {
	manager := NewManager()
	originalErr := errors.New("tool failed")

	manager.RegisterOnToolError(func(ctx context.Context, s graph.StateWriter, call message.ToolCall, err error) (any, error) {
		return nil, err // Propagate
	})

	call := createTestToolCall()
	result, err := manager.ExecuteOnToolError(context.Background(), createTestStateWriter(), call, originalErr)

	if err != originalErr {
		t.Fatalf("expected original error, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result")
	}
}

func TestOnToolErrorCallback_Fallback(t *testing.T) {
	manager := NewManager()
	fallback := "fallback value"

	manager.RegisterOnToolError(func(ctx context.Context, s graph.StateWriter, call message.ToolCall, err error) (any, error) {
		return fallback, nil
	})

	call := createTestToolCall()
	result, err := manager.ExecuteOnToolError(context.Background(), createTestStateWriter(), call, errors.New("timeout"))

	if err != nil {
		t.Fatalf("expected no error with fallback, got: %v", err)
	}
	if result != fallback {
		t.Fatal("expected fallback result")
	}
}

func TestOnToolErrorCallback_TransformError(t *testing.T) {
	manager := NewManager()
	wrappedErr := fmt.Errorf("wrapped error")

	manager.RegisterOnToolError(func(ctx context.Context, s graph.StateWriter, call message.ToolCall, err error) (any, error) {
		return nil, wrappedErr
	})

	call := createTestToolCall()
	result, err := manager.ExecuteOnToolError(context.Background(), createTestStateWriter(), call, errors.New("original"))

	if err != wrappedErr {
		t.Fatalf("expected wrapped error, got: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result")
	}
}

// HasCallbacks Tests

func TestHasCallbacks(t *testing.T) {
	manager := NewManager()

	if manager.HasBeforeModelCallbacks() {
		t.Fatal("expected no BeforeModel callbacks")
	}
	if manager.HasAfterModelCallbacks() {
		t.Fatal("expected no AfterModel callbacks")
	}

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		return nil, nil
	})

	if !manager.HasBeforeModelCallbacks() {
		t.Fatal("expected BeforeModel callbacks")
	}

	manager.RegisterAfterModel(func(ctx context.Context, s graph.StateWriter, response message.Message) (message.Message, error) {
		return nil, nil
	})

	if !manager.HasAfterModelCallbacks() {
		t.Fatal("expected AfterModel callbacks")
	}
}

// Concurrency Tests

func TestConcurrentRegistration(t *testing.T) {
	manager := NewManager()
	var wg sync.WaitGroup

	// Register callbacks concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
				return nil, nil
			})
		}()
	}

	wg.Wait()

	if !manager.HasBeforeModelCallbacks() {
		t.Fatal("expected callbacks to be registered")
	}
}

func TestConcurrentExecution(t *testing.T) {
	manager := NewManager()
	counter := 0
	mu := sync.Mutex{}

	manager.RegisterBeforeModel(func(ctx context.Context, s graph.StateWriter) (message.Message, error) {
		mu.Lock()
		counter++
		mu.Unlock()
		return nil, nil
	})

	var wg sync.WaitGroup

	// Execute callbacks concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = manager.ExecuteBeforeModel(context.Background(), createTestStateWriter())
		}()
	}

	wg.Wait()

	if counter != 100 {
		t.Fatalf("expected 100 executions, got: %d", counter)
	}
}
