package viz

import (
	"sync"
	"time"
)

// GraphStateTracker maintains the current visualization state of a graph execution.
// It tracks node statuses, edge traversals, and execution paths for real-time visualization.
type GraphStateTracker struct {
	mu sync.RWMutex

	runID     string
	superstep int

	// Node tracking
	nodeStates     map[string]NodeStatus
	activeNodes    map[string]bool
	completedNodes map[string]bool
	pausedNodes    map[string]bool
	errorNodes     map[string]bool

	// Edge tracking
	edges          []EdgeTraversal
	maxEdgeHistory int // Maximum edges to keep in history

	// Execution path
	executionPath []string
	currentNode   string

	// State metadata
	stateKeys map[string]bool
	stateSize int
}

// NewGraphStateTracker creates a new graph state tracker.
func NewGraphStateTracker(runID string) *GraphStateTracker {
	return &GraphStateTracker{
		runID:          runID,
		nodeStates:     make(map[string]NodeStatus),
		activeNodes:    make(map[string]bool),
		completedNodes: make(map[string]bool),
		pausedNodes:    make(map[string]bool),
		errorNodes:     make(map[string]bool),
		edges:          make([]EdgeTraversal, 0),
		maxEdgeHistory: 100, // Keep last 100 edges
		executionPath:  make([]string, 0),
		stateKeys:      make(map[string]bool),
	}
}

// UpdateNodeStatus updates the status of a node and returns a state update.
func (t *GraphStateTracker) UpdateNodeStatus(node string, status NodeStatus, superstep int) *GraphStateUpdate {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Update node state
	oldStatus := t.nodeStates[node]
	t.nodeStates[node] = status
	t.superstep = superstep

	// Update tracking maps
	t.updateTrackingMaps(node, oldStatus, status)

	// Determine update type
	updateType := t.getUpdateType(status)

	return &GraphStateUpdate{
		RunID:      t.runID,
		Timestamp:  time.Now(),
		Superstep:  superstep,
		UpdateType: updateType,
		Node:       node,
		NodeStatus: status,
	}
}

// updateTrackingMaps updates the internal tracking maps based on status change.
func (t *GraphStateTracker) updateTrackingMaps(node string, oldStatus, newStatus NodeStatus) {
	// Remove from old status maps
	switch oldStatus {
	case NodeStatusActive:
		delete(t.activeNodes, node)
	case NodeStatusCompleted:
		delete(t.completedNodes, node)
	case NodeStatusPaused:
		delete(t.pausedNodes, node)
	case NodeStatusError:
		delete(t.errorNodes, node)
	}

	// Add to new status maps
	switch newStatus {
	case NodeStatusActive:
		t.activeNodes[node] = true
		t.currentNode = node
		t.executionPath = append(t.executionPath, node)
	case NodeStatusCompleted:
		t.completedNodes[node] = true
	case NodeStatusPaused:
		t.pausedNodes[node] = true
	case NodeStatusError:
		t.errorNodes[node] = true
	}
}

// getUpdateType returns the update type string based on node status.
func (t *GraphStateTracker) getUpdateType(status NodeStatus) string {
	switch status {
	case NodeStatusQueued:
		return "node_queued"
	case NodeStatusActive:
		return "node_activated"
	case NodeStatusCompleted:
		return "node_completed"
	case NodeStatusPaused:
		return "node_paused"
	case NodeStatusError:
		return "node_error"
	case NodeStatusSkipped:
		return "node_skipped"
	default:
		return "node_updated"
	}
}

// AddEdgeTraversal records an edge traversal and returns a state update.
func (t *GraphStateTracker) AddEdgeTraversal(from, to string, superstep int) *GraphStateUpdate {
	t.mu.Lock()
	defer t.mu.Unlock()

	edge := EdgeTraversal{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Superstep: superstep,
	}

	// Add edge to history
	t.edges = append(t.edges, edge)

	// Trim history if needed
	if len(t.edges) > t.maxEdgeHistory {
		t.edges = t.edges[len(t.edges)-t.maxEdgeHistory:]
	}

	t.superstep = superstep

	return &GraphStateUpdate{
		RunID:      t.runID,
		Timestamp:  time.Now(),
		Superstep:  superstep,
		UpdateType: "edge_traversed",
		Edge:       &edge,
	}
}

