package graph_test

import (
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
)

func TestNewRuntimeMetrics(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	assert.NotNil(t, metrics)
	snapshot := metrics.Snapshot()
	assert.Equal(t, int64(0), snapshot.CurrentSuperstep)
	assert.Empty(t, snapshot.CompletedNodes)
	assert.Empty(t, snapshot.PausedNodes)
	assert.Empty(t, snapshot.ResumingNodes)
	assert.Empty(t, snapshot.ActiveNodes)
	assert.Empty(t, snapshot.FailedNodes)
	assert.Equal(t, int64(0), snapshot.TotalMessages)
	assert.Equal(t, int64(0), snapshot.ExecutionTimeNs)
}

func TestRuntimeMetrics_SetSuperstep(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	metrics.SetSuperstep(5)
	snapshot := metrics.Snapshot()
	assert.Equal(t, int64(5), snapshot.CurrentSuperstep)

	metrics.SetSuperstep(10)
	snapshot = metrics.Snapshot()
	assert.Equal(t, int64(10), snapshot.CurrentSuperstep)
}

func TestRuntimeMetrics_AddCompleted(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	// Add node as active first
	metrics.AddActive("node1")
	assert.Contains(t, metrics.Snapshot().ActiveNodes, "node1")

	// Mark as completed - should remove from active
	metrics.AddCompleted("node1")
	snapshot := metrics.Snapshot()
	assert.Contains(t, snapshot.CompletedNodes, "node1")
	assert.NotContains(t, snapshot.ActiveNodes, "node1")

	// Add multiple completed nodes
	metrics.AddCompleted("node2")
	metrics.AddCompleted("node3")
	snapshot = metrics.Snapshot()
	assert.Len(t, snapshot.CompletedNodes, 3)
	assert.Contains(t, snapshot.CompletedNodes, "node2")
	assert.Contains(t, snapshot.CompletedNodes, "node3")
}

func TestRuntimeMetrics_AddPaused(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	// Add node as active first
	metrics.AddActive("node1")

	// Pause the node
	metrics.AddPaused("node1")
	snapshot := metrics.Snapshot()
	assert.Contains(t, snapshot.PausedNodes, "node1")
	assert.NotContains(t, snapshot.ActiveNodes, "node1")

	// Adding same node again should not duplicate
	metrics.AddPaused("node1")
	snapshot = metrics.Snapshot()
	assert.Len(t, snapshot.PausedNodes, 1)
}

func TestRuntimeMetrics_ResumePaused(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	// Pause a node
	metrics.AddPaused("node1")
	assert.Contains(t, metrics.Snapshot().PausedNodes, "node1")

	// Resume the node
	metrics.ResumePaused("node1")
	snapshot := metrics.Snapshot()
	assert.NotContains(t, snapshot.PausedNodes, "node1")
	assert.Contains(t, snapshot.ResumingNodes, "node1")

	// Resuming same node again should not duplicate
	metrics.ResumePaused("node1")
	snapshot = metrics.Snapshot()
	assert.Len(t, snapshot.ResumingNodes, 1)
}

func TestRuntimeMetrics_ClearResuming(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	// Add to resuming
	metrics.ResumePaused("node1")
	assert.Contains(t, metrics.Snapshot().ResumingNodes, "node1")

	// Clear resuming
	metrics.ClearResuming("node1")
	assert.NotContains(t, metrics.Snapshot().ResumingNodes, "node1")
}

func TestRuntimeMetrics_AddActive(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	metrics.AddActive("node1")
	snapshot := metrics.Snapshot()
	assert.Contains(t, snapshot.ActiveNodes, "node1")

	// Adding same node again should not duplicate
	metrics.AddActive("node1")
	snapshot = metrics.Snapshot()
	assert.Len(t, snapshot.ActiveNodes, 1)

	// Add multiple active nodes
	metrics.AddActive("node2")
	metrics.AddActive("node3")
	snapshot = metrics.Snapshot()
	assert.Len(t, snapshot.ActiveNodes, 3)
}

func TestRuntimeMetrics_AddFailed(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	// Add node as active first
	metrics.AddActive("node1")

	// Mark as failed - should remove from active
	metrics.AddFailed("node1")
	snapshot := metrics.Snapshot()
	assert.Contains(t, snapshot.FailedNodes, "node1")
	assert.NotContains(t, snapshot.ActiveNodes, "node1")

	// Adding same node again should not duplicate
	metrics.AddFailed("node1")
	snapshot = metrics.Snapshot()
	assert.Len(t, snapshot.FailedNodes, 1)
}

func TestRuntimeMetrics_IncrementMessages(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	metrics.IncrementMessages(5)
	assert.Equal(t, int64(5), metrics.Snapshot().TotalMessages)

	metrics.IncrementMessages(10)
	assert.Equal(t, int64(15), metrics.Snapshot().TotalMessages)

	metrics.IncrementMessages(1)
	assert.Equal(t, int64(16), metrics.Snapshot().TotalMessages)
}

func TestRuntimeMetrics_AddExecutionTime(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	metrics.AddExecutionTime(1000000)
	assert.Equal(t, int64(1000000), metrics.Snapshot().ExecutionTimeNs)

	metrics.AddExecutionTime(2000000)
	assert.Equal(t, int64(3000000), metrics.Snapshot().ExecutionTimeNs)
}

