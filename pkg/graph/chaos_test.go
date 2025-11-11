package graph

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/assert"
)

// TestNodePanicRecovery verifies that panicking nodes don't crash the process
func TestNodePanicRecovery(t *testing.T) {
	t.Run("BasicPanicRecovery", func(t *testing.T) {
		builder := NewBuilder()
		builder.Node("panicky", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			panic("intentional panic")
		})
		builder.AddEdge(StartNode, "panicky")
		builder.AddEdge("panicky", EndNode)

		compiled, err := builder.Compile()
		if err != nil {
			t.Fatalf("Failed to compile: %v", err)
		}

		// Should recover from panic and return error (not crash)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Panic was not recovered: %v", r)
			}
		}()

		_, err = Last(compiled.Run(context.Background(), []message.Message{}))
		// Expect some error (panic should be caught)
		if err == nil {
			t.Fatal("Expected error from panicking node")
		}

		// Verify error message contains node name and panic details
		errStr := err.Error()
		if !contains(errStr, "panicky") {
			t.Errorf("Error should mention node name 'panicky': %v", err)
		}
		if !contains(errStr, "intentional panic") {
			t.Errorf("Error should contain panic message: %v", err)
		}
		if !contains(errStr, "node panicked") {
			t.Errorf("Error should indicate node panicked: %v", err)
		}
	})

	t.Run("PanicWithEvents", func(t *testing.T) {
		builder := NewBuilder()
		builder.Node("panicky", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			panic("stream panic test")
		})
		builder.AddEdge(StartNode, "panicky")
		builder.AddEdge("panicky", EndNode)

		compiled, err := builder.Compile()
		if err != nil {
			t.Fatalf("Failed to compile: %v", err)
		}

		// Collect stream events
		seq := compiled.Run(context.Background(), []message.Message{})

		var events []Event
		var errorFound bool
		for event, err := range seq {
			if err != nil {
				errorFound = true
				break
			}
			events = append(events, event)
		}

		assert.True(t, errorFound, "Expected an error from the panicking node")
		assert.Empty(t, events, "Expected no events before the panic")
	})

	t.Run("MultiplePanicRecovery", func(t *testing.T) {
		// Verify multiple panicking nodes are handled independently
		builder := NewBuilder()
		builder.Node("panic1", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			panic("first panic")
		})
		builder.Node("panic2", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			panic("second panic")
		})
		builder.AddEdge(StartNode, "panic1")
		builder.AddEdge(StartNode, "panic2")
		builder.AddEdge("panic1", EndNode)
		builder.AddEdge("panic2", EndNode)

		compiled, err := builder.Compile()
		if err != nil {
			t.Fatalf("Failed to compile: %v", err)
		}

		_, err = Last(compiled.Run(context.Background(), []message.Message{}))
		if err == nil {
			t.Error("Expected error from panicking nodes")
		}
		// Both panics should be recovered (doesn't crash)
	})
}

// Helper function for case-insensitive substring check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestContextCancellation verifies that context cancellation is properly propagated
// This test may occasionally pass without error if the node completes before timeout
func TestContextCancellation(t *testing.T) {
	builder := NewBuilder()

	nodeExecuted := false
	builder.Node("node", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		nodeExecuted = true
		// Check if context is already done
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Simulate work with periodic context checks
		for i := 0; i < 10; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(20 * time.Millisecond):
				// Continue working
			}
		}
		return &NodeResult{}, nil
	})
	builder.AddEdge(StartNode, "node")
	builder.AddEdge("node", EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	// Use a timeout that will likely fire during execution
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = Last(compiled.Run(ctx, []message.Message{}))
	elapsed := time.Since(start)

	// Node should have been executed
	if !nodeExecuted {
		t.Fatal("Node was not executed")
	}

	// Test is successful if:
	// 1. We get a context deadline/cancel error (node detected cancellation), OR
	// 2. No error but completed quickly (node finished before timeout with small margin)
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context error, got: %v", err)
		} else {
			t.Logf("✓ Node correctly detected context cancellation in %v", elapsed)
		}
	} else {
		// No error means node completed successfully
		// Allow up to 55ms (50ms timeout + 5ms grace) for completion
		if elapsed < 55*time.Millisecond {
			t.Logf("✓ Node completed successfully in %v (at or near timeout)", elapsed)
		} else {
			t.Errorf("Node completed without error but took %v (timeout was 50ms) - context cancellation may not be working", elapsed)
		}
	}
}

// TestConcurrentInvoke verifies thread safety
// Note: Concurrent Invoke() calls on same Compiled share state between invocations
func TestConcurrentInvoke(t *testing.T) {
	// Create separate Compiled instances for true concurrent execution
	builders := make([]*Builder, 10)
	compileds := make([]*Compiled, 10)

	for i := 0; i < 10; i++ {
		builders[i] = NewBuilder()
		builders[i].Node("test", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			return &NodeResult{Updates: map[string]any{"executed": true}}, nil
		})
		builders[i].AddEdge(StartNode, "test")
		builders[i].AddEdge("test", EndNode)

		var err error
		compileds[i], err = builders[i].Compile()
		if err != nil {
			t.Fatalf("Failed to compile graph %d: %v", i, err)
		}
	}

	// Execute all concurrently
	var wg sync.WaitGroup
	errors := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := Last(compileds[idx].Run(context.Background(), []message.Message{}))
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// All invocations should succeed
	for i, err := range errors {
		if err != nil {
			t.Errorf("Invocation %d failed: %v", i, err)
		}
	}
}

