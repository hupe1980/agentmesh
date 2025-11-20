package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
)

// TestPendingWritesStructure verifies the PendingWrite structure for checkpoint enhancements
func TestPendingWritesStructure(t *testing.T) {
	now := time.Now()
	chkpt := &checkpoint.Checkpoint{
		RunID:     "test-run-001",
		Superstep: 1,
		Timestamp: now,
		State: map[string]any{
			"channel1": "value1",
		},
		PendingWrites: []checkpoint.PendingWrite{
			{
				NodeName:  "node1",
				Channel:   "output",
				Value:     "pending_value",
				Timestamp: now,
			},
		},
	}

	assert.Len(t, chkpt.PendingWrites, 1)
	assert.Equal(t, "node1", chkpt.PendingWrites[0].NodeName)
}

// TestResumeValueContext verifies resume values context handling
func TestResumeValueContext(t *testing.T) {
	ctx := context.Background()
	resumeVal := graph.ResumeValueFromContext(ctx)
	assert.Nil(t, resumeVal)
}

// TestInterruptConfiguration verifies interrupt configuration
func TestInterruptConfiguration(t *testing.T) {
	manager := graphstate.NewManager()
	g, err := graph.NewGraph(manager)
	assert.NoError(t, err)

	node := graph.NewBaseNode("test", func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
		return graphstate.Updates{}, nil
	})

	g.AddNode(node)
	g.AddInterruptBefore("test")

	assert.Contains(t, g.InterruptBefore, "test")
}
