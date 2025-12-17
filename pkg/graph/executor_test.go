package graph_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/stretchr/testify/require"
)

// recordingCheckpointer captures every checkpoint saved during a run so tests can
// inspect intermediate two-phase commits without needing a persistent backend.
type recordingCheckpointer struct {
	mu    sync.Mutex
	saved []*checkpoint.Checkpoint
}

func newRecordingCheckpointer() *recordingCheckpointer {
	return &recordingCheckpointer{}
}

func (r *recordingCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	clone := &checkpoint.Checkpoint{
		RunID:     cp.RunID,
		Superstep: cp.Superstep,
		Version:   cp.Version,
		Timestamp: cp.Timestamp,
		Committed: cp.Committed,
	}

	if len(cp.State) > 0 {
		clone.State = make(map[string]any, len(cp.State))
		for k, v := range cp.State {
			clone.State[k] = v
		}
	}

	if len(cp.PendingWrites) > 0 {
		clone.PendingWrites = append([]checkpoint.PendingWrite(nil), cp.PendingWrites...)
	}

	if len(cp.CompletedNodes) > 0 {
		clone.CompletedNodes = append([]string(nil), cp.CompletedNodes...)
	}

	if len(cp.PausedNodes) > 0 {
		clone.PausedNodes = append([]string(nil), cp.PausedNodes...)
	}

	r.saved = append(r.saved, clone)
	return nil
}

func (r *recordingCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	return nil, nil
}

func (r *recordingCheckpointer) List(ctx context.Context, runID string) ([]*checkpoint.Checkpoint, error) {
	return nil, nil
}

func (r *recordingCheckpointer) Delete(ctx context.Context, runID string) error {
	return nil
}

func (r *recordingCheckpointer) LoadAtSuperstep(ctx context.Context, runID string, superstep int64) (*checkpoint.Checkpoint, error) {
	return nil, nil
}

func (r *recordingCheckpointer) ListPendingApprovals(ctx context.Context) ([]*checkpoint.Checkpoint, error) {
	return nil, nil
}

func (r *recordingCheckpointer) GetApprovalHistory(ctx context.Context, runID string) ([]checkpoint.ApprovalRecord, error) {
	return nil, nil
}

func (r *recordingCheckpointer) firstUncommitted() *checkpoint.Checkpoint {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cp := range r.saved {
		if !cp.Committed {
			return cp
		}
	}
	return nil
}

// ====================
// PregelExecutor Basic Tests
// ====================

func TestPregelExecutorBasic(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)
	g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		return graph.Set(counterKey, counter+1).To(graph.END)
	}, graph.END)
	g.Start("process")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Run with default executor
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}
}

func TestPregelExecutorWithCustomExecutor(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	var execCount atomic.Int32

	g := graph.New(counterKey)
	g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		execCount.Add(1)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("process")

	// Use custom executor with max workers
	executor := graph.NewPregelExecutor().WithMaxWorkers(2)
	g.WithExecutor(executor)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if execCount.Load() != 1 {
		t.Errorf("expected 1 execution, got %d", execCount.Load())
	}
}

func TestPregelExecutorWithMaxSteps(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	var iterations atomic.Int32

	g := graph.New(counterKey)
	g.Node("loop", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		iterations.Add(1)
		// Infinite loop - will be stopped by max steps
		return graph.To("loop")
	}, "loop", graph.END)
	g.Start("loop")

	// Use executor with limited execution steps
	executor := graph.NewPregelExecutor().WithMaxSteps(5)
	g.WithExecutor(executor)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		// MaxSteps should trigger stop, may produce an error or just stop
		_ = err
	}

	// Should have executed at most 5 times
	if iterations.Load() > 5 {
		t.Errorf("expected at most 5 iterations, got %d", iterations.Load())
	}
}

// ====================
// BSP Semantics Tests
// ====================

func TestBSPStateIsolation(t *testing.T) {
	// Test that parallel nodes see the same state snapshot (BSP semantics)
	valueKey := graph.NewKey[int]("value")
	readValues := make([]int, 0)
	var mu sync.Mutex

	g := graph.New(valueKey)

	// Entry point sets initial value and fans out
	g.Node("init", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(valueKey, 10).To("reader1", "reader2")
	}, "reader1", "reader2")

	// Both readers should see the same value (from previous superstep)
	g.Node("reader1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		val := graph.Get(scope, valueKey)
		mu.Lock()
		readValues = append(readValues, val)
		mu.Unlock()
		return graph.Set(valueKey, val+1).To(graph.END)
	}, graph.END)

	g.Node("reader2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		val := graph.Get(scope, valueKey)
		mu.Lock()
		readValues = append(readValues, val)
		mu.Unlock()
		return graph.Set(valueKey, val+100).To(graph.END)
	}, graph.END)

	g.Start("init")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Both readers should have seen the same value (10)
	if len(readValues) != 2 {
		t.Fatalf("expected 2 read values, got %d", len(readValues))
	}
	if readValues[0] != 10 || readValues[1] != 10 {
		t.Errorf("BSP violation: both nodes should see value=10, got %v", readValues)
	}
}

