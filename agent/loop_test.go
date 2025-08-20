package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEscalatingAgent is a mock agent that escalates after a certain number of runs
type TestEscalatingAgent struct {
	BaseAgent
	runCount   int
	escalateOn int
}

func NewTestEscalatingAgent(name string, escalateOn int) *TestEscalatingAgent {
	return &TestEscalatingAgent{
		BaseAgent:  NewBaseAgent(name),
		escalateOn: escalateOn,
		runCount:   0,
	}
}

func (t *TestEscalatingAgent) Run(runCtx *core.RunContext) error {
	t.runCount++

	var ev core.Event

	if t.runCount >= t.escalateOn {
		// Create escalation event
		ev = core.NewEscalationEvent(runCtx.RunID, t.Name(), core.Content{
			Role: "assistant",
			Parts: []core.Part{core.TextPart{
				Text: "Task complexity exceeds my capabilities, escalating to parent",
			}},
		})
	} else {
		// Regular event
		ev = core.NewAssistantEvent(runCtx.RunID, t.Name(), core.Content{
			Role: "assistant",
			Parts: []core.Part{core.TextPart{
				Text: "Working on task iteration " + string(rune(t.runCount+'0')),
			}},
		}, false)
	}

	// Emit the event
	if err := runCtx.EmitEvent(ev); err != nil {
		return err
	}

	// Wait for resume
	return runCtx.WaitForResume()
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

func (t *TestRegularAgent) Run(runCtx *core.RunContext) error {
	t.runCount++

	ev := core.NewAssistantEvent(runCtx.RunID, t.Name(), core.Content{
		Role:  "assistant",
		Parts: []core.Part{core.TextPart{Text: "Working on task iteration " + string(rune(t.runCount+'0'))}},
	}, false)

	// Emit the event
	if err := runCtx.EmitEvent(ev); err != nil {
		return err
	}

	return runCtx.WaitForResume()
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
		tt := tt

		// Use a subtest to isolate each case
		t.Run(tt.name, func(t *testing.T) {
			loopAgent := NewLoopAgent("TestLoop", tt.childAgent, func(o *LoopAgentOptions) { o.MaxIters = tt.maxIters })

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			emitChan := make(chan core.Event, 10)
			resumeChan := make(chan struct{}, 10)

			runCtx := &core.RunContext{
				Context: ctx,
				RunID:   "test-invocation",
				Emit:    emitChan,
				Resume:  resumeChan,
			}

			var (
				events []core.Event
				wg     sync.WaitGroup
			)

			wg.Add(1)

			go func() {
				defer wg.Done()
				for ev := range emitChan {
					events = append(events, ev)
					select {
					case resumeChan <- struct{}{}:
					case <-ctx.Done():
						return
					}
				}
			}()

			err := loopAgent.Run(runCtx)

			close(emitChan)
			wg.Wait()
			close(resumeChan)

			require.NoError(t, err, "unexpected loop run error")
			assert.Len(t, events, tt.expectedIterations, "iteration count mismatch")

			if tt.shouldEscalate && len(events) > 0 {
				last := events[len(events)-1]
				require.NotNil(t, last.Actions.Escalate, "expected escalation flag present")
				assert.True(t, *last.Actions.Escalate, "escalation flag should be true")
			}

			switch child := tt.childAgent.(type) {
			case *TestEscalatingAgent:
				assert.Equal(t, tt.expectedIterations, child.runCount, "escalating agent run count")
			case *TestRegularAgent:
				assert.Equal(t, tt.expectedIterations, child.runCount, "regular agent run count")
			}
		})
	}
}

func TestCreateEscalationEvent(t *testing.T) {
	// Test the escalation event helper function
	author := "TestAgent"
	runID := "test-invocation-123"
	content := core.Content{
		Role: "assistant",
		Parts: []core.Part{core.TextPart{
			Text: "Cannot complete task, escalating",
		}},
	}

	event := core.NewEscalationEvent(runID, author, content)

	// Verify event properties
	assert.Equal(t, author, event.Author, "author mismatch")
	assert.Equal(t, runID, event.RunID, "runID mismatch")
	require.NotNil(t, event.Actions.Escalate, "escalate flag pointer should be set")
	assert.True(t, *event.Actions.Escalate, "escalation flag should be true")

	require.NotNil(t, event.Content)
	assert.Equal(t, content, *event.Content, "content mismatch")

	// Verify that the event has proper metadata
	assert.NotEmpty(t, event.ID, "expected generated ID")
	assert.False(t, event.Timestamp.IsZero(), "expected non-zero timestamp")
}
