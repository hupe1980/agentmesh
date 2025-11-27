package viz

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphStateTracker_NewTracker(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	assert.NotNil(t, tracker)
	assert.Equal(t, "test-run", tracker.runID)
	assert.Equal(t, 0, tracker.superstep)
	assert.NotNil(t, tracker.nodeStates)
	assert.NotNil(t, tracker.activeNodes)
}

func TestGraphStateTracker_UpdateNodeStatus(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	// Update to active
	update := tracker.UpdateNodeStatus("node1", NodeStatusActive, 1)
	require.NotNil(t, update)
	assert.Equal(t, "node_activated", update.UpdateType)
	assert.Equal(t, "node1", update.Node)
	assert.Equal(t, NodeStatusActive, update.NodeStatus)
	assert.Equal(t, 1, update.Superstep)

	// Verify internal state
	snapshot := tracker.GetSnapshot()
	assert.Equal(t, NodeStatusActive, snapshot.NodeStates["node1"])
	assert.Contains(t, snapshot.ActiveNodes, "node1")
	assert.Equal(t, "node1", snapshot.CurrentNode)

	// Update to completed
	update = tracker.UpdateNodeStatus("node1", NodeStatusCompleted, 2)
	require.NotNil(t, update)
	assert.Equal(t, "node_completed", update.UpdateType)

	// Verify state transition
	snapshot = tracker.GetSnapshot()
	assert.Equal(t, NodeStatusCompleted, snapshot.NodeStates["node1"])
	assert.Contains(t, snapshot.CompletedNodes, "node1")
	assert.NotContains(t, snapshot.ActiveNodes, "node1")
}

func TestGraphStateTracker_UpdateNodeStatus_AllStates(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	tests := []struct {
		status         NodeStatus
		expectedUpdate string
		checkList      func(*GraphSnapshot) bool
	}{
		{NodeStatusQueued, "node_queued", func(s *GraphSnapshot) bool { return s.NodeStates["node1"] == NodeStatusQueued }},
		{NodeStatusActive, "node_activated", func(s *GraphSnapshot) bool { return len(s.ActiveNodes) > 0 }},
		{NodeStatusCompleted, "node_completed", func(s *GraphSnapshot) bool { return len(s.CompletedNodes) > 0 }},
		{NodeStatusPaused, "node_paused", func(s *GraphSnapshot) bool { return len(s.PausedNodes) > 0 }},
		{NodeStatusError, "node_error", func(s *GraphSnapshot) bool { return len(s.ErrorNodes) > 0 }},
		{NodeStatusSkipped, "node_skipped", func(s *GraphSnapshot) bool { return s.NodeStates["node1"] == NodeStatusSkipped }},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			tracker.Reset()
			update := tracker.UpdateNodeStatus("node1", tt.status, 1)

			assert.Equal(t, tt.expectedUpdate, update.UpdateType)
			snapshot := tracker.GetSnapshot()
			assert.True(t, tt.checkList(&snapshot))
		})
	}
}

func TestGraphStateTracker_AddEdgeTraversal(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	// Add edge traversal
	update := tracker.AddEdgeTraversal("node1", "node2", 1)
	require.NotNil(t, update)
	assert.Equal(t, "edge_traversed", update.UpdateType)
	assert.NotNil(t, update.Edge)
	assert.Equal(t, "node1", update.Edge.From)
	assert.Equal(t, "node2", update.Edge.To)
	assert.Equal(t, 1, update.Edge.Superstep)

	// Verify in snapshot
	snapshot := tracker.GetSnapshot()
	assert.Len(t, snapshot.RecentEdges, 1)
	assert.Equal(t, "node1", snapshot.RecentEdges[0].From)
	assert.Equal(t, "node2", snapshot.RecentEdges[0].To)
}

func TestGraphStateTracker_EdgeHistory(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")
	tracker.maxEdgeHistory = 10 // Set low for testing

	// Add more edges than the limit
	for i := 0; i < 15; i++ {
		tracker.AddEdgeTraversal("node1", "node2", i)
	}

	// Should only keep last 10
	snapshot := tracker.GetSnapshot()
	assert.LessOrEqual(t, len(snapshot.RecentEdges), 10)

	// Most recent should be superstep 14
	if len(snapshot.RecentEdges) > 0 {
		lastEdge := snapshot.RecentEdges[len(snapshot.RecentEdges)-1]
		assert.Equal(t, 14, lastEdge.Superstep)
	}
}

func TestGraphStateTracker_UpdateState(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	// First update with new keys
	update := tracker.UpdateState([]string{"key1", "key2"}, 1024)
	require.NotNil(t, update)
	assert.Equal(t, "state_changed", update.UpdateType)
	assert.ElementsMatch(t, []string{"key1", "key2"}, update.StateChanged)

	// Second update with same keys (no new keys)
	update = tracker.UpdateState([]string{"key1", "key2"}, 2048)
	assert.Nil(t, update) // No new keys

	// Third update with one new key
	update = tracker.UpdateState([]string{"key1", "key2", "key3"}, 3072)
	require.NotNil(t, update)
	assert.ElementsMatch(t, []string{"key3"}, update.StateChanged)

	// Verify snapshot
	snapshot := tracker.GetSnapshot()
	assert.ElementsMatch(t, []string{"key1", "key2", "key3"}, snapshot.StateKeys)
	assert.Equal(t, 3072, snapshot.StateSize)
}

