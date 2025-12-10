package pregel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_BasicExecution(t *testing.T) {
	ctx := context.Background()
	var executed atomic.Int32

	pool, err := NewWorkerPool(ctx, WorkerPoolConfig{
		WorkerCount: 2,
	}, func(ctx context.Context, task string) error {
		executed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("NewWorkerPool failed: %v", err)
	}

	// Submit tasks
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		pool.Submit(ctx, name)
	}

	close(pool.tasks)

	if err := pool.Wait(); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if got := executed.Load(); got != 5 {
		t.Errorf("expected 5 tasks executed, got %d", got)
	}
}

func TestWorkerPool_ErrorPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	expectedErr := errors.New("task failed")

	pool, err := NewWorkerPool(ctx, WorkerPoolConfig{
		WorkerCount: 1,
		Cancel:      cancel, // Cancel context on error so Submit unblocks
	}, func(ctx context.Context, task string) error {
		if task == "fail" {
			return expectedErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NewWorkerPool failed: %v", err)
	}

	// Submit tasks - the third one may not submit due to error+cancel
	pool.Submit(ctx, "ok")
	pool.Submit(ctx, "fail")
	pool.Submit(ctx, "ok2") // May return false due to context cancellation

	close(pool.tasks)

	err = pool.Wait()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestWorkerPool_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Bool

	pool, err := NewWorkerPool(ctx, WorkerPoolConfig{
		WorkerCount: 1,
		Cancel:      cancel,
	}, func(ctx context.Context, task string) error {
		started.Store(true)
		// Simulate long-running task
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	})
	if err != nil {
		t.Fatalf("NewWorkerPool failed: %v", err)
	}

	pool.Submit(ctx, "blocking")

	// Wait for task to start
	for !started.Load() {
		time.Sleep(time.Millisecond)
	}

	// Cancel context
	cancel()
	close(pool.tasks)

	err = pool.Wait()
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestWorkerPool_SubmitAll(t *testing.T) {
	ctx := context.Background()
	var executed atomic.Int32

	pool, err := NewWorkerPool(ctx, WorkerPoolConfig{
		WorkerCount: 3,
	}, func(ctx context.Context, task string) error {
		executed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("NewWorkerPool failed: %v", err)
	}

	names := []string{"a", "b", "c", "d", "e", "f", "g"}
	pool.SubmitAll(ctx, names)
	close(pool.tasks)

	if err := pool.Wait(); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if got := executed.Load(); got != int32(len(names)) {
		t.Errorf("expected %d tasks executed, got %d", len(names), got)
	}
}

func TestWorkerPool_Shutdown(t *testing.T) {
	ctx := context.Background()
	blockCh := make(chan struct{})

	pool, err := NewWorkerPool(ctx, WorkerPoolConfig{
		WorkerCount: 1,
	}, func(ctx context.Context, task string) error {
		<-blockCh // Block forever unless signaled
		return nil
	})
	if err != nil {
		t.Fatalf("NewWorkerPool failed: %v", err)
	}

	pool.Submit(ctx, "blocking")

	// Shutdown should time out since worker is blocked
	err = pool.Shutdown(10 * time.Millisecond)
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Errorf("expected ErrShutdownTimeout, got %v", err)
	}

	// Unblock worker
	close(blockCh)

	// Now wait should succeed
	pool.Wait() //nolint:errcheck // Ignored: shutdown already returned error
}

func TestWorkerPool_FirstErrorWins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstErr := errors.New("first error")
	secondErr := errors.New("second error")

	// Use single worker to guarantee order
	pool, err := NewWorkerPool(ctx, WorkerPoolConfig{
		WorkerCount: 1,
		Cancel:      cancel, // Cancel context on error
	}, func(ctx context.Context, task string) error {
		if task == "first" {
			return firstErr
		}
		if task == "second" {
			return secondErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NewWorkerPool failed: %v", err)
	}

	pool.Submit(ctx, "first")
	pool.Submit(ctx, "second") // May not submit due to cancel
	close(pool.tasks)

	err = pool.Wait()
	if !errors.Is(err, firstErr) {
		t.Errorf("expected first error, got %v", err)
	}
}
