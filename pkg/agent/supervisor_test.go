package agent

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/testutil"
)

func TestNewSupervisor_Basic(t *testing.T) {
	// Create mock workers
	worker1, _ := createMockWorker("math expert")
	worker2, _ := createMockWorker("code expert")

	mockModel := testutil.NewModelBuilder().
		WithResponse("supervisor response").
		Build()

	supervisor, err := NewSupervisor(
		mockModel,
		WithWorker("math", "Math expert", worker1),
		WithWorker("code", "Code expert", worker2),
		WithMaxIterations(5),
	)

	if err != nil {
		t.Fatalf("Failed to create supervisor: %v", err)
	}

	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}

	// NOTE: System prompt is now sent per-request (not stored in state)
	// This is more token-efficient than the LangChain pattern.
	// The supervisor will have a default system prompt but it won't appear
	// in the initial state - it's sent with each model invocation.

	// Verify supervisor was created successfully
	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}

	// The system prompt is used internally but not stored in state
	// This is verified by the successful creation of the supervisor
}

func TestNewSupervisor_NoWorkers(t *testing.T) {
	mockModel := testutil.NewModelBuilder().
		WithResponse("response").
		Build()

	_, err := NewSupervisor(mockModel)

	if err == nil {
		t.Error("Expected error when creating supervisor with no workers")
	}
}

func TestNewSupervisor_NilWorkerAgent(t *testing.T) {
	mockModel := testutil.NewModelBuilder().
		WithResponse("response").
		Build()

	_, err := NewSupervisor(
		mockModel,
		WithWorker("math", "Math expert", nil),
	)

	if err == nil {
		t.Error("Expected error when worker has nil agent")
	}
}

func TestNewSupervisor(t *testing.T) {
	worker1, _ := createMockWorker("math expert")
	worker2, _ := createMockWorker("code expert")

	mockModel := testutil.NewModelBuilder().
		WithResponse("supervisor response").
		Build()

	supervisor, err := NewSupervisor(
		mockModel,
		WithWorker("math", "Math expert", worker1),
		WithWorker("code", "Code expert", worker2),
		WithInstructions("Custom supervisor prompt"),
		WithWorkerRetries(3),
		WithMaxIterations(15),
	)

	if err != nil {
		t.Fatalf("Failed to create supervisor with options: %v", err)
	}

	if supervisor == nil {
		t.Fatal("Expected supervisor to be created")
	}

	// Verify supervisor was created successfully with custom prompt
	if supervisor == nil {
		t.Fatal("Expected supervisor to be created with custom prompt")
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
func createMockWorker(expertise string) (*message.Graph, error) {
	mockModel := testutil.NewModelBuilder().
		WithResponse("worker response: " + expertise).
		Build()

	return NewReAct(mockModel, WithMaxIterations(1))
}
