package graph_test

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

// ====================
// State Key Tests
// ====================

func TestNewKey(t *testing.T) {
	key := graph.NewKey[int]("counter")
	if key.Name() != "counter" {
		t.Errorf("expected name=counter, got %v", key.Name())
	}
	// Zero value comes from the reducer (ReplaceReducer by default)
	if key.Zero() != 0 {
		t.Errorf("expected Zero()=0, got %v", key.Zero())
	}
}

func TestNewListKey(t *testing.T) {
	key := graph.NewListKey[string]("test_messages")
	if key.Name() != "test_messages" {
		t.Errorf("expected name=test_messages, got %v", key.Name())
	}
}

// ====================
// Get Tests
// ====================

// createTestView creates a View for testing using BSPState
func createTestView(data map[string]any) graph.ReadOnlyScope {
	return graph.NewBSPState(data, graph.NewKeyRegistry()).ReadView()
}

func TestGet(t *testing.T) {
	key := graph.NewKey[int]("counter")
	view := createTestView(map[string]any{"counter": 42})

	val := graph.Get(view, key)
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

func TestGetDefault(t *testing.T) {
	key := graph.NewKey[int]("counter")
	view := createTestView(map[string]any{})

	val := graph.Get(view, key)
	// With reducer-based design, zero value is returned for unset keys
	if val != 0 {
		t.Errorf("expected zero value 0, got %v", val)
	}
}

func TestGetList(t *testing.T) {
	key := graph.NewListKey[string]("test_messages")
	view := createTestView(map[string]any{"test_messages": []string{"hello", "world"}})

	val := graph.GetList(view, key)
	if len(val) != 2 || val[0] != "hello" || val[1] != "world" {
		t.Errorf("expected [hello, world], got %v", val)
	}
}

func TestGetListEmpty(t *testing.T) {
	key := graph.NewListKey[string]("test_messages")
	view := createTestView(map[string]any{})

	val := graph.GetList(view, key)
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

// ====================
// BSP State Tests
// ====================

func TestBSPStateReadSnapshot(t *testing.T) {
	// Initial state
	initial := map[string]any{"counter": 10, "status": "init"}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Reads should see initial values
	view := bsp.ReadView()
	if v, ok := view.GetValue("counter"); !ok || v.(int) != 10 {
		t.Errorf("expected counter=10, got %v", v)
	}
	if v, ok := view.GetValue("status"); !ok || v.(string) != "init" {
		t.Errorf("expected status=init, got %v", v)
	}
}

func TestBSPStateWriteBuffering(t *testing.T) {
	initial := map[string]any{"counter": 10}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Write should be buffered, not visible in read view
	bsp.Write("node-write-buffer", graph.Updates{"counter": 20, "new_key": "value"})

	// Read view should still show old value (BSP semantics)
	view := bsp.ReadView()
	if v, ok := view.GetValue("counter"); !ok || v.(int) != 10 {
		t.Errorf("expected counter=10 (buffered write not visible), got %v", v)
	}
	if _, ok := view.GetValue("new_key"); ok {
		t.Error("expected new_key to not be visible before barrier")
	}
}

func TestBSPStateBarrierCommit(t *testing.T) {
	initial := map[string]any{"counter": 10}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Write and commit
	bsp.Write("node-barrier", graph.Updates{"counter": 20, "new_key": "value"})
	bsp.CommitBarrier()

	// After barrier, writes should be visible
	view := bsp.ReadView()
	if v, ok := view.GetValue("counter"); !ok || v.(int) != 20 {
		t.Errorf("expected counter=20 after barrier, got %v", v)
	}
	if v, ok := view.GetValue("new_key"); !ok || v.(string) != "value" {
		t.Errorf("expected new_key=value after barrier, got %v", v)
	}
}

func TestBSPStateListMerging(t *testing.T) {
	initial := map[string]any{"items": []string{"a", "b"}}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Multiple writes to same list key should merge
	bsp.Write("node-list", graph.Updates{"items": []string{"c"}})
	bsp.Write("node-list", graph.Updates{"items": []string{"d", "e"}})
	bsp.CommitBarrier()

	view := bsp.ReadView()
	items, ok := view.GetValue("items")
	if !ok {
		t.Fatal("expected items to exist")
	}
	list := items.([]string)
	// Initial: [a, b] + writes: [c] + [d, e] = [a, b, c, d, e]
	if len(list) != 5 {
		t.Errorf("expected 5 items, got %d: %v", len(list), list)
	}
}

func TestBSPStateSnapshot(t *testing.T) {
	initial := map[string]any{"counter": 10}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	bsp.Write("node-snapshot", graph.Updates{"counter": 20})
	bsp.CommitBarrier()

	snapshot := bsp.Snapshot()
	if snapshot["counter"] != 20 {
		t.Errorf("expected snapshot counter=20, got %v", snapshot["counter"])
	}

	// Modifying snapshot should not affect BSP state
	snapshot["counter"] = 999
	if v, _ := bsp.GetCommitted("counter"); v != 20 {
		t.Error("snapshot modification should not affect BSP state")
	}
}

func TestBSPStateConcurrentReads(t *testing.T) {
	// All reads within a superstep should see the same values
	// regardless of when they read (before or after writes from parallel nodes)
	initial := map[string]any{"counter": 10}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Simulate parallel execution: read, then write, then read again
	view1 := bsp.ReadView()
	bsp.Write("node-concurrent", graph.Updates{"counter": 20}) // Another node wrote
	view2 := bsp.ReadView()

	// Both reads should see the same value (BSP guarantee)
	v1, _ := view1.GetValue("counter")
	v2, _ := view2.GetValue("counter")
	if v1 != v2 {
		t.Errorf("BSP violation: reads in same superstep saw different values: %v vs %v", v1, v2)
	}
	if v1 != 10 {
		t.Errorf("expected both reads to see initial value 10, got %v", v1)
	}
}

func TestBSPStatePendingWrites(t *testing.T) {
	initial := map[string]any{"counter": 10}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Before any writes
	if bsp.HasPendingWrites() {
		t.Error("expected no pending writes initially")
	}

	// Write some updates
	bsp.Write("node-pending", graph.Updates{"counter": 20, "new_key": "value"})

	// Should have pending writes
	if !bsp.HasPendingWrites() {
		t.Error("expected pending writes after Write()")
	}

	// Get pending writes
	pending := bsp.PendingWrites()
	if len(pending) != 2 {
		t.Errorf("expected 2 pending writes, got %d", len(pending))
	}
	if pending[0].NodeName == "" || pending[0].Channel == "" {
		t.Error("expected pending writes to include node and channel metadata")
	}
	channels := map[string]any{}
	for _, pw := range pending {
		channels[pw.Channel] = pw.Value
	}
	if channels["counter"] != 20 {
		t.Errorf("expected pending counter=20, got %v", channels["counter"])
	}
	if channels["new_key"] != "value" {
		t.Errorf("expected pending new_key=value, got %v", channels["new_key"])
	}

	// After barrier, no pending writes
	bsp.CommitBarrier()
	if bsp.HasPendingWrites() {
		t.Error("expected no pending writes after barrier")
	}
}

func TestBSPStateApplyPendingWrites(t *testing.T) {
	// Simulates two-phase commit recovery: applying pending writes from checkpoint
	initial := map[string]any{"counter": 10, "items": []string{"a"}}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Simulate recovery: apply pending writes from uncommitted checkpoint
	pending := []checkpoint.PendingWrite{
		{NodeName: "restored", Channel: "counter", Value: 20},
		{NodeName: "restored", Channel: "items", Value: []string{"b", "c"}},
		{NodeName: "restored", Channel: "new_key", Value: "recovered"},
	}
	bsp.ApplyPendingWrites(pending)

	// Verify writes were applied to committed state and read view
	view := bsp.ReadView()

	// Scalar update
	if v, ok := view.GetValue("counter"); !ok || v.(int) != 20 {
		t.Errorf("expected counter=20 after ApplyPendingWrites, got %v", v)
	}

	// List merge
	items, ok := view.GetValue("items")
	if !ok {
		t.Fatal("expected items to exist")
	}
	list := items.([]string)
	if len(list) != 3 {
		t.Errorf("expected 3 items after merge, got %d: %v", len(list), list)
	}

	// New key
	if v, ok := view.GetValue("new_key"); !ok || v.(string) != "recovered" {
		t.Errorf("expected new_key=recovered, got %v", v)
	}
}

func TestBSPStateTwoPhaseCommitFlow(t *testing.T) {
	// Tests the full two-phase commit flow:
	// 1. Node executes and writes
	// 2. Phase 1: Capture pending writes (for checkpoint)
	// 3. Phase 2: Commit barrier
	// 4. State is now visible

	initial := map[string]any{"step": 0}
	bsp := graph.NewBSPState(initial, graph.NewKeyRegistry())

	// Superstep 1: Node writes
	bsp.Write("node-two-phase", graph.Updates{"step": 1, "data": "superstep1"})

	// Phase 1: Capture pending writes (as would be saved to checkpoint)
	pending1 := bsp.PendingWrites()
	if len(pending1) != 2 {
		t.Errorf("phase 1: expected 2 pending writes, got %d", len(pending1))
	}

	// Committed state should still be old (pre-barrier)
	snapshot1 := bsp.Snapshot()
	if snapshot1["step"] != 0 {
		t.Errorf("phase 1: committed state should be step=0, got %v", snapshot1["step"])
	}

	// Phase 2: Commit barrier
	bsp.CommitBarrier()

	// Now committed state should be updated
	snapshot2 := bsp.Snapshot()
	if snapshot2["step"] != 1 {
		t.Errorf("phase 2: committed state should be step=1, got %v", snapshot2["step"])
	}
	if snapshot2["data"] != "superstep1" {
		t.Errorf("phase 2: committed state should have data=superstep1, got %v", snapshot2["data"])
	}

	// No more pending writes
	if bsp.HasPendingWrites() {
		t.Error("phase 2: expected no pending writes after barrier")
	}
}

// ====================
// Slice Merge Tests
// ====================

// Custom type for testing with non-primitive types
type testMessage struct {
	ID   int
	Text string
}

func TestBSPStateMergeSlicesViaWrite(t *testing.T) {
	t.Run("slices merge via reflection fallback", func(t *testing.T) {
		bsp := graph.NewBSPState(nil, graph.NewKeyRegistry())

		// First write
		bsp.Write("node1", graph.Updates{
			"test_messages": []testMessage{{ID: 1, Text: "first"}},
		})

		// Second write should merge
		bsp.Write("node2", graph.Updates{
			"test_messages": []testMessage{{ID: 2, Text: "second"}},
		})

		// Commit
		bsp.CommitBarrier()

		// Check result
		val, ok := bsp.GetCommitted("test_messages")
		if !ok {
			t.Fatal("expected messages key to exist")
		}

		result, ok := val.([]testMessage)
		if !ok {
			t.Fatalf("expected []testMessage, got %T", val)
		}

		if len(result) != 2 {
			t.Errorf("expected 2 messages, got %d", len(result))
		}
		if result[0].ID != 1 || result[1].ID != 2 {
			t.Errorf("unexpected messages: %v", result)
		}
	})

	t.Run("multiple writes accumulate correctly", func(t *testing.T) {
		bsp := graph.NewBSPState(nil, graph.NewKeyRegistry())

		// Multiple writes in same superstep
		for i := 0; i < 5; i++ {
			bsp.Write("node", graph.Updates{
				"items": []int{i},
			})
		}

		bsp.CommitBarrier()

		val, _ := bsp.GetCommitted("items")
		result := val.([]int)

		if len(result) != 5 {
			t.Errorf("expected 5 items, got %d", len(result))
		}
		for i := 0; i < 5; i++ {
			if result[i] != i {
				t.Errorf("expected result[%d]=%d, got %d", i, i, result[i])
			}
		}
	})
}

func TestGetListWithPlainSlice(t *testing.T) {
	key := graph.NewListKey[testMessage]("test_list_messages")

	bsp := graph.NewBSPState(map[string]any{
		"test_list_messages": []testMessage{{ID: 1, Text: "test"}},
	}, graph.NewKeyRegistry())

	view := bsp.ReadView()
	result := graph.GetList(view, key)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].ID != 1 || result[0].Text != "test" {
		t.Errorf("unexpected message: %v", result[0])
	}
}

func TestMergeSlicesPrimitiveFallback(t *testing.T) {
	// Test that primitive slices still work via type switch
	bsp := graph.NewBSPState(nil, graph.NewKeyRegistry())

	// Write plain []string
	bsp.Write("node1", graph.Updates{
		"tags": []string{"a", "b"},
	})
	bsp.Write("node2", graph.Updates{
		"tags": []string{"c"},
	})

	bsp.CommitBarrier()

	val, _ := bsp.GetCommitted("tags")
	result, ok := val.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", val)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 tags, got %d", len(result))
	}
}
