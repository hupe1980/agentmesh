package exec

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// extractRunOptions applies graph.RunOption slice and returns RunOptions.
func extractRunOptions(opts []graph.RunOption) graph.RunOptions {
	return graph.ApplyOptions(opts...)
}

// checkpointWorker manages asynchronous checkpoint saves.
type checkpointWorker struct {
	queue chan *checkpoint.Checkpoint
	wg    sync.WaitGroup
}

// restoreCheckpoint loads and applies a checkpoint if configured.
func (p *Pregel) restoreCheckpoint(ctx context.Context, compiled *compile.CompiledGraph, opts graph.RunOptions) error {
	if opts.Checkpointer == nil || opts.RunID == "" || !opts.AutoRestore {
		return nil
	}

	logger := logging.FromContext(ctx)
	var chkpt *checkpoint.Checkpoint
	var err error

	if opts.ResumeFrom > 0 {
		logger.Info("loading checkpoint at specific superstep",
			"run_id", opts.RunID,
			"superstep", opts.ResumeFrom)
		chkpt, err = opts.Checkpointer.LoadAtSuperstep(ctx, opts.RunID, opts.ResumeFrom)
	} else {
		logger.Info("loading latest checkpoint",
			"run_id", opts.RunID)
		chkpt, err = opts.Checkpointer.Load(ctx, opts.RunID)
	}

	if err != nil {
		logger.Error("failed to load checkpoint",
			"run_id", opts.RunID,
			"error", err)
		return fmt.Errorf("failed to load checkpoint: %w", err)
	}

	if chkpt == nil {
		return nil // No checkpoint to restore
	}

	logger.Info("restoring from checkpoint",
		"run_id", opts.RunID,
		"superstep", chkpt.Superstep)

	// Restore state
	if len(chkpt.State) > 0 {
		// Apply checkpoint state to restore values
		if err := state.ApplyUpdates(ctx, compiled.Manager, chkpt.State); err != nil {
			logger.Error("failed to apply checkpoint state",
				"run_id", opts.RunID,
				"error", err)
			return fmt.Errorf("failed to apply checkpoint state: %w", err)
		}
		logger.Info("restored state from checkpoint",
			"run_id", opts.RunID,
			"state_keys", len(chkpt.State))
	}

	// TODO: Restore messages, completed nodes, paused nodes if needed
	// This requires additional API in compile.CompiledGraph

	logger.Info("checkpoint restored successfully",
		"run_id", opts.RunID,
		"superstep", chkpt.Superstep)

	return nil
}

// saveCheckpoint creates and saves a checkpoint.
func (p *Pregel) saveCheckpoint(ctx context.Context, compiled *compile.CompiledGraph, opts graph.RunOptions, superstep int64, worker *checkpointWorker) {
	if opts.Checkpointer == nil || opts.RunID == "" {
		return
	}

	// Check if we should save based on interval
	if opts.CheckpointInterval > 0 && superstep%int64(opts.CheckpointInterval) != 0 {
		return
	}

	logger := logging.FromContext(ctx)
	logger.Debug("saving checkpoint",
		"run_id", opts.RunID,
		"superstep", superstep)

	// Create checkpoint using Manager's Snapshot
	vsnap, err := compiled.Manager.Snapshot(ctx, map[string]string{
		"run_id":    opts.RunID,
		"superstep": fmt.Sprintf("%d", superstep),
	})
	if err != nil {
		// Log error but don't fail the superstep
		// TODO: Consider making checkpointing errors more visible
		_ = err
	}

	chkpt := &checkpoint.Checkpoint{
		RunID:     opts.RunID,
		Superstep: superstep,
		Timestamp: vsnap.Timestamp,
		Version:   0, // Manager handles versioning internally
		State:     vsnap.Data,
		// TODO: Add messages, completed nodes, paused nodes
		Messages:       []message.Message{},
		CompletedNodes: []string{},
		PausedNodes:    []string{},
		Metadata:       map[string]any{},
	}

	// Queue or save checkpoint
	if worker != nil && worker.queue != nil {
		// Block if queue is full - applies backpressure to prevent checkpoint loss
		logger.Debug("queueing checkpoint for async save",
			"run_id", opts.RunID,
			"superstep", superstep)

		// This blocks until space is available in the queue
		worker.queue <- chkpt

		logger.Debug("checkpoint queued successfully",
			"run_id", opts.RunID,
			"superstep", superstep)
	} else {
		// No async worker - save synchronously
		if err := p.saveCheckpointSync(ctx, opts, chkpt); err != nil {
			logger.Error("synchronous checkpoint save failed",
				"run_id", opts.RunID,
				"superstep", superstep,
				"error", err)
		}
	}
}

// saveCheckpointSync saves a checkpoint synchronously.
func (p *Pregel) saveCheckpointSync(ctx context.Context, opts graph.RunOptions, chkpt *checkpoint.Checkpoint) error {
	logger := logging.FromContext(ctx)

	// Use context.WithoutCancel to ensure checkpoint completes even if main context is cancelled
	saveCtx := context.WithoutCancel(ctx)

	if err := opts.Checkpointer.Save(saveCtx, chkpt); err != nil {
		logger.Error("failed to save checkpoint",
			"run_id", opts.RunID,
			"superstep", chkpt.Superstep,
			"error", err)
		if opts.FailOnCheckpointErr {
			return fmt.Errorf("checkpoint save failed at superstep %d: %w", chkpt.Superstep, err)
		}
		return nil // Error logged but not propagated
	}

	logger.Info("checkpoint saved successfully",
		"run_id", opts.RunID,
		"superstep", chkpt.Superstep)
	return nil
}

// startCheckpointWorker starts an asynchronous checkpoint worker.
func (p *Pregel) startCheckpointWorker(ctx context.Context, opts graph.RunOptions) *checkpointWorker {
	if opts.Checkpointer == nil || opts.RunID == "" {
		return nil
	}

	// CheckpointQueueSize of 0 means synchronous checkpoints only
	if opts.CheckpointQueueSize <= 0 {
		return nil
	}

	logger := logging.FromContext(ctx)
	logger.Debug("starting async checkpoint worker",
		"run_id", opts.RunID,
		"queue_size", opts.CheckpointQueueSize)

	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, opts.CheckpointQueueSize),
	}

	// Use context.WithoutCancel to ensure worker completes all queued checkpoints
	saveCtx := context.WithoutCancel(ctx)

	worker.wg.Add(1)
	go func() {
		defer worker.wg.Done()
		logger := logging.FromContext(saveCtx)

		for chkpt := range worker.queue {
			if chkpt == nil {
				continue
			}

			logger.Debug("processing checkpoint from queue",
				"run_id", chkpt.RunID,
				"superstep", chkpt.Superstep)

			if err := opts.Checkpointer.Save(saveCtx, chkpt); err != nil {
				logger.Error("async checkpoint save failed",
					"run_id", chkpt.RunID,
					"superstep", chkpt.Superstep,
					"error", err)
			} else {
				logger.Info("async checkpoint saved successfully",
					"run_id", chkpt.RunID,
					"superstep", chkpt.Superstep)
			}
		}

		logger.Debug("checkpoint worker stopped", "run_id", opts.RunID)
	}()

	return worker
}

// stopCheckpointWorker stops the checkpoint worker and waits for completion.
func (p *Pregel) stopCheckpointWorker(worker *checkpointWorker) {
	if worker == nil || worker.queue == nil {
		return
	}

	close(worker.queue)
	worker.wg.Wait()
}
