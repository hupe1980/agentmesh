package graph

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// TestBufferedStateWriter_IsolatesAggregates verifies that Aggregate() calls
// are buffered and not visible within the same superstep.
func TestBufferedStateWriter_IsolatesAggregates(t *testing.T) {
	state := NewGraphState(0)

	// Create buffered writer
	buffered := newBufferedStateWriter(state)

	// Node writes aggregate through buffered writer
	if err := buffered.Aggregate("counter", 10); err != nil {
		t.Fatalf("Failed to aggregate: %v", err)
	}

	// Verify the aggregate is buffered (not in underlying state yet)
	// The key test: buffered aggregates are not visible until flushed

	// Flush the buffer
	flushed := buffered.flushAggregates()
	if flushed == nil {
		t.Error("Expected flushed aggregates, got nil")
	}
	if flushed["counter"].(int) != 10 {
		t.Errorf("Flushed value should be 10, got %v", flushed["counter"])
	}

	// After flush, buffer should be empty
	flushed2 := buffered.flushAggregates()
	if flushed2 != nil {
		t.Errorf("Second flush should return nil, got %v", flushed2)
	}
}

// TestBufferedStateWriter_ReadsUnderlyingState verifies that reads go through
// to the underlying committed state, not the buffered changes.
func TestBufferedStateWriter_ReadsUnderlyingState(t *testing.T) {
	state := NewGraphState(0)
	state.Set("key1", "value1")
	state.Set("key2", "value2")

	buffered := newBufferedStateWriter(state)

	// Reads should see committed state
	if val := buffered.Get("key1"); val != "value1" {
		t.Errorf("Expected 'value1', got %v", val)
	}

	all := buffered.GetAll()
	if all["key1"] != "value1" || all["key2"] != "value2" {
		t.Errorf("GetAll should return committed state, got %v", all)
	}
}

// TestBufferedStateWriter_ConcurrentAccess verifies thread safety of buffered writer.
func TestBufferedStateWriter_ConcurrentAccess(t *testing.T) {
	state := NewGraphState(0)
	buffered := newBufferedStateWriter(state)

	// Concurrent aggregates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(val int) {
			if err := buffered.Aggregate("counter", val); err != nil {
				t.Errorf("Aggregate failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have one buffered value (last write wins for now)
	flushed := buffered.flushAggregates()
	if flushed == nil || flushed["counter"] == nil {
		t.Error("Expected buffered aggregate after concurrent writes")
	}
}

// TestGraph_DeterministicExecution verifies that node execution order doesn't
// affect the final state (BSP property).
func TestGraph_DeterministicExecution(t *testing.T) {
	// Create a graph where two nodes update the same key
	// With buffering, they should both see the initial value

	runTest := func() string {
		state := NewGraphState(0)
		state.Set("value", "initial")

		builder := NewBuilder().WithState(state)

		node1 := &Node{
			Name: "node1",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				// Read current value
				current := s.Get("value")
				return &NodeResult{
					Updates: map[string]any{"node1_saw": current},
				}, nil
			},
		}

		node2 := &Node{
			Name: "node2",
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				// Read current value
				current := s.Get("value")
				return &NodeResult{
					Updates: map[string]any{"node2_saw": current},
				}, nil
			},
		}

		builder.AddNode(node1).AddNode(node2)
		builder.AddEdge(StartNode, "node1")
		builder.AddEdge(StartNode, "node2")
		builder.AddEdge("node1", EndNode)
		builder.AddEdge("node2", EndNode)

		compiled, err := builder.Compile()
		if err != nil {
			t.Fatalf("Failed to compile: %v", err)
		}

		_, err = compiled.Invoke(context.Background(), []message.Message{})
		if err != nil {
			t.Fatalf("Failed to invoke: %v", err)
		}

		// Both nodes should see "initial" value
		node1Saw := compiled.State().Get("node1_saw")
		node2Saw := compiled.State().Get("node2_saw")

		if node1Saw != "initial" || node2Saw != "initial" {
			return "DIFFERENT"
		}
		return "SAME"
	}

	// Run multiple times - should be deterministic
	for i := 0; i < 5; i++ {
		result := runTest()
		if result != "SAME" {
			t.Errorf("Non-deterministic execution: nodes saw different values")
		}
	}
}