func TestBSPWriteBuffering(t *testing.T) {
	// Test that writes are buffered until superstep barrier
	counterKey := graph.NewKey[int]("counter")
	var step1Val, step2Val int

	g := graph.New(counterKey)

	g.Node("writer", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		step1Val = graph.Get(scope, counterKey)
		return graph.Set(counterKey, 42).To("reader")
	}, "reader")

	g.Node("reader", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Should see the committed value from previous superstep (42)
		step2Val = graph.Get(scope, counterKey)
		return graph.To(graph.END)
	}, graph.END)

	g.Start("writer")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if step1Val != 0 {
		t.Errorf("step1 should see initial value 0, got %d", step1Val)
	}
	if step2Val != 42 {
		t.Errorf("step2 should see committed value 42, got %d", step2Val)
	}
}

// ====================
// Checkpointing Tests
// ====================

func TestCheckpointingSaveAndRestore(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	g := graph.New(counterKey)
	g.Node("increment", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		return graph.Set(counterKey, counter+1).To(graph.END)
	}, graph.END)
	g.Start("increment")
	g.WithCheckpointer(checkpointer, "test-run")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// First run
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Verify checkpoint was saved
	cp, err := checkpointer.Load(context.Background(), "test-run")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("Expected checkpoint to be saved, got nil")
	}
	if cp.RunID != "test-run" {
		t.Errorf("Expected runID 'test-run', got '%s'", cp.RunID)
	}
}

func TestCheckpointingAutoRestore(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Pre-save a checkpoint with counter=100
	preCheckpoint := &checkpoint.Checkpoint{
		RunID:     "restore-test",
		Superstep: 1,
		State: map[string]any{
			"counter": 100,
		},
		Committed: true,
		Timestamp: time.Now(),
	}
	if err := checkpointer.Save(context.Background(), preCheckpoint); err != nil {
		t.Fatalf("Failed to pre-save checkpoint: %v", err)
	}

	var restoredValue int

	g := graph.New(counterKey)
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		restoredValue = graph.Get(scope, counterKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")
	g.WithCheckpointer(checkpointer, "restore-test")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Resume with auto-restore (default behavior)
	for _, err := range compiled.Resume(context.Background(), "restore-test") {
		if err != nil {
			t.Fatalf("Resume failed: %v", err)
		}
	}

	if restoredValue != 100 {
		t.Errorf("Expected restored value 100, got %d", restoredValue)
	}
}

func TestCheckpointingWithInterval(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	g := graph.New(counterKey)

	// Create a multi-step graph
	g.Node("step1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(counterKey, 1).To("step2")
	}, "step2")
	g.Node("step2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(counterKey, 2).To("step3")
	}, "step3")
	g.Node("step3", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(counterKey, 3).To(graph.END)
	}, graph.END)
	g.Start("step1")
	g.WithCheckpointer(checkpointer, "interval-test")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Run with checkpoint interval of 2
	for _, err := range compiled.Run(context.Background(), nil, graph.WithCheckpointInterval(2)) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Should have checkpoints
	checkpoints, err := checkpointer.List(context.Background(), "interval-test")
	if err != nil {
		t.Fatalf("Failed to list checkpoints: %v", err)
	}
	if len(checkpoints) == 0 {
		t.Error("Expected at least one checkpoint with interval=2")
	}
}

func TestCheckpointingWithPendingWrites(t *testing.T) {
	t.Run("applies pending writes on restore", func(t *testing.T) {
		counterKey := graph.NewKey[int]("counter")
		checkpointer := checkpoint.NewInMemoryCheckpointer()

		pendingCheckpoint := &checkpoint.Checkpoint{
			RunID:     "pending-test",
			Superstep: 1,
			State: map[string]any{
				"counter": 0,
			},
			PendingWrites: []checkpoint.PendingWrite{
				{NodeName: "writer", Channel: "counter", Value: 50, Timestamp: time.Now()},
			},
			Committed: false,
			Timestamp: time.Now(),
		}
		if err := checkpointer.Save(context.Background(), pendingCheckpoint); err != nil {
			t.Fatalf("Failed to save pending checkpoint: %v", err)
		}

		var restoredValue int

		g := graph.New(counterKey)
		g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			restoredValue = graph.Get(scope, counterKey)
			return graph.To(graph.END)
		}, graph.END)
		g.Start("read")
		g.WithCheckpointer(checkpointer, "pending-test")

		compiled, err := g.Build()
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		for _, err := range compiled.Resume(context.Background(), "pending-test") {
			if err != nil {
				t.Fatalf("Resume failed: %v", err)
			}
		}

		if restoredValue != 50 {
			t.Errorf("Expected pending write value 50 to be applied, got %d", restoredValue)
		}
	})

	t.Run("records node provenance in pending writes", func(t *testing.T) {
		counterKey := graph.NewKey[int]("counter")
		recorder := newRecordingCheckpointer()

		g := graph.New(counterKey)
		g.Node("writer", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			counter := graph.Get(scope, counterKey)
			return graph.Set(counterKey, counter+1).To(graph.END)
		}, graph.END)
		g.Start("writer")
		g.WithCheckpointer(recorder, "provenance-run")

		compiled, err := g.Build()
		if err != nil {
			t.Fatalf("Build failed: %v", err)
		}

		for _, err := range compiled.Run(
			context.Background(),
			nil,
			graph.WithRunID("provenance-run"),
			graph.WithCheckpointInterval(1),
		) {
			if err != nil {
				t.Fatalf("Run failed: %v", err)
			}
		}

		pending := recorder.firstUncommitted()
		if pending == nil {
			t.Fatal("expected an uncommitted checkpoint with pending writes")
		}
		if len(pending.PendingWrites) == 0 {
			t.Fatal("expected pending writes to be captured")
		}
		write := pending.PendingWrites[0]
		if write.NodeName != "writer" {
			t.Errorf("expected pending write node name 'writer', got %q", write.NodeName)
		}
		if write.Channel != "counter" {
			t.Errorf("expected pending write channel 'counter', got %q", write.Channel)
		}
		if write.Timestamp.IsZero() {
			t.Error("expected pending write timestamp to be set")
		}
	})
}

