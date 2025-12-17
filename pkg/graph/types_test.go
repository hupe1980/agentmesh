package graph_test

import (
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
)

func TestApprovalDecisionConstants(t *testing.T) {
	assert.Equal(t, graph.ApprovalDecision("approved"), graph.ApprovalApproved)
	assert.Equal(t, graph.ApprovalDecision("rejected"), graph.ApprovalRejected)
}

func TestApprovalResponse(t *testing.T) {
	now := time.Now()

	response := graph.ApprovalResponse{
		Decision:  graph.ApprovalApproved,
		Reason:    "Looks good",
		User:      "admin",
		Timestamp: now,
		Edits: graph.Updates{
			"modified": true,
		},
		Annotations: map[string]any{
			"priority": "high",
		},
	}

	assert.Equal(t, graph.ApprovalApproved, response.Decision)
	assert.Equal(t, "Looks good", response.Reason)
	assert.Equal(t, "admin", response.User)
	assert.Equal(t, now, response.Timestamp)
	assert.Equal(t, true, response.Edits["modified"])
	assert.Equal(t, "high", response.Annotations["priority"])
}

func TestApprovalResponseRejected(t *testing.T) {
	response := graph.ApprovalResponse{
		Decision: graph.ApprovalRejected,
		Reason:   "Not authorized",
	}

	assert.Equal(t, graph.ApprovalRejected, response.Decision)
	assert.Equal(t, "Not authorized", response.Reason)
}

func TestInterruptOptions(t *testing.T) {
	t.Run("with feedback annotation", func(t *testing.T) {
		opt := graph.WithFeedbackAnnotation(true)
		assert.NotNil(t, opt)
	})
}

func TestExecutorNode(t *testing.T) {
	node := graph.ExecutorNode{
		Name:    "processor",
		Fn:      nil, // NodeFunc would be set in real usage
		Targets: []string{"next", "error"},
	}

	assert.Equal(t, "processor", node.Name)
	assert.Equal(t, []string{"next", "error"}, node.Targets)
}
