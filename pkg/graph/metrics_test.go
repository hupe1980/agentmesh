package graph

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRuntimeMetrics(t *testing.T) {
	rm := NewRuntimeMetrics()
	require.NotNil(t, rm)
	assert.Equal(t, int64(0), rm.CurrentSuperstep)
	assert.Empty(t, rm.CompletedNodes)
	assert.Empty(t, rm.PausedNodes)
	assert.Empty(t, rm.ResumingNodes)
	assert.Empty(t, rm.ActiveNodes)
	assert.Empty(t, rm.FailedNodes)
	assert.Equal(t, int64(0), rm.TotalMessages)
	assert.Equal(t, int64(0), rm.ExecutionTimeNs)
}

func TestSuperstep(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.SetSuperstep(5)
	assert.Equal(t, int64(5), rm.GetSuperstep())

	rm.SetSuperstep(10)
	assert.Equal(t, int64(10), rm.GetSuperstep())
}

func TestAddCompleted(t *testing.T) {
	rm := NewRuntimeMetrics()

	// Add to active first
	rm.AddActive("node1")
	assert.Contains(t, rm.ActiveNodes, "node1")

	// Mark as completed
	rm.AddCompleted("node1")
	assert.Contains(t, rm.CompletedNodes, "node1")
	assert.NotContains(t, rm.ActiveNodes, "node1")
}

func TestAddPaused(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.AddActive("node1")
	rm.AddPaused("node1")

	assert.Contains(t, rm.PausedNodes, "node1")
	assert.NotContains(t, rm.ActiveNodes, "node1")

	// Adding same node again should not duplicate
	rm.AddPaused("node1")
	count := 0
	for _, n := range rm.PausedNodes {
		if n == "node1" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestResumePaused(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.AddPaused("node1")
	assert.Contains(t, rm.PausedNodes, "node1")

	rm.ResumePaused("node1")
	assert.NotContains(t, rm.PausedNodes, "node1")
	assert.Contains(t, rm.ResumingNodes, "node1")

	// Should not duplicate in resuming
	rm.ResumePaused("node1")
	count := 0
	for _, n := range rm.ResumingNodes {
		if n == "node1" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestClearResuming(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.ResumePaused("node1")
	assert.Contains(t, rm.ResumingNodes, "node1")

	rm.ClearResuming("node1")
	assert.NotContains(t, rm.ResumingNodes, "node1")
}

func TestIsResuming(t *testing.T) {
	rm := NewRuntimeMetrics()

	assert.False(t, rm.IsResuming("node1"))

	rm.ResumePaused("node1")
	assert.True(t, rm.IsResuming("node1"))

	rm.ClearResuming("node1")
	assert.False(t, rm.IsResuming("node1"))
}

func TestAddActive(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.AddActive("node1")
	assert.Contains(t, rm.ActiveNodes, "node1")

	// Should not duplicate
	rm.AddActive("node1")
	count := 0
	for _, n := range rm.ActiveNodes {
		if n == "node1" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestAddFailed(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.AddActive("node1")
	rm.AddFailed("node1")

	assert.Contains(t, rm.FailedNodes, "node1")
	assert.NotContains(t, rm.ActiveNodes, "node1")

	// Should not duplicate
	rm.AddFailed("node1")
	count := 0
	for _, n := range rm.FailedNodes {
		if n == "node1" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestAddMessage(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.AddMessage()
	assert.Equal(t, int64(1), rm.TotalMessages)

	rm.AddMessage()
	assert.Equal(t, int64(2), rm.TotalMessages)
}

func TestAddMessages(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.AddMessages(5)
	assert.Equal(t, int64(5), rm.TotalMessages)

	rm.AddMessages(3)
	assert.Equal(t, int64(8), rm.TotalMessages)
}

func TestSetExecutionTime(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.SetExecutionTime(1000000)
	assert.Equal(t, int64(1000000), rm.ExecutionTimeNs)
}

func TestSnapshot(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.SetSuperstep(3)
	rm.AddCompleted("node1")
	rm.AddPaused("node2")
	rm.ResumePaused("node3")
	rm.AddActive("node4")
	rm.AddFailed("node5")
	rm.AddMessages(10)
	rm.SetExecutionTime(5000)

	snap := rm.Snapshot()

	assert.Equal(t, int64(3), snap.CurrentSuperstep)
	assert.Contains(t, snap.CompletedNodes, "node1")
	assert.Contains(t, snap.PausedNodes, "node2")
	assert.Contains(t, snap.ResumingNodes, "node3")
	assert.Contains(t, snap.ActiveNodes, "node4")
	assert.Contains(t, snap.FailedNodes, "node5")
	assert.Equal(t, int64(10), snap.TotalMessages)
	assert.Equal(t, int64(5000), snap.ExecutionTimeNs)

	// Verify snapshot is a copy (modifying original doesn't affect snapshot)
	rm.AddCompleted("node6")
	assert.NotContains(t, snap.CompletedNodes, "node6")
}

func TestReset(t *testing.T) {
	rm := NewRuntimeMetrics()

	rm.SetSuperstep(5)
	rm.AddCompleted("node1")
	rm.AddPaused("node2")
	rm.AddActive("node3")
	rm.AddFailed("node4")
	rm.AddMessages(10)
	rm.SetExecutionTime(5000)

	rm.Reset()

	assert.Equal(t, int64(0), rm.CurrentSuperstep)
	assert.Empty(t, rm.CompletedNodes)
	assert.Empty(t, rm.PausedNodes)
	assert.Empty(t, rm.ResumingNodes)
	assert.Empty(t, rm.ActiveNodes)
	assert.Empty(t, rm.FailedNodes)
	assert.Equal(t, int64(0), rm.TotalMessages)
	assert.Equal(t, int64(0), rm.ExecutionTimeNs)
}

func TestRuntimeMetricsConcurrentAccess(t *testing.T) {
	rm := NewRuntimeMetrics()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rm.AddMessage()
			rm.SetSuperstep(int64(i))
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rm.GetSuperstep()
			_ = rm.Snapshot()
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(numGoroutines), rm.TotalMessages)
}

func TestRuntimeMetricsConcurrentNodeOperations(t *testing.T) {
	rm := NewRuntimeMetrics()

	var wg sync.WaitGroup

	// Simulate concurrent node state changes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			nodeName := "node" + string(rune('A'+i%26))
			rm.AddActive(nodeName)
			rm.AddCompleted(nodeName)
		}(i)
	}

	wg.Wait()

	// All nodes should be in completed, none in active
	snap := rm.Snapshot()
	assert.Empty(t, snap.ActiveNodes)
	assert.NotEmpty(t, snap.CompletedNodes)
}