func TestResumeMergesInput(t *testing.T) {
	// Verify that a resume merges new input into the checkpointed state
	messagesKey := graph.NewListKey[string]("test_messages")
	countKey := graph.NewKey[int]("count")
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "state-merge"

	var capturedMessages []string
	var capturedCount int

	g := graph.New(messagesKey, countKey)

	g.Node("A", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		cnt := graph.Get(scope, countKey)
		return graph.Cmd().
			With(graph.SetValue(messagesKey, []string{"A executed"})).
			With(graph.SetValue(countKey, cnt+1)).
			To("B")
	}, "B")

	g.Node("B", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		cnt := graph.Get(scope, countKey)
		return graph.Cmd().
			With(graph.SetValue(messagesKey, []string{"B executed"})).
			With(graph.SetValue(countKey, cnt+2)).
			To("collect")
	}, "collect")

	g.Node("collect", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedMessages = graph.GetList(scope, messagesKey)
		capturedCount = graph.Get(scope, countKey)
		return graph.To(graph.END)
	}, graph.END)

	g.Start("A")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	require.NoError(t, err)

	// First run: seeds state and checkpoints
	for _, err := range compiled.Run(context.Background(), nil,
		graph.WithInitialValue(messagesKey, []string{"hi"}),
	) {
		require.NoError(t, err)
	}
	require.Equal(t, []string{"hi", "A executed", "B executed"}, capturedMessages)
	require.Equal(t, 3, capturedCount)

	// Second run with Resume - should merge with checkpoint state
	for _, err := range compiled.Resume(context.Background(), runID,
		graph.WithStateUpdates(map[string]any{messagesKey.Name(): []string{"new message"}}),
	) {
		require.NoError(t, err)
	}

	// Expected merged behavior: old + new input + new execution
	expectedMessages := []string{"hi", "A executed", "B executed", "new message", "A executed", "B executed"}
	require.Equal(t, expectedMessages, capturedMessages)
	require.Equal(t, 6, capturedCount) // 3 (checkpoint) + 3 (A/B increments on resumed run)
}

// ====================
// Interrupt Tests
// ====================

func TestInterruptBefore(t *testing.T) {
	statusKey := graph.NewKey[string]("status")

	g := graph.New(statusKey)
	g.Node("sensitive", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(statusKey, "executed").To(graph.END)
	}, graph.END)
	g.InterruptBefore("sensitive")
	g.Start("sensitive")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Run without approval - should get interrupt error
	var gotErr error
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			gotErr = err
			break
		}
	}

	if gotErr == nil {
		t.Fatal("Expected interrupt error, got nil")
	}

	var interruptErr *graph.InterruptError
	if !errors.As(gotErr, &interruptErr) {
		t.Fatalf("Expected InterruptError, got %T: %v", gotErr, gotErr)
	}
	if interruptErr.NodeName != "sensitive" {
		t.Errorf("Expected node 'sensitive', got '%s'", interruptErr.NodeName)
	}
	if !interruptErr.Before {
		t.Error("Expected Before=true for InterruptBefore")
	}
}

func TestInterruptAfter(t *testing.T) {
	statusKey := graph.NewKey[string]("status")
	var nodeExecuted bool

	g := graph.New(statusKey)
	g.Node("action", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		nodeExecuted = true
		return graph.Set(statusKey, "completed").To(graph.END)
	}, graph.END)
	g.InterruptAfter("action")
	g.Start("action")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Run without approval
	var gotErr error
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			gotErr = err
			break
		}
	}

	// Node should have executed
	if !nodeExecuted {
		t.Error("Expected node to execute before interrupt")
	}

	// But we should still get interrupt error
	if gotErr == nil {
		t.Fatal("Expected interrupt error, got nil")
	}

	var interruptErr *graph.InterruptError
	if !errors.As(gotErr, &interruptErr) {
		t.Fatalf("Expected InterruptError, got %T: %v", gotErr, gotErr)
	}
	if interruptErr.Before {
		t.Error("Expected Before=false for InterruptAfter")
	}
}

