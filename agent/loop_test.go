package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/core"
	itestutil "github.com/hupe1980/agentmesh/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEscalatingAgent is a mock agent that escalates after a certain number of runs
type TestEscalatingAgent struct {
	*BaseAgent
	runCount   int
	escalateOn int
}

func NewTestEscalatingAgent(name string, escalateOn int) *TestEscalatingAgent {
	a := &TestEscalatingAgent{
		escalateOn: escalateOn,
		runCount:   0,
	}

	a.BaseAgent = NewBaseAgent(a, name, fmt.Sprintf("Agent %s", name))

	return a
}

func (t *TestEscalatingAgent) Run(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
	t.runCount++

	var ev *core.Event

	if t.runCount >= t.escalateOn {
		// Create escalation event
		ev = core.NewFunctionResponseEvent(reqCtx.RunID(), t.Name(), "call-3", "f", "ok")
		ev.Actions.Escalate = core.Bool(true)
	} else {
		// Regular event
		ev = core.NewFullAssistantEvent(
			reqCtx.RunID(),
			t.Name(),
			core.NewPartFromText("Working on task iteration "+string(rune(t.runCount+'0'))),
		)
	}

	if queue != nil {
		_ = queue.Write(ctx, ev)
	}

	return nil
}

// TestRegularAgent is a mock agent that never escalates
type TestRegularAgent struct {
	*BaseAgent
	runCount int
}

func NewTestRegularAgent(name string) *TestRegularAgent {
	a := &TestRegularAgent{
		runCount: 0,
	}

	a.BaseAgent = NewBaseAgent(a, name, fmt.Sprintf("Agent %s", name))

	return a
}

func (t *TestRegularAgent) Run(ctx context.Context, reqCtx core.RequestContext, queue core.EventWriter) error {
	t.runCount++

	ev := core.NewFullAssistantEvent(
		reqCtx.RunID(),
		t.Name(),
		core.NewPartFromText("Working on task iteration "+string(rune(t.runCount+'0'))),
	)

	if queue != nil {
		_ = queue.Write(ctx, ev)
	}

	return nil
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

			reqCtx := core.NewRequestContext(core.RequestContextParams{
				RunID:         "run_id",
				Agent:         core.AgentInfo{},
				UserParts:     nil,
				MaxModelCalls: 100,
				Session:       core.NewSession("app", "user1", "sess1"),
			})

			q := &itestutil.CollectingWriter{}

			err := loopAgent.Run(ctx, reqCtx, q)

			require.NoError(t, err, "unexpected loop run error")
			assert.Len(t, q.Events, tt.expectedIterations, "iteration count mismatch")

			if tt.shouldEscalate && len(q.Events) > 0 {
				last := q.Events[len(q.Events)-1]
				require.NotNil(t, last.Actions.Escalate, "expected escalation flag present")
				assert.True(t, last.Actions.Escalate.Or(false), "escalation flag should be true")
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