// UpdateState records state changes.
func (t *GraphStateTracker) UpdateState(keys []string, sizeBytes int) *GraphStateUpdate {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Update state keys
	changedKeys := make([]string, 0)
	for _, key := range keys {
		if !t.stateKeys[key] {
			changedKeys = append(changedKeys, key)
			t.stateKeys[key] = true
		}
	}

	t.stateSize = sizeBytes

	if len(changedKeys) == 0 {
		return nil // No new keys
	}

	return &GraphStateUpdate{
		RunID:        t.runID,
		Timestamp:    time.Now(),
		Superstep:    t.superstep,
		UpdateType:   "state_changed",
		StateChanged: changedKeys,
	}
}

// GetSnapshot returns the current graph state snapshot.
func (t *GraphStateTracker) GetSnapshot() GraphSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Convert maps to slices
	activeNodes := make([]string, 0, len(t.activeNodes))
	for node := range t.activeNodes {
		activeNodes = append(activeNodes, node)
	}

	completedNodes := make([]string, 0, len(t.completedNodes))
	for node := range t.completedNodes {
		completedNodes = append(completedNodes, node)
	}

	pausedNodes := make([]string, 0, len(t.pausedNodes))
	for node := range t.pausedNodes {
		pausedNodes = append(pausedNodes, node)
	}

	errorNodes := make([]string, 0, len(t.errorNodes))
	for node := range t.errorNodes {
		errorNodes = append(errorNodes, node)
	}

	stateKeys := make([]string, 0, len(t.stateKeys))
	for key := range t.stateKeys {
		stateKeys = append(stateKeys, key)
	}

	// Get recent edges (last 20)
	recentEdges := t.edges
	if len(recentEdges) > 20 {
		recentEdges = recentEdges[len(recentEdges)-20:]
	}

	// Get active edges (from active nodes)
	activeEdges := make([]EdgeTraversal, 0)
	for _, edge := range t.edges {
		if t.activeNodes[edge.From] || t.activeNodes[edge.To] {
			activeEdges = append(activeEdges, edge)
		}
	}

	// Copy execution path
	executionPath := make([]string, len(t.executionPath))
	copy(executionPath, t.executionPath)

	return GraphSnapshot{
		RunID:          t.runID,
		Superstep:      t.superstep,
		Timestamp:      time.Now(),
		NodeStates:     t.copyNodeStates(),
		ActiveNodes:    activeNodes,
		CompletedNodes: completedNodes,
		PausedNodes:    pausedNodes,
		ErrorNodes:     errorNodes,
		RecentEdges:    recentEdges,
		ActiveEdges:    activeEdges,
		CurrentNode:    t.currentNode,
		ExecutionPath:  executionPath,
		StateKeys:      stateKeys,
		StateSize:      t.stateSize,
	}
}

// copyNodeStates creates a copy of the node states map.
func (t *GraphStateTracker) copyNodeStates() map[string]NodeStatus {
	states := make(map[string]NodeStatus, len(t.nodeStates))
	for node, status := range t.nodeStates {
		states[node] = status
	}
	return states
}

// Reset clears the tracker state.
func (t *GraphStateTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.superstep = 0
	t.nodeStates = make(map[string]NodeStatus)
	t.activeNodes = make(map[string]bool)
	t.completedNodes = make(map[string]bool)
	t.pausedNodes = make(map[string]bool)
	t.errorNodes = make(map[string]bool)
	t.edges = make([]EdgeTraversal, 0)
	t.executionPath = make([]string, 0)
	t.currentNode = ""
	t.stateKeys = make(map[string]bool)
	t.stateSize = 0
}