func TestInterruptWithApproval(t *testing.T) {
	statusKey := graph.NewKey[string]("status")
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "approval-test"

	nodeExecutedFlag := false

	g := graph.New(statusKey)
	g.Node("sensitive", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		nodeExecutedFlag = true
		scope.Stream(message.NewAIMessageFromText("done"))
		return graph.Set(statusKey, "executed").To(graph.END)
	}, graph.END)
	g.InterruptBefore("sensitive")
	g.Start("sensitive")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// First Run should hit the interrupt
	var gotInterrupt bool
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			var interruptErr *graph.InterruptError
			if errors.As(err, &interruptErr) {
				gotInterrupt = true
			}
		}
	}
	if !gotInterrupt {
		t.Fatal("Expected interrupt before sensitive node")
	}

	// Resume WITH approval - should proceed past interrupt
	approval := &graph.ApprovalResponse{
		Decision:  graph.ApprovalApproved,
		Timestamp: time.Now(),
	}
	for _, err := range compiled.Resume(context.Background(), runID, graph.WithApproval("sensitive", approval)) {
		if err != nil {
			t.Fatalf("Resume failed: %v", err)
		}
	}

	if !nodeExecutedFlag {
		t.Error("Expected node to execute when approval provided via Resume")
	}
}

// ====================
// Error Handling Tests
// ====================

func TestNodeError(t *testing.T) {
	expectedErr := errors.New("node execution failed")

	g := graph.New()
	g.Node("failing", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Fail(expectedErr)
	}, graph.END)
	g.Start("failing")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	var gotErr error
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			gotErr = err
			break
		}
	}

	if gotErr == nil {
		t.Fatal("Expected error, got nil")
	}
	if !errors.Is(gotErr, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, gotErr)
	}
}

func TestContextCancellation(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	var executionCount atomic.Int32

	g := graph.New(counterKey)
	g.Node("slow", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		executionCount.Add(1)
		// Check context before doing work
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		return graph.To("slow") // Loop
	}, "slow", graph.END)
	g.Start("slow")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	for _, err := range compiled.Run(ctx, nil) {
		// May get context deadline exceeded
		_ = err
	}

	// Should have tried at least once but stopped due to timeout
	count := executionCount.Load()
	if count == 0 {
		t.Error("Expected at least one execution attempt")
	}
}

// ====================
// Middleware Tests
// ====================

func TestMiddlewareExecution(t *testing.T) {
	var middlewareOrder []string
	var mu sync.Mutex

	middleware1 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			mu.Lock()
			middlewareOrder = append(middlewareOrder, "m1-before")
			mu.Unlock()
			cmd, err := next(ctx, scope)
			mu.Lock()
			middlewareOrder = append(middlewareOrder, "m1-after")
			mu.Unlock()
			return cmd, err
		}
	}

	middleware2 := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			mu.Lock()
			middlewareOrder = append(middlewareOrder, "m2-before")
			mu.Unlock()
			cmd, err := next(ctx, scope)
			mu.Lock()
			middlewareOrder = append(middlewareOrder, "m2-after")
			mu.Unlock()
			return cmd, err
		}
	}

	g := graph.New()
	g.Node("process", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		mu.Lock()
		middlewareOrder = append(middlewareOrder, "node")
		mu.Unlock()
		return graph.To(graph.END)
	}, graph.END)
	g.Start("process")
	g.WithNodeMiddleware(middleware1, middleware2)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Middleware should wrap in order: m1 wraps m2 wraps node
	// So execution order is: m1-before, m2-before, node, m2-after, m1-after
	expected := []string{"m1-before", "m2-before", "node", "m2-after", "m1-after"}
	if len(middlewareOrder) != len(expected) {
		t.Fatalf("Expected %d middleware calls, got %d: %v", len(expected), len(middlewareOrder), middlewareOrder)
	}
	for i, exp := range expected {
		if middlewareOrder[i] != exp {
			t.Errorf("Position %d: expected '%s', got '%s'", i, exp, middlewareOrder[i])
		}
	}
}

// ====================
// Run Options Tests
// ====================

func TestWithMaxConcurrency(t *testing.T) {
	valueKey := graph.NewKey[int]("value")
	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	g := graph.New(valueKey)
	g.Node("init", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("worker1", "worker2", "worker3", "worker4")
	}, "worker1", "worker2", "worker3", "worker4")

	workerFn := func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		current := currentConcurrent.Add(1)
		// Track max concurrent
		for {
			max := maxConcurrent.Load()
			if current <= max || maxConcurrent.CompareAndSwap(max, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond) // Simulate work
		currentConcurrent.Add(-1)
		return graph.To(graph.END)
	}

	g.Node("worker1", workerFn, graph.END)
	g.Node("worker2", workerFn, graph.END)
	g.Node("worker3", workerFn, graph.END)
	g.Node("worker4", workerFn, graph.END)
	g.Start("init")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Limit to 2 concurrent
	for _, err := range compiled.Run(context.Background(), nil, graph.WithMaxConcurrency(2)) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if maxConcurrent.Load() > 2 {
		t.Errorf("Expected max 2 concurrent, got %d", maxConcurrent.Load())
	}
}

