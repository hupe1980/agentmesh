package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
)

// TestEscalatingAgent is a mock agent that escalates after a certain number of runs
type TestEscalatingAgent struct {
	BaseAgent
	runCount    int
	escalateOn  int
	shouldError bool
}

func NewTestEscalatingAgent(name string, escalateOn int) *TestEscalatingAgent {
	return &TestEscalatingAgent{
		BaseAgent:  NewBaseAgent(name),
		escalateOn: escalateOn,
		runCount:   0,
	}
}

func (t *TestEscalatingAgent) Run(invocationCtx *core.InvocationContext) error {
	t.runCount++

	// Create a basic event
	ev := core.NewEvent(t.Name(), invocationCtx.InvocationID)

	if t.runCount >= t.escalateOn {
		// Create escalation event
		escalate := true
		ev.Actions.Escalate = &escalate
		ev.Content = &core.Content{
			Role: "assistant",
			Parts: []core.Part{core.TextPart{
				Text: "Task complexity exceeds my capabilities, escalating to parent",
			}},
		}
	} else {
		// Regular event
		ev.Content = &core.Content{
			Role: "assistant",
			Parts: []core.Part{core.TextPart{
				Text: "Working on task iteration " + string(rune(t.runCount+'0')),
			}},
		}
	}

	// Emit the event
	if err := invocationCtx.EmitEvent(ev); err != nil {
		return err
	}

	// Wait for resume
	return invocationCtx.WaitForResume()
}

// TestRegularAgent is a mock agent that never escalates
type TestRegularAgent struct {
	BaseAgent
	runCount int
}

func NewTestRegularAgent(name string) *TestRegularAgent {
	return &TestRegularAgent{
		BaseAgent: NewBaseAgent(name),
		runCount:  0,
	}
}

func (t *TestRegularAgent) Run(invocationCtx *core.InvocationContext) error {
	t.runCount++

	// Create a regular event
	ev := core.NewEvent(t.Name(), invocationCtx.InvocationID)
	ev.Content = &core.Content{
		Role: "assistant",
		Parts: []core.Part{core.TextPart{
			Text: "Working on task iteration " + string(rune(t.runCount+'0')),
		}},
	}

	// Emit the event
	if err := invocationCtx.EmitEvent(ev); err != nil {
		return err
	}

	// Wait for resume
	return invocationCtx.WaitForResume()
}

func TestLoopAgent_EscalationHandling(t *testing.T) {
	tests := []struct {
		name               string
		childAgent         core.Agent
		maxIters           int
		expectedIterations int
		shouldEscalate     bool
	}{
		{
			name:               "Agent escalates on iteration 2",
			childAgent:         NewTestEscalatingAgent("escalator", 2),
			maxIters:           5,
			expectedIterations: 2,
			shouldEscalate:     true,
		},
		{
			name:               "Agent never escalates, completes all iterations",
			childAgent:         NewTestRegularAgent("regular"),
			maxIters:           3,
			expectedIterations: 3,
			shouldEscalate:     false,
		},
		{
			name:               "Agent escalates immediately",
			childAgent:         NewTestEscalatingAgent("immediate", 1),
			maxIters:           5,
			expectedIterations: 1,
			shouldEscalate:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create loop agent
			loopAgent := NewLoopAgent("TestLoop", tt.childAgent, WithMaxIters(tt.maxIters))

			// Create test context
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Create mock invocation context with proper bidirectional channels
			emitChan := make(chan core.Event, 10)
			resumeChan := make(chan struct{}, 10)

			invocationCtx := &core.InvocationContext{
				Context:      ctx,
				InvocationID: "test-invocation",
				Emit:         emitChan,
				Resume:       resumeChan,
			}

			// Track events in a separate goroutine
			var events []core.Event
			var eventWg sync.WaitGroup
			eventWg.Add(1)

			go func() {
				defer eventWg.Done()
				for event := range emitChan {
					events = append(events, event)
					// Send resume signal
					select {
					case resumeChan <- struct{}{}:
					case <-ctx.Done():
						return
					}
				}
			}()

			// Run the loop agent
			err := loopAgent.Run(invocationCtx)

			// Close channels and wait for event processing to complete
			close(emitChan)
			eventWg.Wait()
			close(resumeChan)

			// Verify results
			if err != nil {
				t.Errorf("Loop agent returned unexpected error: %v", err)
			}

			// Check the number of events (should match expected iterations)
			if len(events) != tt.expectedIterations {
				t.Errorf("Expected %d events, got %d", tt.expectedIterations, len(events))
			}

			// Check if escalation was handled correctly
			if tt.shouldEscalate {
				// Last event should have escalation flag set
				if len(events) > 0 {
					lastEvent := events[len(events)-1]
					if lastEvent.Actions.Escalate == nil || !*lastEvent.Actions.Escalate {
						t.Error("Expected last event to have escalation flag set")
					}
				}
			}

			// Verify iteration counts in child agents
			switch child := tt.childAgent.(type) {
			case *TestEscalatingAgent:
				if child.runCount != tt.expectedIterations {
					t.Errorf("Expected escalating agent to run %d times, ran %d times",
						tt.expectedIterations, child.runCount)
				}
			case *TestRegularAgent:
				if child.runCount != tt.expectedIterations {
					t.Errorf("Expected regular agent to run %d times, ran %d times",
						tt.expectedIterations, child.runCount)
				}
			}
		})
	}
}

func TestCreateEscalationEvent(t *testing.T) {
	// Test the escalation event helper function
	author := "TestAgent"
	invocationID := "test-invocation-123"
	content := &core.Content{
		Role: "assistant",
		Parts: []core.Part{core.TextPart{
			Text: "Cannot complete task, escalating",
		}},
	}

	event := CreateEscalationEvent(author, invocationID, content)

	// Verify event properties
	if event.Author != author {
		t.Errorf("Expected author %s, got %s", author, event.Author)
	}

	if event.InvocationID != invocationID {
		t.Errorf("Expected invocationID %s, got %s", invocationID, event.InvocationID)
	}

	if event.Actions.Escalate == nil || !*event.Actions.Escalate {
		t.Error("Expected escalation flag to be set to true")
	}

	if event.Content != content {
		t.Error("Expected content to match provided content")
	}

	// Verify that the event has proper metadata
	if event.ID == "" {
		t.Error("Expected event to have generated ID")
	}

	if event.Timestamp == 0 {
		t.Error("Expected event to have generated timestamp")
	}
}
