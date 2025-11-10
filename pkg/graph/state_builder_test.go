package graph

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func TestNewStateBuilder(t *testing.T) {
	builder := NewStateBuilder()
	if builder == nil {
		t.Fatal("NewStateBuilder returned nil")
	}
	if builder.maxMessages != 100 {
		t.Errorf("Expected default maxMessages=100, got %d", builder.maxMessages)
	}
}

func TestStateBuilder_WithMessages(t *testing.T) {
	state := NewStateBuilder().
		WithMessages(50).
		Build()

	// Verify the messages channel has correct limit
	ch, ok := state.GetChannel("messages")
	if !ok {
		t.Fatal("messages channel not found")
	}

	tc, ok := ch.(*channel.TopicChannel)
	if !ok {
		t.Fatal("messages channel is not a TopicChannel")
	}

	// Note: TopicChannel doesn't expose maxValues directly in current implementation
	// We can verify it works by checking behavior
	_ = tc
}

func TestStateBuilder_WithUnlimitedMessages(t *testing.T) {
	state := NewStateBuilder().
		WithUnlimitedMessages().
		Build()

	ch, ok := state.GetChannel("messages")
	if !ok {
		t.Fatal("messages channel not found")
	}

	if _, ok := ch.(*channel.TopicChannel); !ok {
		t.Fatal("messages channel is not a TopicChannel")
	}
}

func TestStateBuilder_WithLastValue(t *testing.T) {
	state := NewStateBuilder().
		WithLastValue("status", "pending").
		WithLastValue("temperature", 72).
		Build()

	// Check status channel
	if _, ok := state.GetChannel("status"); !ok {
		t.Error("status channel not found")
	}

	status := state.Get("status")
	if status != "pending" {
		t.Errorf("Expected status='pending', got %v", status)
	}

	// Check temperature channel
	if _, ok := state.GetChannel("temperature"); !ok {
		t.Error("temperature channel not found")
	}

	temp := state.Get("temperature")
	if temp != 72 {
		t.Errorf("Expected temperature=72, got %v", temp)
	}
}

func TestStateBuilder_WithCounter(t *testing.T) {
	state := NewStateBuilder().
		WithCounter("iterations").
		WithCounter("score").
		Build()

	// Check counter channels exist
	if _, ok := state.GetChannel("iterations"); !ok {
		t.Error("iterations channel not found")
	}
	if _, ok := state.GetChannel("score"); !ok {
		t.Error("score channel not found")
	}

	// Verify initial value is 0
	iterations := state.Get("iterations")
	if iterations != 0 {
		t.Errorf("Expected iterations=0, got %v", iterations)
	}

	// Test accumulation
	state.Set("iterations", 1)
	state.Set("iterations", 2)
	state.Set("iterations", 3)

	total := state.Get("iterations")
	if total != 6 { // 0 + 1 + 2 + 3
		t.Errorf("Expected accumulated iterations=6, got %v", total)
	}
}

func TestStateBuilder_WithFlag(t *testing.T) {
	state := NewStateBuilder().
		WithFlag("completed").
		WithFlag("validated").
		Build()

	// Check flags exist and start false
	completed := state.Get("completed")
	if completed != false {
		t.Errorf("Expected completed=false, got %v", completed)
	}

	validated := state.Get("validated")
	if validated != false {
		t.Errorf("Expected validated=false, got %v", validated)
	}

	// Test setting flag
	state.Set("completed", true)
	completed = state.Get("completed")
	if completed != true {
		t.Errorf("Expected completed=true after set, got %v", completed)
	}
}

func TestStateBuilder_WithList(t *testing.T) {
	state := NewStateBuilder().
		WithList("action_history").
		WithList("errors").
		Build()

	// Check list channels exist
	if _, ok := state.GetChannel("action_history"); !ok {
		t.Error("action_history channel not found")
	}
	if _, ok := state.GetChannel("errors"); !ok {
		t.Error("errors channel not found")
	}

	// Test appending values
	state.Set("action_history", []string{"action1"})
	state.Set("action_history", []string{"action2"})
	state.Set("action_history", []string{"action3"})

	history := state.Get("action_history")
	historySlice, ok := history.([]any)
	if !ok {
		t.Fatalf("Expected action_history to be []any, got %T", history)
	}

	if len(historySlice) != 3 {
		t.Errorf("Expected 3 actions in history, got %d", len(historySlice))
	}
}

func TestStateBuilder_WithListLimit(t *testing.T) {
	state := NewStateBuilder().
		WithListLimit("recent_actions", 2).
		Build()

	// Add more items than the limit
	state.Set("recent_actions", []string{"action1"})
	state.Set("recent_actions", []string{"action2"})
	state.Set("recent_actions", []string{"action3"})

	recent := state.Get("recent_actions")
	recentSlice, ok := recent.([]any)
	if !ok {
		t.Fatalf("Expected recent_actions to be []any, got %T", recent)
	}

	// With limit of 2, should only keep last 2 items
	if len(recentSlice) > 2 {
		t.Errorf("Expected at most 2 items due to limit, got %d", len(recentSlice))
	}
}