func TestWithMaxIterations(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	var iterations atomic.Int32

	g := graph.New(counterKey)
	g.Node("loop", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		iterations.Add(1)
		return graph.To("loop") // Infinite loop
	}, "loop", graph.END)
	g.Start("loop")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Limit to 3 iterations
	for _, err := range compiled.Run(context.Background(), nil, graph.WithMaxIterations(3)) {
		_ = err
	}

	if iterations.Load() > 3 {
		t.Errorf("Expected max 3 iterations, got %d", iterations.Load())
	}
}

// ====================
// Streaming Output Tests
// ====================

func TestStreamingOutput(t *testing.T) {
	g := graph.New()
	g.Node("emit", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Emit multiple messages using ReplyAll
		msg1 := message.NewAIMessageFromText("hello")
		msg2 := message.NewAIMessageFromText("world")
		return graph.ReplyAll(msg1, msg2).To(graph.END)
	}, graph.END)
	g.Start("emit")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	var outputs []string
	for output, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		outputs = append(outputs, output.String())
	}

	// Should yield each message
	if len(outputs) != 2 {
		t.Fatalf("Expected 2 outputs, got %d: %v", len(outputs), outputs)
	}
	if outputs[0] != "hello" || outputs[1] != "world" {
		t.Errorf("Expected [hello, world], got %v", outputs)
	}
}

func TestStreamingWithSingleOutput(t *testing.T) {
	g := graph.New()
	g.Node("compute", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		msg := message.NewAIMessageFromText("done")
		return graph.Reply(msg).To(graph.END)
	}, graph.END)
	g.Start("compute")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	var outputs []string
	for output, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		outputs = append(outputs, output.String())
	}

	if len(outputs) != 1 {
		t.Fatalf("Expected 1 output, got %d", len(outputs))
	}
	if outputs[0] != "done" {
		t.Errorf("Expected 'done', got '%s'", outputs[0])
	}
}

// ====================
// List Key Tests
// ====================

func TestListKeyAppend(t *testing.T) {
	tagsKey := graph.NewListKey[string]("tags")
	var capturedTags []string

	g := graph.New(tagsKey)
	g.Node("add1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(tagsKey, []string{"tag1", "tag2"}).To("add2")
	}, "add2")
	g.Node("add2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(tagsKey, []string{"tag3"}).To("read")
	}, "read")
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedTags = graph.GetList(scope, tagsKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("add1")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if len(capturedTags) != 3 {
		t.Fatalf("Expected 3 tags, got %d: %v", len(capturedTags), capturedTags)
	}
	expected := []string{"tag1", "tag2", "tag3"}
	for i, exp := range expected {
		if capturedTags[i] != exp {
			t.Errorf("Tag %d: expected '%s', got '%s'", i, exp, capturedTags[i])
		}
	}
}

// ====================
// Input/Output Key Tests
// ====================

func TestInputMessages(t *testing.T) {
	var capturedMessages []message.Message

	g := graph.New()
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedMessages = graph.GetMessages(scope)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	inputMsg := message.NewHumanMessageFromText("test-input")
	for _, err := range compiled.Run(context.Background(), []message.Message{inputMsg}) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if len(capturedMessages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(capturedMessages))
	}
	if capturedMessages[0].String() != "test-input" {
		t.Errorf("Expected 'test-input', got '%s'", capturedMessages[0].String())
	}
}

// ====================
// Node Context Tests
// ====================

func TestNodeNameInScope(t *testing.T) {
	var capturedNodeName string

	middleware := func(next graph.NodeFunc) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			capturedNodeName = scope.NodeName()
			return next(ctx, scope)
		}
	}

	g := graph.New()
	g.Node("mynode", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("mynode")
	g.WithNodeMiddleware(middleware)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if capturedNodeName != "mynode" {
		t.Errorf("Expected node name 'mynode', got '%s'", capturedNodeName)
	}
}

// ====================
// Parallel Execution Tests
// ====================

func TestParallelNodeExecution(t *testing.T) {
	resultKey := graph.NewListKey[string]("results")
	var executionOrder []string
	var mu sync.Mutex

	g := graph.New(resultKey)

	g.Node("start", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("parallel1", "parallel2", "parallel3")
	}, "parallel1", "parallel2", "parallel3")

	createWorker := func(name string, delay time.Duration) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			time.Sleep(delay)
			mu.Lock()
			executionOrder = append(executionOrder, name)
			mu.Unlock()
			return graph.Set(resultKey, []string{name}).To(graph.END)
		}
	}

	g.Node("parallel1", createWorker("p1", 30*time.Millisecond), graph.END)
	g.Node("parallel2", createWorker("p2", 10*time.Millisecond), graph.END)
	g.Node("parallel3", createWorker("p3", 20*time.Millisecond), graph.END)
	g.Start("start")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	start := time.Now()
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}
	elapsed := time.Since(start)

	// All nodes should execute
	if len(executionOrder) != 3 {
		t.Errorf("Expected 3 executions, got %d", len(executionOrder))
	}

	// With parallel execution, total time should be less than sum of delays (60ms)
	// Should be around 30ms (longest delay) plus overhead
	if elapsed > 55*time.Millisecond {
		t.Logf("Warning: parallel execution took %v, may not be truly parallel", elapsed)
	}
}

// ====================
// Collect/Last Helper Tests
// ====================

