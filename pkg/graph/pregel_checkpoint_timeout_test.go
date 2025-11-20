package graph

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCheckpointer simulates checkpoint saves with configurable delays
type mockCheckpointer struct {
	saveDelay   time.Duration
	saveCalled  int
	mu          sync.Mutex
	blockOnSave chan struct{} // Optional: blocks save until closed
}

func (m *mockCheckpointer) Save(ctx context.Context, cp *checkpoint.Checkpoint) error {
	m.mu.Lock()
	m.saveCalled++
	m.mu.Unlock()

	if m.blockOnSave != nil {
		<-m.blockOnSave
	}

	if m.saveDelay > 0 {
		select {
		case <-time.After(m.saveDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (m *mockCheckpointer) Load(ctx context.Context, runID string) (*checkpoint.Checkpoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCheckpointer) List(ctx context.Context) ([]*checkpoint.Checkpoint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockCheckpointer) Delete(ctx context.Context, runID string) error {
	return fmt.Errorf("not implemented")
}

func (m *mockCheckpointer) GetSaveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveCalled
}

func TestStopCheckpointWorker_GracefulShutdown(t *testing.T) {
	ctx := context.Background()
	executor := &PregelExecutor[any, any]{}

	// Create a worker with fast checkpoint saves
	mockCP := &mockCheckpointer{saveDelay: 10 * time.Millisecond}

	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, 5),
	}
	worker.wg.Add(1)

	// Start worker goroutine
	go func() {
		defer worker.wg.Done()
		for cp := range worker.queue {
			_ = mockCP.Save(ctx, cp)
		}
	}()

	// Queue a few checkpoints
	for i := 0; i < 3; i++ {
		worker.queue <- &checkpoint.Checkpoint{RunID: fmt.Sprintf("test-%d", i)}
	}

	// Stop worker with reasonable timeout
	timeout := 500 * time.Millisecond
	start := time.Now()
	err := executor.stopCheckpointWorker(ctx, worker, timeout)
	elapsed := time.Since(start)

	// Assertions
	assert.NoError(t, err, "Worker should stop gracefully without error")
	assert.Less(t, elapsed, timeout, "Should complete well before timeout")
	assert.Equal(t, 3, mockCP.GetSaveCount(), "All checkpoints should be processed")
}

func TestStopCheckpointWorker_TimeoutExceeded(t *testing.T) {
	ctx := context.Background()
	executor := &PregelExecutor[any, any]{}

	// Create a worker with blocking checkpoint saves
	blockChan := make(chan struct{})
	mockCP := &mockCheckpointer{blockOnSave: blockChan}

	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, 5),
	}
	worker.wg.Add(1)

	// Start worker goroutine
	go func() {
		defer worker.wg.Done()
		for cp := range worker.queue {
			_ = mockCP.Save(ctx, cp)
		}
	}()

	// Queue a checkpoint that will block
	worker.queue <- &checkpoint.Checkpoint{RunID: "blocked"}

	// Stop worker with short timeout
	timeout := 100 * time.Millisecond
	start := time.Now()
	err := executor.stopCheckpointWorker(ctx, worker, timeout)
	elapsed := time.Since(start)

	// Assertions
	require.Error(t, err, "Should return timeout error")
	assert.Contains(t, err.Error(), "checkpoint worker did not stop within")
	assert.GreaterOrEqual(t, elapsed, timeout, "Should wait for full timeout")
	assert.Less(t, elapsed, timeout+50*time.Millisecond, "Should not wait significantly longer than timeout")

	// Cleanup: unblock the worker
	close(blockChan)
	time.Sleep(50 * time.Millisecond)
}

func TestStopCheckpointWorker_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	executor := &PregelExecutor[any, any]{}

	// Create a worker with blocking checkpoint saves
	blockChan := make(chan struct{})
	mockCP := &mockCheckpointer{blockOnSave: blockChan}

	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, 5),
	}
	worker.wg.Add(1)

	// Start worker goroutine
	go func() {
		defer worker.wg.Done()
		for cp := range worker.queue {
			_ = mockCP.Save(context.Background(), cp) // Use separate context
		}
	}()

	// Queue a checkpoint that will block
	worker.queue <- &checkpoint.Checkpoint{RunID: "blocked"}

	// Cancel context after short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Stop worker with long timeout but cancelled context
	timeout := 5 * time.Second
	start := time.Now()
	err := executor.stopCheckpointWorker(ctx, worker, timeout)
	elapsed := time.Since(start)

	// Assertions
	require.Error(t, err, "Should return context cancellation error")
	assert.Contains(t, err.Error(), "checkpoint worker stop cancelled")
	assert.Less(t, elapsed, 200*time.Millisecond, "Should return quickly after context cancellation")

	// Cleanup: unblock the worker
	close(blockChan)
	time.Sleep(50 * time.Millisecond)
}

