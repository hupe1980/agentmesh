package pregel

import (
	"context"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/internal/safego"
	"github.com/hupe1980/agentmesh/pkg/quota"
)

// WorkerPool manages a pool of worker goroutines for parallel task execution.
// It provides explicit lifecycle management with graceful shutdown support.
//
// The pool is designed for short-lived, superstep-scoped execution where:
//   - Workers are spawned at pool creation
//   - Tasks are submitted via Submit()
//   - Shutdown waits for all workers to complete
//
// Thread-safety: All methods are safe for concurrent use.
type WorkerPool struct {
	tasks        chan string
	wg           sync.WaitGroup
	once         sync.Once
	err          error
	cancel       context.CancelFunc
	quotaManager *quota.Manager
	workerCount  int
}

// WorkerPoolConfig configures a WorkerPool.
type WorkerPoolConfig struct {
	// WorkerCount is the number of worker goroutines to spawn.
	WorkerCount int

	// QuotaManager optionally tracks goroutine quotas.
	QuotaManager *quota.Manager

	// Cancel is called when the first error occurs.
	Cancel context.CancelFunc
}

// WorkerFunc is the function executed by each worker for a task.
type WorkerFunc func(ctx context.Context, task string) error

// NewWorkerPool creates a new worker pool and starts worker goroutines.
// Workers immediately begin waiting for tasks on the internal channel.
//
// The pool must be shut down via Shutdown() to release resources.
func NewWorkerPool(ctx context.Context, cfg WorkerPoolConfig, workerFn WorkerFunc) (*WorkerPool, error) {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}

	p := &WorkerPool{
		tasks:        make(chan string),
		cancel:       cfg.Cancel,
		quotaManager: cfg.QuotaManager,
		workerCount:  cfg.WorkerCount,
	}

	// Start workers
	for range cfg.WorkerCount {
		if err := p.startWorker(ctx, workerFn); err != nil {
			// Failed to start all workers - shut down gracefully
			// Ignore shutdown error as we're already in an error path
			_ = p.Shutdown(time.Second)
			return nil, err
		}
	}

	return p, nil
}

// startWorker spawns a single worker goroutine with quota management and panic recovery.
func (p *WorkerPool) startWorker(ctx context.Context, workerFn WorkerFunc) error {
	// Acquire goroutine quota before spawning
	if p.quotaManager != nil {
		if err := p.quotaManager.AcquireGoroutine(ctx); err != nil {
			return err
		}
	}

	p.wg.Add(1)

	safego.Go(func() error {
		defer func() {
			p.wg.Done()
			if p.quotaManager != nil {
				p.quotaManager.ReleaseGoroutine()
			}
		}()

		p.workerLoop(ctx, workerFn)
		return nil
	}, func(err error) {
		p.recordError(err)
	})

	return nil
}

// workerLoop is the main loop for a worker goroutine.
func (p *WorkerPool) workerLoop(ctx context.Context, workerFn WorkerFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-p.tasks:
			if !ok {
				return // Channel closed, shutdown
			}

			if err := workerFn(ctx, task); err != nil {
				p.recordError(err)
				return
			}
		}
	}
}

// recordError records the first error and cancels the context.
func (p *WorkerPool) recordError(err error) {
	if err == nil {
		return
	}
	p.once.Do(func() {
		p.err = err
		if p.cancel != nil {
			p.cancel()
		}
	})
}

// Submit sends a task to the worker pool.
// Returns false if the context is cancelled or the pool is shut down.
func (p *WorkerPool) Submit(ctx context.Context, task string) bool {
	select {
	case <-ctx.Done():
		return false
	case p.tasks <- task:
		return true
	}
}

// SubmitAll sends multiple tasks to the worker pool.
// Stops early if the context is cancelled.
func (p *WorkerPool) SubmitAll(ctx context.Context, tasks []string) {
	for _, task := range tasks {
		if !p.Submit(ctx, task) {
			return
		}
	}
}

// Shutdown closes the task channel and waits for all workers to complete.
// Returns the first error that occurred during execution, or ErrShutdownTimeout
// if workers don't finish within the timeout.
//
// After Shutdown returns, the pool cannot be reused.
func (p *WorkerPool) Shutdown(timeout time.Duration) error {
	close(p.tasks)

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return p.err
	case <-time.After(timeout):
		return ErrShutdownTimeout
	}
}

// Wait blocks until all workers complete.
// Unlike Shutdown, this does not close the task channel.
// Use this when you want to wait for completion after manually closing tasks.
func (p *WorkerPool) Wait() error {
	p.wg.Wait()
	return p.err
}

// WorkerCount returns the number of workers in the pool.
func (p *WorkerPool) WorkerCount() int {
	return p.workerCount
}