func TestGraphCollect(t *testing.T) {
	g := graph.New()
	g.Node("emit", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		scope.Stream(message.NewAIMessageFromText("a"))
		scope.Stream(message.NewAIMessageFromText("b"))
		scope.Stream(message.NewAIMessageFromText("c"))
		return graph.To(graph.END)
	}, graph.END)
	g.Start("emit")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	results, err := graph.Collect(compiled.Run(context.Background(), nil))
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}
}

func TestGraphLast(t *testing.T) {
	g := graph.New()
	g.Node("emit", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		msg1 := message.NewAIMessageFromText("first")
		msg2 := message.NewAIMessageFromText("middle")
		msg3 := message.NewAIMessageFromText("last")
		return graph.ReplyAll(msg1, msg2, msg3).To(graph.END)
	}, graph.END)
	g.Start("emit")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	result, err := graph.Last(compiled.Run(context.Background(), nil))
	if err != nil {
		t.Fatalf("Last failed: %v", err)
	}

	if result.String() != "last" {
		t.Errorf("Expected 'last', got '%s'", result.String())
	}
}

// ====================
// Two-Phase Commit Tests
// ====================

func TestTwoPhaseCommitProtocol(t *testing.T) {
	counterKey := graph.NewKey[int]("counter")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	g := graph.New(counterKey)
	g.Node("step1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(counterKey, 10).To("step2")
	}, "step2")
	g.Node("step2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		return graph.Set(counterKey, counter+5).To(graph.END)
	}, graph.END)
	g.Start("step1")
	g.WithCheckpointer(checkpointer, "2pc-test")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Run graph
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Verify final checkpoint is committed
	cp, err := checkpointer.Load(context.Background(), "2pc-test")
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("Expected checkpoint, got nil")
	}
	if !cp.Committed {
		t.Error("Expected final checkpoint to be committed")
	}
}

func TestResumeRequiresManagedValues(t *testing.T) {
	resultKey := graph.NewKey[string]("result")
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	const runID = "managed-values-resume"

	apiKey := graph.NewManagedValue("api_key", "sk_live", graph.WithManagedValueRequired())

	g := graph.New(resultKey)
	g.Node("emit", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(resultKey, "ok").To(graph.END)
	}, graph.END)
	g.Start("emit")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	for _, err := range compiled.Run(ctx, nil,
		graph.WithRunID(runID),
		graph.WithManagedValues(apiKey),
	) {
		if err != nil {
			t.Fatalf("initial run failed: %v", err)
		}
	}

	cp, err := checkpointer.Load(ctx, runID)
	if err != nil {
		t.Fatalf("load checkpoint failed: %v", err)
	}
	if cp == nil || len(cp.ManagedValues) == 0 {
		t.Fatalf("expected checkpoint to capture managed value descriptors")
	}

	var resumeErr error
	for _, err := range compiled.Resume(ctx, runID) {
		if err != nil {
			resumeErr = err
			break
		}
	}

	if resumeErr == nil {
		t.Fatalf("expected resume to fail without managed values")
	}
	if !strings.Contains(resumeErr.Error(), "managed value") {
		t.Fatalf("unexpected resume error: %v", resumeErr)
	}
}

func TestResumeManagedValueRehydrate(t *testing.T) {
	resultKey := graph.NewKey[string]("result")
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	const runID = "managed-values-rehydrate"

	var rehydrateCalls atomic.Int32
	session := graph.NewManagedValue("session", map[string]string{"token": "initial"},
		graph.WithManagedValueRequired(),
		graph.WithManagedValueRehydrator(func(context.Context) error {
			rehydrateCalls.Add(1)
			return nil
		}),
	)

	g := graph.New(resultKey)
	g.Node("emit", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(resultKey, "ok").To(graph.END)
	}, graph.END)
	g.Start("emit")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	ctx := context.Background()
	for _, err := range compiled.Run(ctx, nil,
		graph.WithRunID(runID),
		graph.WithManagedValues(session),
	) {
		if err != nil {
			t.Fatalf("initial run failed: %v", err)
		}
	}

	rehydrateCalls.Store(0)
	for _, err := range compiled.Resume(ctx, runID,
		graph.WithManagedValues(session),
	) {
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}
	}

	if rehydrateCalls.Load() == 0 {
		t.Fatalf("expected rehydrate callback to run during resume")
	}
}

// ====================
// Slice Input Handling Tests
// ====================