func TestStopCheckpointWorker_NoTimeout(t *testing.T) {
	ctx := context.Background()
	executor := &PregelExecutor[any, any]{}

	// Create a worker with slow checkpoint saves
	mockCP := &mockCheckpointer{saveDelay: 200 * time.Millisecond}

	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, 5),
	}
	worker.wg.Add(1)

	// Start worker goroutine
	go func() {
		defer worker.wg.Done()
		for cp := range worker.queue {
			_ = mockCP.Save(ctx, cp)
		}
	}()

	// Queue a checkpoint
	worker.queue <- &checkpoint.Checkpoint{RunID: "slow"}

	// Stop worker with zero timeout (wait indefinitely)
	timeout := time.Duration(0)
	err := executor.stopCheckpointWorker(ctx, worker, timeout)

	// Assertions
	assert.NoError(t, err, "Should wait indefinitely and complete without error")
	assert.Equal(t, 1, mockCP.GetSaveCount(), "Checkpoint should be processed")
}

func TestStopCheckpointWorker_NilWorker(t *testing.T) {
	ctx := context.Background()
	executor := &PregelExecutor[any, any]{}

	// Stop with nil worker
	err := executor.stopCheckpointWorker(ctx, nil, 1*time.Second)
	assert.NoError(t, err, "Should handle nil worker gracefully")
}

func TestStopCheckpointWorker_NilQueue(t *testing.T) {
	ctx := context.Background()
	executor := &PregelExecutor[any, any]{}

	// Worker with nil queue
	worker := &checkpointWorker{queue: nil}

	err := executor.stopCheckpointWorker(ctx, worker, 1*time.Second)
	assert.NoError(t, err, "Should handle nil queue gracefully")
}

func TestStopCheckpointWorker_EmptyQueue(t *testing.T) {
	ctx := context.Background()
	executor := &PregelExecutor[any, any]{}

	// Create a worker with no queued checkpoints
	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, 5),
	}
	worker.wg.Add(1)

	// Start worker goroutine
	go func() {
		defer worker.wg.Done()
		for range worker.queue {
			// No-op
		}
	}()

	// Stop worker immediately
	timeout := 1 * time.Second
	start := time.Now()
	err := executor.stopCheckpointWorker(ctx, worker, timeout)
	elapsed := time.Since(start)

	// Assertions
	assert.NoError(t, err, "Should stop immediately with empty queue")
	assert.Less(t, elapsed, 100*time.Millisecond, "Should complete very quickly")
}

func TestStopCheckpointWorker_MultipleCheckpoints(t *testing.T) {
	ctx := context.Background()
	executor := &PregelExecutor[any, any]{}

	// Create a worker with moderate checkpoint save time
	mockCP := &mockCheckpointer{saveDelay: 50 * time.Millisecond}

	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, 20),
	}
	worker.wg.Add(1)

	// Start worker goroutine
	go func() {
		defer worker.wg.Done()
		for cp := range worker.queue {
			_ = mockCP.Save(ctx, cp)
		}
	}()

	// Queue many checkpoints
	checkpointCount := 10
	for i := 0; i < checkpointCount; i++ {
		worker.queue <- &checkpoint.Checkpoint{RunID: fmt.Sprintf("test-%d", i)}
	}

	// Stop worker with sufficient timeout
	timeout := 2 * time.Second
	start := time.Now()
	err := executor.stopCheckpointWorker(ctx, worker, timeout)
	elapsed := time.Since(start)

	// Assertions
	assert.NoError(t, err, "Should process all checkpoints within timeout")
	assert.Less(t, elapsed, timeout, "Should complete before timeout")
	assert.Equal(t, checkpointCount, mockCP.GetSaveCount(), "All checkpoints should be processed")
}

func TestWithCheckpointStopTimeout(t *testing.T) {
	tests := []struct {
		name            string
		timeout         time.Duration
		expectedTimeout time.Duration
	}{
		{
			name:            "custom timeout",
			timeout:         60 * time.Second,
			expectedTimeout: 60 * time.Second,
		},
		{
			name:            "short timeout",
			timeout:         5 * time.Second,
			expectedTimeout: 5 * time.Second,
		},
		{
			name:            "zero timeout (wait indefinitely)",
			timeout:         0,
			expectedTimeout: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &RunOptions{}

			// Apply the option
			WithCheckpointStopTimeout(tt.timeout)(opts)

			assert.Equal(t, tt.expectedTimeout, opts.CheckpointStopTimeout)
		})
	}
}

func TestWithCheckpointStopTimeout_NilOptions(t *testing.T) {
	// Should not panic with nil options
	assert.NotPanics(t, func() {
		WithCheckpointStopTimeout(30 * time.Second)(nil)
	})
}

func TestDefaultRunOptions_CheckpointStopTimeout(t *testing.T) {
	opts := defaultRunOptions()

	// Verify default timeout is set
	assert.Equal(t, 30*time.Second, opts.CheckpointStopTimeout,
		"Default checkpoint stop timeout should be 30 seconds")
}