func TestRuntimeMetrics_Reset(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	// Populate metrics
	metrics.SetSuperstep(10)
	metrics.AddCompleted("node1")
	metrics.AddPaused("node2")
	metrics.AddActive("node3")
	metrics.AddFailed("node4")
	metrics.IncrementMessages(100)
	metrics.AddExecutionTime(5000000)

	// Verify populated
	snapshot := metrics.Snapshot()
	assert.Equal(t, int64(10), snapshot.CurrentSuperstep)
	assert.Len(t, snapshot.CompletedNodes, 1)
	assert.Len(t, snapshot.PausedNodes, 1)
	assert.Len(t, snapshot.ActiveNodes, 1)
	assert.Len(t, snapshot.FailedNodes, 1)
	assert.Equal(t, int64(100), snapshot.TotalMessages)
	assert.Equal(t, int64(5000000), snapshot.ExecutionTimeNs)

	// Reset
	metrics.Reset()

	// Verify reset
	snapshot = metrics.Snapshot()
	assert.Equal(t, int64(0), snapshot.CurrentSuperstep)
	assert.Empty(t, snapshot.CompletedNodes)
	assert.Empty(t, snapshot.PausedNodes)
	assert.Empty(t, snapshot.ActiveNodes)
	assert.Empty(t, snapshot.FailedNodes)
	assert.Equal(t, int64(0), snapshot.TotalMessages)
	assert.Equal(t, int64(0), snapshot.ExecutionTimeNs)
}

func TestRuntimeMetrics_Snapshot_ImmutableCopy(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	metrics.AddCompleted("node1")
	snapshot1 := metrics.Snapshot()

	// Modify original
	metrics.AddCompleted("node2")

	// Original snapshot should be unchanged
	assert.Len(t, snapshot1.CompletedNodes, 1)
	assert.Contains(t, snapshot1.CompletedNodes, "node1")

	// New snapshot should have both
	snapshot2 := metrics.Snapshot()
	assert.Len(t, snapshot2.CompletedNodes, 2)
}

func TestRuntimeMetrics_ConcurrentAccess(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			nodeName := "node" + string(rune('0'+n%10))
			metrics.AddActive(nodeName)
			metrics.AddCompleted(nodeName)
		}(i)

		wg.Add(1)
		go func() {
			defer wg.Done()
			metrics.IncrementMessages(1)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			metrics.AddExecutionTime(1000)
		}()
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = metrics.Snapshot()
		}()
	}

	wg.Wait()

	// Verify final state is consistent
	snapshot := metrics.Snapshot()
	assert.Equal(t, int64(100), snapshot.TotalMessages)
	assert.Equal(t, int64(100000), snapshot.ExecutionTimeNs)
}

func TestRuntimeMetrics_NodeLifecycle(t *testing.T) {
	// Test complete lifecycle of a node through different states
	metrics := graph.NewRuntimeMetrics()

	// 1. Node becomes active
	metrics.AddActive("worker")
	snapshot := metrics.Snapshot()
	assert.Contains(t, snapshot.ActiveNodes, "worker")
	assert.Len(t, snapshot.ActiveNodes, 1)

	// 2. Node gets paused (removed from active)
	metrics.AddPaused("worker")
	snapshot = metrics.Snapshot()
	assert.Contains(t, snapshot.PausedNodes, "worker")
	assert.NotContains(t, snapshot.ActiveNodes, "worker")

	// 3. Node resumes (removed from paused, added to resuming)
	metrics.ResumePaused("worker")
	snapshot = metrics.Snapshot()
	assert.NotContains(t, snapshot.PausedNodes, "worker")
	assert.Contains(t, snapshot.ResumingNodes, "worker")

	// 4. Node starts executing again (clear resuming, add active)
	metrics.ClearResuming("worker")
	metrics.AddActive("worker")
	snapshot = metrics.Snapshot()
	assert.NotContains(t, snapshot.ResumingNodes, "worker")
	assert.Contains(t, snapshot.ActiveNodes, "worker")

	// 5. Node completes (removed from active, added to completed)
	metrics.AddCompleted("worker")
	snapshot = metrics.Snapshot()
	assert.Contains(t, snapshot.CompletedNodes, "worker")
	assert.NotContains(t, snapshot.ActiveNodes, "worker")
}

func TestRuntimeMetrics_FailedNodeLifecycle(t *testing.T) {
	// Test node that fails during execution
	metrics := graph.NewRuntimeMetrics()

	// Node becomes active
	metrics.AddActive("failing_node")
	assert.Contains(t, metrics.Snapshot().ActiveNodes, "failing_node")

	// Node fails (removed from active, added to failed)
	metrics.AddFailed("failing_node")
	snapshot := metrics.Snapshot()
	assert.Contains(t, snapshot.FailedNodes, "failing_node")
	assert.NotContains(t, snapshot.ActiveNodes, "failing_node")
}

func TestRuntimeMetrics_MultipleSupersteps(t *testing.T) {
	metrics := graph.NewRuntimeMetrics()

	// Superstep 0
	metrics.SetSuperstep(0)
	metrics.AddCompleted("node1")

	// Superstep 1
	metrics.SetSuperstep(1)
	metrics.AddCompleted("node2")

	// Superstep 2
	metrics.SetSuperstep(2)
	metrics.AddCompleted("node3")

	snapshot := metrics.Snapshot()
	assert.Equal(t, int64(2), snapshot.CurrentSuperstep)
	assert.Len(t, snapshot.CompletedNodes, 3)
}