func TestSliceInputNotOverwrittenByNil(t *testing.T) {
	// Test that slice inputs work correctly and don't get treated as nil
	messagesKey := graph.NewListKey[string]("test_messages")
	var capturedMessages []string

	g := graph.New(messagesKey)
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedMessages = graph.GetList(scope, messagesKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Pass initial values via WithInitialValue
	for _, err := range compiled.Run(context.Background(), nil,
		graph.WithInitialValue(messagesKey, []string{"hello", "world"}),
	) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if len(capturedMessages) != 2 {
		t.Errorf("Expected 2 messages, got %d: %v", len(capturedMessages), capturedMessages)
	}
}

func TestEmptySliceInputTreatedAsZero(t *testing.T) {
	// Empty slices should be treated as zero value
	messagesKey := graph.NewListKey[string]("test_messages")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Pre-save a checkpoint with messages
	preCheckpoint := &checkpoint.Checkpoint{
		RunID:     "slice-test",
		Superstep: 1,
		State: map[string]any{
			"test_messages": []string{"restored1", "restored2"},
		},
		Committed: true,
		Timestamp: time.Now(),
	}
	if err := checkpointer.Save(context.Background(), preCheckpoint); err != nil {
		t.Fatalf("Failed to pre-save checkpoint: %v", err)
	}

	var capturedMessages []string

	g := graph.New(messagesKey)
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedMessages = graph.GetList(scope, messagesKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")
	g.WithCheckpointer(checkpointer, "slice-test")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Resume should restore the checkpoint state (not overwrite with empty input)
	for _, err := range compiled.Resume(context.Background(), "slice-test") {
		if err != nil {
			t.Fatalf("Resume failed: %v", err)
		}
	}

	// Should have restored messages, not empty
	if len(capturedMessages) != 2 {
		t.Errorf("Expected 2 restored messages, got %d: %v", len(capturedMessages), capturedMessages)
	}
}

// ====================
// BSP Slice Merging Tests
// ====================

func TestBSPSliceMergingAcrossSupersteps(t *testing.T) {
	// Test that slices are properly merged across supersteps
	tagsKey := graph.NewListKey[string]("tags")
	var finalTags []string

	g := graph.New(tagsKey)

	g.Node("add1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(tagsKey, []string{"a", "b"}).To("add2")
	}, "add2")

	g.Node("add2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		// Should see tags from previous superstep
		existing := graph.GetList(scope, tagsKey)
		if len(existing) != 2 {
			return graph.Fail(errors.New("expected 2 tags from previous superstep"))
		}
		return graph.Set(tagsKey, []string{"c", "d"}).To("read")
	}, "read")

	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		finalTags = graph.GetList(scope, tagsKey)
		return graph.To(graph.END)
	}, graph.END)

	g.Start("add1")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Should have all 4 tags
	if len(finalTags) != 4 {
		t.Fatalf("Expected 4 tags, got %d: %v", len(finalTags), finalTags)
	}
	expected := []string{"a", "b", "c", "d"}
	for i, exp := range expected {
		if finalTags[i] != exp {
			t.Errorf("Tag %d: expected '%s', got '%s'", i, exp, finalTags[i])
		}
	}
}

func TestBSPParallelSliceAppends(t *testing.T) {
	// Test that parallel nodes can append to the same list
	resultsKey := graph.NewListKey[string]("results")
	var finalResults []string
	var mu sync.Mutex

	g := graph.New(resultsKey)

	g.Node("start", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("worker1", "worker2", "worker3")
	}, "worker1", "worker2", "worker3")

	createWorker := func(name string) graph.NodeFunc {
		return func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
			return graph.Set(resultsKey, []string{name}).To("collect")
		}
	}

	g.Node("worker1", createWorker("w1"), "collect")
	g.Node("worker2", createWorker("w2"), "collect")
	g.Node("worker3", createWorker("w3"), "collect")

	g.Node("collect", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		mu.Lock()
		finalResults = graph.GetList(scope, resultsKey)
		mu.Unlock()
		return graph.To(graph.END)
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// Should have all 3 worker results (order may vary due to parallelism)
	if len(finalResults) != 3 {
		t.Fatalf("Expected 3 results, got %d: %v", len(finalResults), finalResults)
	}

	// Check all workers contributed
	hasW1, hasW2, hasW3 := false, false, false
	for _, r := range finalResults {
		switch r {
		case "w1":
			hasW1 = true
		case "w2":
			hasW2 = true
		case "w3":
			hasW3 = true
		}
	}
	if !hasW1 || !hasW2 || !hasW3 {
		t.Errorf("Missing worker results: w1=%v, w2=%v, w3=%v, results=%v", hasW1, hasW2, hasW3, finalResults)
	}
}

// ====================
// Typed Slice Handling Tests
// ====================