func TestStateBuilder_WithMap(t *testing.T) {
	state := NewStateBuilder().
		WithMap("task_results").
		Build()

	// Check map channel exists
	if _, ok := state.GetChannel("task_results"); !ok {
		t.Error("task_results channel not found")
	}

	// Test merging maps
	state.Set("task_results", map[string]any{"task_a": "result_a"})
	state.Set("task_results", map[string]any{"task_b": "result_b"})

	results := state.Get("task_results")
	resultsMap, ok := results.(map[string]any)
	if !ok {
		t.Fatalf("Expected task_results to be map[string]any, got %T", results)
	}

	if len(resultsMap) != 2 {
		t.Errorf("Expected 2 entries in merged map, got %d", len(resultsMap))
	}

	if resultsMap["task_a"] != "result_a" {
		t.Errorf("Expected task_a='result_a', got %v", resultsMap["task_a"])
	}

	if resultsMap["task_b"] != "result_b" {
		t.Errorf("Expected task_b='result_b', got %v", resultsMap["task_b"])
	}
}

func TestStateBuilder_WithBinaryOp(t *testing.T) {
	// Create a custom reducer that concatenates strings
	concat := func(oldValue, newValue any) any {
		oldStr, _ := oldValue.(string)
		newStr, _ := newValue.(string)
		return oldStr + newStr
	}

	state := NewStateBuilder().
		WithBinaryOp("concatenated", "", concat).
		Build()

	// Test custom reducer
	state.Set("concatenated", "Hello")
	state.Set("concatenated", " ")
	state.Set("concatenated", "World")

	result := state.Get("concatenated")
	if result != "Hello World" {
		t.Errorf("Expected 'Hello World', got %v", result)
	}
}

func TestStateBuilder_WithChannel(t *testing.T) {
	customChannel := channel.NewLastValueChannel("custom")

	state := NewStateBuilder().
		WithChannel(customChannel).
		Build()

	// Check custom channel exists
	if _, ok := state.GetChannel("custom"); !ok {
		t.Error("custom channel not found")
	}
}

func TestStateBuilder_ChainedCalls(t *testing.T) {
	// Test fluent API with multiple chained calls
	state := NewStateBuilder().
		WithMessages(50).
		WithLastValue("status", "running").
		WithCounter("iterations").
		WithFlag("completed").
		WithList("logs").
		WithMap("results").
		Build()

	// Verify all channels exist
	channels := []string{"messages", "status", "iterations", "completed", "logs", "results"}
	for _, name := range channels {
		if _, ok := state.GetChannel(name); !ok {
			t.Errorf("Channel %s not found", name)
		}
	}

	// Verify initial values
	if state.Get("status") != "running" {
		t.Error("status not set correctly")
	}
	if state.Get("iterations") != 0 {
		t.Error("iterations not initialized to 0")
	}
	if state.Get("completed") != false {
		t.Error("completed flag not initialized to false")
	}
}

func TestStateBuilder_EmptyBuild(t *testing.T) {
	// Build with no additional channels (just default messages)
	state := NewStateBuilder().Build()

	if state == nil {
		t.Fatal("Build returned nil")
	}

	// Should at least have messages channel
	if _, ok := state.GetChannel("messages"); !ok {
		t.Error("Default messages channel not found")
	}
}

func TestStateBuilder_ComplexWorkflow(t *testing.T) {
	// Simulate a complex workflow state setup
	state := NewStateBuilder().
		WithMessages(100).
		WithLastValue("current_phase", "initialization").
		WithCounter("total_attempts").
		WithFlag("validation_passed").
		WithList("error_log").
		WithMap("phase_results").
		WithBinaryOp("score_sum", 0.0, func(old, new any) any {
			oldF, _ := old.(float64)
			newF, _ := new.(float64)
			return oldF + newF
		}).
		Build()

	// Simulate workflow operations
	state.Set("current_phase", "processing")
	state.Set("total_attempts", 1)
	state.Set("total_attempts", 1)
	state.Set("total_attempts", 1)
	state.Set("validation_passed", true)
	state.Set("error_log", []string{"warning: high latency"})
	state.Set("phase_results", map[string]any{"init": "success"})
	state.Set("phase_results", map[string]any{"process": "success"})
	state.Set("score_sum", 0.95)
	state.Set("score_sum", 0.92)

	// Verify final state
	if state.Get("current_phase") != "processing" {
		t.Error("Phase not updated correctly")
	}

	attempts := state.Get("total_attempts")
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %v", attempts)
	}

	if state.Get("validation_passed") != true {
		t.Error("Validation flag not set")
	}

	results := state.Get("phase_results").(map[string]any)
	if len(results) != 2 {
		t.Errorf("Expected 2 phase results, got %d", len(results))
	}

	score := state.Get("score_sum").(float64)
	if score != 1.87 {
		t.Errorf("Expected score 1.87, got %f", score)
	}
}

func TestStateBuilder_WithInitialMessages(t *testing.T) {
	systemMsg := message.NewSystemMessageFromText("You are a helpful assistant")
	humanMsg := message.NewHumanMessageFromText("Hello")

	state := NewStateBuilder().
		WithUnlimitedMessages().
		WithInitialMessages(systemMsg, humanMsg).
		Build()

	events := state.MessageEventsSnapshot()
	if len(events) != 2 {
		t.Fatalf("Expected 2 initial messages, got %d", len(events))
	}

	// Verify first message is system message
	if _, ok := events[0].Message.(*message.SystemMessage); !ok {
		t.Error("First message should be SystemMessage")
	}

	// Verify second message is human message
	if _, ok := events[1].Message.(*message.HumanMessage); !ok {
		t.Error("Second message should be HumanMessage")
	}

	// Verify message content
	if text := getMessageText(events[0].Message); text != "You are a helpful assistant" {
		t.Errorf("Expected system message text 'You are a helpful assistant', got %q", text)
	}

	if text := getMessageText(events[1].Message); text != "Hello" {
		t.Errorf("Expected human message text 'Hello', got %q", text)
	}
}
