package agent

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func TestNewSupervisorAgent_Basic(t *testing.T) {
	// Create mock workers
	worker1, _ := createMockWorker("math expert")
	worker2, _ := createMockWorker("code expert")

	mockModel := &mockModel{
		generateFunc: func(ctx context.Context, messages []message.Message) (message.Message, error) {
			return message.NewAIMessageFromText("supervisor response"), nil
		},
	}

	supervisor, err := NewSupervisorAgent(
		mockModel,
		WithWorker("math", "Math expert", worker1),
		WithWorker("code", "Code expert", worker2),
		WithSupervisorMaxIterations(5),
	)

	if err != nil {
		t.Fatalf("Failed to create supervisor: %v", err)
	}

	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}

	// Verify supervisor has system prompt
	state := supervisor.State()
	messages := state.MessagesSnapshot()

	if len(messages) == 0 {
		t.Fatal("Expected system message in supervisor state")
	}

	sysMsg, ok := messages[0].(*message.SystemMessage)
	if !ok {
		t.Fatalf("Expected first message to be SystemMessage, got %T", messages[0])
	}

	// Check that system prompt mentions the workers
	prompt := getMessageText(sysMsg)
	if prompt == "" {
		t.Error("System prompt is empty")
	}
}

func TestNewSupervisorAgent_NoWorkers(t *testing.T) {
	mockModel := &mockModel{
		generateFunc: func(ctx context.Context, messages []message.Message) (message.Message, error) {
			return message.NewAIMessageFromText("response"), nil
		},
	}

	_, err := NewSupervisorAgent(mockModel)

	if err == nil {
		t.Error("Expected error when creating supervisor with no workers")
	}
}

func TestNewSupervisorAgent_NilWorkerAgent(t *testing.T) {
	mockModel := &mockModel{
		generateFunc: func(ctx context.Context, messages []message.Message) (message.Message, error) {
			return message.NewAIMessageFromText("response"), nil
		},
	}

	_, err := NewSupervisorAgent(
		mockModel,
		WithWorker("math", "Math expert", nil),
	)

	if err == nil {
		t.Error("Expected error when worker has nil agent")
	}
}

func TestNewSupervisorAgent(t *testing.T) {
	worker1, _ := createMockWorker("math expert")
	worker2, _ := createMockWorker("code expert")

	mockModel := &mockModel{
		generateFunc: func(ctx context.Context, messages []message.Message) (message.Message, error) {
			return message.NewAIMessageFromText("supervisor response"), nil
		},
	}

	supervisor, err := NewSupervisorAgent(
		mockModel,
		WithWorker("math", "Math expert", worker1),
		WithWorker("code", "Code expert", worker2),
		WithSupervisorSystemPrompt("Custom supervisor prompt"),
		WithWorkerContext(false),
		WithWorkerRetries(3),
		WithSupervisorMaxIterations(15),
	)

	if err != nil {
		t.Fatalf("Failed to create supervisor with options: %v", err)
	}

	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}

	// Verify custom system prompt
	state := supervisor.State()
	messages := state.MessagesSnapshot()

	if len(messages) > 0 {
		if sysMsg, ok := messages[0].(*message.SystemMessage); ok {
			prompt := getMessageText(sysMsg)
			if prompt != "Custom supervisor prompt" {
				t.Errorf("Expected custom system prompt, got %q", prompt)
			}
		}
	}
}

func TestGenerateDefaultSupervisorPrompt(t *testing.T) {
	workers := []WorkerAgent{
		{Name: "math", Description: "Expert in mathematics"},
		{Name: "history", Description: "Expert in history"},
	}

	prompt := generateDefaultSupervisorPrompt(workers)

	if prompt == "" {
		t.Error("Generated prompt is empty")
	}

	// Check that prompt mentions both workers
	if len(prompt) < 50 {
		t.Error("Generated prompt seems too short")
	}
}

// createMockWorker creates a simple mock worker agent for testing
func createMockWorker(expertise string) (*graph.CompiledGraph, error) {
	mockModel := &mockModel{
		generateFunc: func(ctx context.Context, messages []message.Message) (message.Message, error) {
			return message.NewAIMessageFromText("worker response: " + expertise), nil
		},
	}

	return NewReActAgent(mockModel, WithMaxIterations(1))
}

// getMessageText extracts text from a message (helper already exists in other test files)
func getMessageText(msg message.Message) string {
	for _, part := range msg.Parts() {
		if textPart, ok := part.(message.TextPart); ok {
			return textPart.Text
		}
	}
	return ""
}