func TestTypedSliceAppend(t *testing.T) {
	type Message struct {
		Role    string
		Content string
	}
	messagesKey := graph.NewListKey[Message]("test_typed_messages")
	var capturedMessages []Message

	g := graph.New(messagesKey)
	g.Node("add", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(messagesKey, []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		}).To("read")
	}, "read")
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedMessages = graph.GetList(scope, messagesKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("add")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if len(capturedMessages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != "user" || capturedMessages[0].Content != "hello" {
		t.Errorf("First message mismatch: %+v", capturedMessages[0])
	}
	if capturedMessages[1].Role != "assistant" || capturedMessages[1].Content != "hi there" {
		t.Errorf("Second message mismatch: %+v", capturedMessages[1])
	}
}

func TestIntSliceAppend(t *testing.T) {
	numbersKey := graph.NewListKey[int]("numbers")
	var capturedNumbers []int

	g := graph.New(numbersKey)
	g.Node("add", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(numbersKey, []int{1, 2, 3}).To("read")
	}, "read")
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedNumbers = graph.GetList(scope, numbersKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("add")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if len(capturedNumbers) != 3 {
		t.Fatalf("Expected 3 numbers, got %d", len(capturedNumbers))
	}
	for i, exp := range []int{1, 2, 3} {
		if capturedNumbers[i] != exp {
			t.Errorf("Number %d: expected %d, got %d", i, exp, capturedNumbers[i])
		}
	}
}

// ====================
// Map Input Handling Tests
// ====================

func TestMapInputHandling(t *testing.T) {
	dataKey := graph.NewKey[map[string]int]("data")
	var capturedData map[string]int

	g := graph.New(dataKey)
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedData = graph.Get(scope, dataKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	input := map[string]int{"a": 1, "b": 2}
	for _, err := range compiled.Run(context.Background(), nil,
		graph.WithInitialValue(dataKey, input),
	) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	if len(capturedData) != 2 {
		t.Errorf("Expected 2 entries, got %d: %v", len(capturedData), capturedData)
	}
	if capturedData["a"] != 1 || capturedData["b"] != 2 {
		t.Errorf("Map mismatch: %v", capturedData)
	}
}

func TestResumePreservesCheckpointState(t *testing.T) {
	dataKey := graph.NewKey[map[string]int]("data")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	runID := "map-test"

	// Pre-save a checkpoint with data
	preCheckpoint := &checkpoint.Checkpoint{
		RunID:     runID,
		Superstep: 1,
		State: map[string]any{
			"data": map[string]int{"saved": 100},
		},
		Committed: true,
		Timestamp: time.Now(),
	}
	if err := checkpointer.Save(context.Background(), preCheckpoint); err != nil {
		t.Fatalf("Failed to pre-save checkpoint: %v", err)
	}

	var capturedData map[string]int

	g := graph.New(dataKey)
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedData = graph.Get(scope, dataKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")
	g.WithCheckpointer(checkpointer, runID)

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Use Resume() - should NOT overwrite checkpoint state with zero input
	// Uses auto-restore by default when no WithCheckpoint option is provided
	for _, err := range compiled.Resume(context.Background(), runID) {
		if err != nil {
			t.Fatalf("Resume failed: %v", err)
		}
	}

	// Should have restored data
	if capturedData["saved"] != 100 {
		t.Errorf("Expected restored data with saved=100, got %v", capturedData)
	}
}

func TestRunWithNilInputOverwritesState(t *testing.T) {
	// This test verifies that Run() with nil input DOES overwrite state
	// (as opposed to Resume() which preserves it)
	dataKey := graph.NewKey[map[string]int]("data")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Pre-save a checkpoint with data
	preCheckpoint := &checkpoint.Checkpoint{
		RunID:     "overwrite-test",
		Superstep: 1,
		State: map[string]any{
			"data": map[string]int{"saved": 100},
		},
		Committed: true,
		Timestamp: time.Now(),
	}
	if err := checkpointer.Save(context.Background(), preCheckpoint); err != nil {
		t.Fatalf("Failed to pre-save checkpoint: %v", err)
	}

	var capturedData map[string]int

	g := graph.New(dataKey)
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		capturedData = graph.Get(scope, dataKey)
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")
	g.WithCheckpointer(checkpointer, "overwrite-test")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Use Run() with nil - SHOULD overwrite checkpoint state with nil
	for _, err := range compiled.Run(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	}

	// State should be nil (overwritten by nil input via ReplaceReducer)
	if capturedData != nil {
		t.Errorf("Expected nil data (overwritten), got %v", capturedData)
	}
}

func TestCheckpointStateInjectionPrevented(t *testing.T) {
	// This test verifies that checkpoints with undeclared keys are rejected
	// to prevent state injection attacks via corrupted/malicious checkpoints

	dataKey := graph.NewKey[string]("data")
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	// Pre-save a checkpoint with an INJECTED key not declared in the graph
	maliciousCheckpoint := &checkpoint.Checkpoint{
		RunID:     "injection-test",
		Superstep: 1,
		State: map[string]any{
			"data":         "legitimate",
			"injected_key": "malicious_value", // NOT declared in graph!
		},
		Committed: true,
		Timestamp: time.Now(),
	}
	if err := checkpointer.Save(context.Background(), maliciousCheckpoint); err != nil {
		t.Fatalf("Failed to pre-save checkpoint: %v", err)
	}

	g := graph.New(dataKey)
	g.Node("read", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)
	g.Start("read")
	g.WithCheckpointer(checkpointer, "injection-test")

	compiled, err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Resume with auto-restore - should fail due to unknown key
	var runErr error
	for _, err := range compiled.Resume(context.Background(), "injection-test") {
		if err != nil {
			runErr = err
			break
		}
	}

	if runErr == nil {
		t.Fatal("Expected error due to unknown checkpoint key, got nil")
	}

	// Verify it's the right error type
	var stateErr *graph.CheckpointStateError
	if !errors.As(runErr, &stateErr) {
		t.Fatalf("Expected CheckpointStateError, got %T: %v", runErr, runErr)
	}

	// Verify the unknown key is reported
	found := false
	for _, key := range stateErr.UnknownKeys {
		if key == "injected_key" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'injected_key' in UnknownKeys, got %v", stateErr.UnknownKeys)
	}
}