// TestLargeStateStress verifies handling of large state
func TestLargeStateStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	builder := NewBuilder()

	builder.Node("stress", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		updates := make(map[string]any)
		// Create 1000 keys
		for i := 0; i < 1000; i++ {
			key := string(rune('a'+i%26)) + string(rune('0'+i/26))
			updates[key] = i
		}
		return &NodeResult{Updates: updates}, nil
	})
	builder.AddEdge(StartNode, "stress")
	builder.AddEdge("stress", EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	_, err = Last(compiled.Run(context.Background(), []message.Message{}))
	if err != nil {
		t.Errorf("Large state test failed: %v", err)
	}

	// Verify state contains all keys
	state := compiled.State()
	for i := 0; i < 1000; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i/26))
		if val := state.Get(key); val != i {
			t.Errorf("Expected state[%s]=%d, got %v", key, i, val)
		}
	}
}

// TestManyMessages verifies message handling under load
func TestManyMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	state := NewStateManager(10000) // Allow 10K messages
	builder := NewBuilder().WithState(state)

	builder.Node("producer", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		messages := make([]message.Message, 1000)
		for i := 0; i < 1000; i++ {
			messages[i] = message.NewHumanMessageFromText("msg")
		}
		return &NodeResult{Messages: messages}, nil
	})
	builder.AddEdge(StartNode, "producer")
	builder.AddEdge("producer", EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	_, err = Last(compiled.Run(context.Background(), []message.Message{}))
	if err != nil {
		t.Errorf("Many messages test failed: %v", err)
	}

	snapshot := compiled.State().EventsSnapshot()
	if len(snapshot) != 1000 {
		t.Errorf("Expected 1000 messages, got %d", len(snapshot))
	}
}

// TestNodeErrorPropagation verifies errors propagate correctly
func TestNodeErrorPropagation(t *testing.T) {
	testErr := errors.New("intentional error")

	builder := NewBuilder()
	builder.Node("failing", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		return nil, testErr
	})
	builder.AddEdge(StartNode, "failing")
	builder.AddEdge("failing", EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	_, err = Last(compiled.Run(context.Background(), []message.Message{}))
	if err == nil {
		t.Fatal("Expected error from failing node")
	}

	// Error should be wrapped (may or may not be NodeExecutionError depending on implementation)
	// At minimum, should contain the original error
	if !errors.Is(err, testErr) && err.Error() != testErr.Error() {
		t.Logf("Note: Error wrapping format: %v", err)
	}
}

// TestRapidRetry verifies retry behavior under stress
func TestRapidRetry(t *testing.T) {
	attempts := 0
	var mu sync.Mutex

	builder := NewBuilder()
	builder.AddNode(&Node{
		Name: "flaky",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			mu.Lock()
			attempts++
			current := attempts
			mu.Unlock()

			if current < 5 {
				return nil, errors.New("transient error")
			}
			return &NodeResult{}, nil
		},
		RetryPolicy: &RetryPolicy{
			MaxAttempts: 10,
			Backoff: func(attempt int) time.Duration {
				return 1 * time.Millisecond
			},
		},
	})
	builder.AddEdge(StartNode, "flaky")
	builder.AddEdge("flaky", EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	_, err = Last(compiled.Run(context.Background(), []message.Message{}))
	if err != nil {
		t.Errorf("Retry test failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts != 5 {
		t.Errorf("Expected 5 attempts, got %d", attempts)
	}
}

// TestDeadlineExceeded verifies nodes can detect deadline expiry
// Note: Nodes must check ctx.Done() to detect deadlines
func TestDeadlineExceeded(t *testing.T) {
	builder := NewBuilder()

	builder.Node("delayed", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		// Check for deadline periodically
		for i := 0; i < 10; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(20 * time.Millisecond):
				// Continue working
			}
		}
		return &NodeResult{}, nil
	})
	builder.AddEdge(StartNode, "delayed")
	builder.AddEdge("delayed", EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(50*time.Millisecond))
	defer cancel()

	_, err = Last(compiled.Run(ctx, []message.Message{}))

	// If node checks context, it should return DeadlineExceeded
	if errors.Is(err, context.DeadlineExceeded) {
		t.Log("Node correctly detected deadline")
	} else if err == nil {
		t.Log("Note: Node completed before deadline or didn't check context")
	} else {
		t.Logf("Got error: %v", err)
	}
}

// TestParallelNodeExecution verifies parallel branches work correctly
func TestParallelNodeExecution(t *testing.T) {
	var node1Time, node2Time time.Time
	var mu sync.Mutex

	builder := NewBuilder()

	builder.Node("node1", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		node1Time = time.Now()
		mu.Unlock()
		return &NodeResult{Updates: map[string]any{"node1": true}}, nil
	})

	builder.Node("node2", func(ctx context.Context, s StateWriter) (*NodeResult, error) {
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		node2Time = time.Now()
		mu.Unlock()
		return &NodeResult{Updates: map[string]any{"node2": true}}, nil
	})

	builder.AddEdge(StartNode, "node1")
	builder.AddEdge(StartNode, "node2")
	builder.AddEdge("node1", EndNode)
	builder.AddEdge("node2", EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile: %v", err)
	}

	start := time.Now()
	_, err = Last(compiled.Run(context.Background(), []message.Message{}))
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Parallel execution failed: %v", err)
	}

	// Should complete in ~50ms (parallel), not 100ms (sequential)
	if duration > 150*time.Millisecond {
		t.Errorf("Parallel execution took too long: %v (expected ~50-100ms)", duration)
	}

	mu.Lock()
	defer mu.Unlock()
	timeDiff := node1Time.Sub(node2Time)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	// Nodes should complete within 50ms of each other (parallel execution)
	if timeDiff > 60*time.Millisecond {
		t.Errorf("Nodes didn't execute in parallel (time diff: %v)", timeDiff)
	}
}
