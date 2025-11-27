package viz

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunController_NewRunController(t *testing.T) {
	runnable := &mockRunnable{}
	rc := NewRunController("run-1", runnable)

	assert.NotNil(t, rc)
	assert.Equal(t, "run-1", rc.runID)
	assert.Equal(t, runnable, rc.runnable)
	assert.NotNil(t, rc.ctx)
	assert.NotNil(t, rc.cancel)
	assert.Equal(t, StatusPending, rc.Status())
}

func TestRunController_Context(t *testing.T) {
	t.Run("context is not nil", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		ctx := rc.Context()
		assert.NotNil(t, ctx)
	})

	t.Run("context is not done initially", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		ctx := rc.Context()
		
		select {
		case <-ctx.Done():
			t.Fatal("context should not be done initially")
		default:
			// Context is not done, as expected
		}
	})

	t.Run("context is done after cancel", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		ctx := rc.Context()
		
		rc.Cancel()

		select {
		case <-ctx.Done():
			// Context is done, as expected
		case <-time.After(100 * time.Millisecond):
			t.Fatal("context should be done after cancel")
		}

		assert.Equal(t, context.Canceled, ctx.Err())
	})
}

func TestRunController_Cancel(t *testing.T) {
	t.Run("cancel updates status", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		
		rc.Cancel()
		
		assert.Equal(t, StatusCanceled, rc.Status())
	})

	t.Run("cancel propagates to context", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		ctx := rc.Context()
		
		rc.Cancel()
		
		select {
		case <-ctx.Done():
			// Expected
		case <-time.After(100 * time.Millisecond):
			t.Fatal("context should be canceled")
		}
	})

	t.Run("multiple cancels are safe", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		
		rc.Cancel()
		rc.Cancel()
		rc.Cancel()
		
		assert.Equal(t, StatusCanceled, rc.Status())
	})

	t.Run("cancel from different status", func(t *testing.T) {
		statuses := []RunStatus{StatusPending, StatusRunning, StatusPaused}
		
		for _, initialStatus := range statuses {
			rc := NewRunController("run-1", &mockRunnable{})
			rc.SetStatus(initialStatus)
			
			rc.Cancel()
			
			assert.Equal(t, StatusCanceled, rc.Status())
		}
	})
}

func TestRunController_Status(t *testing.T) {
	t.Run("initial status is pending", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		assert.Equal(t, StatusPending, rc.Status())
	})

	t.Run("status after set", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		
		rc.SetStatus(StatusRunning)
		assert.Equal(t, StatusRunning, rc.Status())
		
		rc.SetStatus(StatusCompleted)
		assert.Equal(t, StatusCompleted, rc.Status())
	})
}

func TestRunController_SetStatus(t *testing.T) {
	t.Run("set all valid statuses", func(t *testing.T) {
		statuses := []RunStatus{
			StatusPending,
			StatusRunning,
			StatusPaused,
			StatusCompleted,
			StatusFailed,
			StatusCanceled,
		}
		
		for _, status := range statuses {
			rc := NewRunController("run-1", &mockRunnable{})
			rc.SetStatus(status)
			assert.Equal(t, status, rc.Status())
		}
	})

	t.Run("status transitions", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		
		// Typical lifecycle
		rc.SetStatus(StatusRunning)
		assert.Equal(t, StatusRunning, rc.Status())
		
		rc.SetStatus(StatusPaused)
		assert.Equal(t, StatusPaused, rc.Status())
		
		rc.SetStatus(StatusRunning)
		assert.Equal(t, StatusRunning, rc.Status())
		
		rc.SetStatus(StatusCompleted)
		assert.Equal(t, StatusCompleted, rc.Status())
	})
}

func TestRunController_ThreadSafety(t *testing.T) {
	rc := NewRunController("run-1", &mockRunnable{})

	done := make(chan bool)
	iterations := 100

	// Concurrent status reads
	go func() {
		for i := 0; i < iterations; i++ {
			rc.Status()
		}
		done <- true
	}()

	// Concurrent status writes
	go func() {
		for i := 0; i < iterations; i++ {
			rc.SetStatus(StatusRunning)
		}
		done <- true
	}()

	// Concurrent context access
	go func() {
		for i := 0; i < iterations; i++ {
			rc.Context()
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

func TestRunController_ContextCancellationPropagation(t *testing.T) {
	t.Run("context can be used for goroutine cancellation", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		ctx := rc.Context()

		workDone := make(chan bool)
		workCanceled := make(chan bool)

		// Simulate work that respects context
		go func() {
			select {
			case <-ctx.Done():
				workCanceled <- true
			case <-time.After(1 * time.Second):
				workDone <- true
			}
		}()

		// Cancel after a short delay
		time.Sleep(10 * time.Millisecond)
		rc.Cancel()

		// Work should be canceled, not completed
		select {
		case <-workCanceled:
			// Expected
		case <-workDone:
			t.Fatal("work should have been canceled")
		case <-time.After(200 * time.Millisecond):
			t.Fatal("timeout waiting for cancellation")
		}
	})
}

func TestRunStatus_Constants(t *testing.T) {
	// Verify status constants have expected values
	assert.Equal(t, RunStatus("pending"), StatusPending)
	assert.Equal(t, RunStatus("running"), StatusRunning)
	assert.Equal(t, RunStatus("paused"), StatusPaused)
	assert.Equal(t, RunStatus("completed"), StatusCompleted)
	assert.Equal(t, RunStatus("failed"), StatusFailed)
	assert.Equal(t, RunStatus("canceled"), StatusCanceled)
}

func TestRunController_CancelNilSafety(t *testing.T) {
	t.Run("cancel handles nil cancel function", func(t *testing.T) {
		rc := NewRunController("run-1", &mockRunnable{})
		
		// Manually set cancel to nil to test safety
		rc.mu.Lock()
		rc.cancel = nil
		rc.mu.Unlock()
		
		// Should not panic
		require.NotPanics(t, func() {
			rc.Cancel()
		})
		
		assert.Equal(t, StatusCanceled, rc.Status())
	})
}
