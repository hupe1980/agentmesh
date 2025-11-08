package graph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/message"
)

// PregelExecutor implements the Executor interface using the Pregel BSP execution model.
// It wraps the internal graphRuntime to provide a clean execution API.
type PregelExecutor struct {
	cg           *CompiledGraph
	stateManager StateManager
	pausedNodes  map[string]bool
	pausedMutex  sync.RWMutex
	stats        ExecutionStats
	statsMutex   sync.RWMutex
	// superstep    int64
}

// NewPregelExecutor creates a new Pregel-based executor.
func NewPregelExecutor(cg *CompiledGraph, stateManager StateManager) *PregelExecutor {
	return &PregelExecutor{
		cg:           cg,
		stateManager: stateManager,
		pausedNodes:  make(map[string]bool),
		stats: ExecutionStats{
			StartedAt: nil,
		},
	}
}

// Execute runs the graph to completion and returns the final result.
func (e *PregelExecutor) Execute(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (*InvokeResult, error) {
	e.updateStats(func(s *ExecutionStats) {
		s.StartedAt = time.Now()
	})

	// Apply initial messages to state
	if len(initialMessages) > 0 {
		e.stateManager.ApplyUpdates(nil, initialMessages)
	}

	runOpts := e.buildRunOptions(options)
	done := make(chan struct{})
	defer close(done)

	ctxWithCancel, cancel := context.WithCancel(ctx)
	defer cancel()

	runtime := newPregelRuntime(e.cg, ctxWithCancel, cancel, runOpts, nil, done)
	runtime.checkpointQueue = make(chan *Checkpoint, 1)
	runtime.checkpointWG.Add(1)

	// Start checkpoint processor if needed
	if options.Checkpointer != nil && options.CheckpointInterval > 0 {
		go e.processCheckpoints(ctx, runtime.checkpointQueue, &runtime.checkpointWG, options.Checkpointer)
	} else {
		runtime.checkpointWG.Done()
	}

	err := runtime.run()

	// Wait for checkpoint processor to finish
	close(runtime.checkpointQueue)
	runtime.checkpointWG.Wait()

	e.updateStats(func(s *ExecutionStats) {
		now := time.Now()
		s.CompletedAt = &now
		s.Supersteps = e.cg.CurrentSuperstep()
	})

	if err != nil {
		return nil, err
	}

	// Build result from final state
	result := &InvokeResult{
		Messages: e.stateManager.MessagesSnapshot(),
		State:    e.stateManager.GetAll(),
	}

	return result, nil
}

// Stream executes the graph with real-time event streaming.
func (e *PregelExecutor) Stream(ctx context.Context, initialMessages []message.Message, options ExecuteOptions) (<-chan interface{}, <-chan error) {
	eventChan := make(chan interface{}, 100)
	errChan := make(chan error, 1)

	go func() {
		defer close(eventChan)
		defer close(errChan)

		e.updateStats(func(s *ExecutionStats) {
			s.StartedAt = time.Now()
		})

		// Apply initial messages to state
		if len(initialMessages) > 0 {
			e.stateManager.ApplyUpdates(nil, initialMessages)
		}

		runOpts := e.buildRunOptions(options)
		done := make(chan struct{})
		defer close(done)

		ctxWithCancel, cancel := context.WithCancel(ctx)
		defer cancel()

		streamChan := make(chan StreamEvent, 100)
		runtime := newPregelRuntime(e.cg, ctxWithCancel, cancel, runOpts, streamChan, done)
		runtime.checkpointQueue = make(chan *Checkpoint, 1)
		runtime.checkpointWG.Add(1)

		// Start checkpoint processor if needed
		if options.Checkpointer != nil && options.CheckpointInterval > 0 {
			go e.processCheckpoints(ctx, runtime.checkpointQueue, &runtime.checkpointWG, options.Checkpointer)
		} else {
			runtime.checkpointWG.Done()
		}

		// Forward events from internal stream to external stream
		go func() {
			for event := range streamChan {
				eventChan <- event
			}
		}()

		err := runtime.run()

		// Wait for checkpoint processor to finish
		close(runtime.checkpointQueue)
		runtime.checkpointWG.Wait()

		e.updateStats(func(s *ExecutionStats) {
			now := time.Now()
			s.CompletedAt = &now
			s.Supersteps = e.cg.CurrentSuperstep()
		})

		if err != nil {
			errChan <- err
		}
	}()

	return eventChan, errChan
}

// Pause pauses execution before the specified node.
func (e *PregelExecutor) Pause(nodeName string) {
	e.pausedMutex.Lock()
	defer e.pausedMutex.Unlock()
	e.pausedNodes[nodeName] = true
}

// Resume resumes execution of a paused node.
func (e *PregelExecutor) Resume(nodeName string) {
	e.pausedMutex.Lock()
	defer e.pausedMutex.Unlock()
	delete(e.pausedNodes, nodeName)
}

// IsPaused returns whether the specified node is currently paused.
func (e *PregelExecutor) IsPaused(nodeName string) bool {
	e.pausedMutex.RLock()
	defer e.pausedMutex.RUnlock()
	return e.pausedNodes[nodeName]
}

// CurrentSuperstep returns the current superstep number.
func (e *PregelExecutor) CurrentSuperstep() int64 {
	return e.cg.CurrentSuperstep()
}

// Stats returns execution statistics.
func (e *PregelExecutor) Stats() ExecutionStats {
	e.statsMutex.RLock()
	defer e.statsMutex.RUnlock()
	return e.stats
}

// buildRunOptions converts ExecuteOptions to internal runOptions.
func (e *PregelExecutor) buildRunOptions(options ExecuteOptions) runOptions {
	return runOptions{
		maxIterations:      options.MaxIterations,
		maxConcurrency:     options.MaxWorkers,
		checkpointInterval: options.CheckpointInterval,
		runID:              options.RunID,
		initialSuperstep:   e.cg.CurrentSuperstep(),
	}
}

// processCheckpoints handles checkpoint persistence in a background goroutine.
func (e *PregelExecutor) processCheckpoints(ctx context.Context, checkpointQueue <-chan *Checkpoint, wg *sync.WaitGroup, checkpointer interface {
	Save(ctx context.Context, checkpoint any) error
	Load(ctx context.Context, runID string) (any, error)
}) {
	defer wg.Done()

	for checkpoint := range checkpointQueue {
		if err := checkpointer.Save(ctx, checkpoint); err != nil {
			// Log error but don't fail execution
			fmt.Printf("checkpoint save failed: %v\n", err)
		}
	}
}

// updateStats safely updates execution statistics.
func (e *PregelExecutor) updateStats(fn func(*ExecutionStats)) {
	e.statsMutex.Lock()
	defer e.statsMutex.Unlock()
	fn(&e.stats)
}

// Verify PregelExecutor implements Executor interface.
var _ Executor = (*PregelExecutor)(nil)