func TestGraphStateTracker_GetSnapshot(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	// Setup some state - activate nodes to add to execution path
	tracker.UpdateNodeStatus("node1", NodeStatusActive, 1)
	tracker.UpdateNodeStatus("node1", NodeStatusCompleted, 1) // Complete it
	tracker.UpdateNodeStatus("node2", NodeStatusActive, 2)
	tracker.UpdateNodeStatus("node2", NodeStatusCompleted, 2)
	tracker.UpdateNodeStatus("node3", NodeStatusActive, 3)
	tracker.UpdateNodeStatus("node3", NodeStatusPaused, 3)
	tracker.AddEdgeTraversal("node1", "node2", 3) // Use superstep 3
	tracker.AddEdgeTraversal("node2", "node3", 3)
	tracker.UpdateState([]string{"key1", "key2"}, 512)

	// Get snapshot
	snapshot := tracker.GetSnapshot()

	assert.Equal(t, "test-run", snapshot.RunID)
	assert.Equal(t, 3, snapshot.Superstep)
	assert.Len(t, snapshot.NodeStates, 3)
	assert.Contains(t, snapshot.CompletedNodes, "node1")
	assert.Contains(t, snapshot.CompletedNodes, "node2")
	assert.Contains(t, snapshot.PausedNodes, "node3")
	assert.Len(t, snapshot.RecentEdges, 2)
	assert.ElementsMatch(t, []string{"key1", "key2"}, snapshot.StateKeys)
	assert.Equal(t, 512, snapshot.StateSize)
	assert.Equal(t, "node3", snapshot.CurrentNode)
	assert.Len(t, snapshot.ExecutionPath, 3)
}

func TestGraphStateTracker_ExecutionPath(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	// Activate nodes in sequence
	tracker.UpdateNodeStatus("node1", NodeStatusActive, 1)
	tracker.UpdateNodeStatus("node2", NodeStatusActive, 2)
	tracker.UpdateNodeStatus("node3", NodeStatusActive, 3)

	snapshot := tracker.GetSnapshot()
	assert.Equal(t, []string{"node1", "node2", "node3"}, snapshot.ExecutionPath)
	assert.Equal(t, "node3", snapshot.CurrentNode)
}

func TestGraphStateTracker_ActiveEdges(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	// Add edges and set some nodes active
	tracker.AddEdgeTraversal("node1", "node2", 1)
	tracker.AddEdgeTraversal("node2", "node3", 2)
	tracker.AddEdgeTraversal("node3", "node4", 3)

	// Make node2 active
	tracker.UpdateNodeStatus("node2", NodeStatusActive, 2)

	snapshot := tracker.GetSnapshot()

	// Active edges should include edges from/to node2
	hasActiveEdge := false
	for _, edge := range snapshot.ActiveEdges {
		if edge.From == "node2" || edge.To == "node2" {
			hasActiveEdge = true
			break
		}
	}
	assert.True(t, hasActiveEdge)
}

func TestGraphStateTracker_Reset(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")

	// Setup some state
	tracker.UpdateNodeStatus("node1", NodeStatusActive, 1)
	tracker.AddEdgeTraversal("node1", "node2", 1)
	tracker.UpdateState([]string{"key1"}, 100)

	// Reset
	tracker.Reset()

	// Verify everything is cleared
	snapshot := tracker.GetSnapshot()
	assert.Equal(t, 0, snapshot.Superstep)
	assert.Empty(t, snapshot.NodeStates)
	assert.Empty(t, snapshot.ActiveNodes)
	assert.Empty(t, snapshot.CompletedNodes)
	assert.Empty(t, snapshot.RecentEdges)
	assert.Empty(t, snapshot.ExecutionPath)
	assert.Empty(t, snapshot.StateKeys)
	assert.Equal(t, 0, snapshot.StateSize)
	assert.Empty(t, snapshot.CurrentNode)
}

func TestGraphStateTracker_ThreadSafety(t *testing.T) {
	tracker := NewGraphStateTracker("test-run")
	done := make(chan bool)

	// Concurrent updates
	go func() {
		for i := 0; i < 50; i++ {
			tracker.UpdateNodeStatus("node1", NodeStatusActive, i)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			tracker.AddEdgeTraversal("node1", "node2", i)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			tracker.GetSnapshot()
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// If we get here without data races, test passes
	assert.True(t, true)
}

func TestGraphStateUpdate_Fields(t *testing.T) {
	now := time.Now()
	edge := &EdgeTraversal{
		From:      "node1",
		To:        "node2",
		Timestamp: now,
		Superstep: 1,
	}

	update := &GraphStateUpdate{
		RunID:        "test-run",
		Timestamp:    now,
		Superstep:    1,
		UpdateType:   "edge_traversed",
		Node:         "node1",
		NodeStatus:   NodeStatusActive,
		Edge:         edge,
		StateChanged: []string{"key1", "key2"},
	}

	assert.Equal(t, "test-run", update.RunID)
	assert.Equal(t, 1, update.Superstep)
	assert.Equal(t, "edge_traversed", update.UpdateType)
	assert.NotNil(t, update.Edge)
	assert.Equal(t, "node1", update.Edge.From)
}

func TestGraphSnapshot_Fields(t *testing.T) {
	snapshot := GraphSnapshot{
		RunID:          "test-run",
		Superstep:      5,
		Timestamp:      time.Now(),
		NodeStates:     map[string]NodeStatus{"node1": NodeStatusActive},
		ActiveNodes:    []string{"node1"},
		CompletedNodes: []string{"node0"},
		PausedNodes:    []string{},
		ErrorNodes:     []string{},
		RecentEdges:    []EdgeTraversal{},
		ActiveEdges:    []EdgeTraversal{},
		CurrentNode:    "node1",
		ExecutionPath:  []string{"node0", "node1"},
		StateKeys:      []string{"key1"},
		StateSize:      100,
	}

	assert.Equal(t, "test-run", snapshot.RunID)
	assert.Equal(t, 5, snapshot.Superstep)
	assert.Len(t, snapshot.NodeStates, 1)
	assert.Equal(t, "node1", snapshot.CurrentNode)
}
