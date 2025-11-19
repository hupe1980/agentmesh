package graph

import (
	"context"
	"fmt"
	"sync"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
)

// checkpointWorker manages asynchronous checkpoint saves.
type checkpointWorker struct {
	queue chan *checkpoint.Checkpoint
	wg    sync.WaitGroup
}

// restoreCheckpoint loads and applies a checkpoint if configured.
// Supports WithCheckpoint() option and pending writes application.
func (p *PregelExecutor[I, O]) restoreCheckpoint(ctx context.Context, compiled *Compiled[I, O], opts RunOptions) error {
	chkpt, err := p.loadCheckpoint(ctx, opts)
	if err != nil {
		return err
	}

	if chkpt == nil {
		return nil // No checkpoint to restore
	}

	logger := logging.FromContext(ctx)
	logger.Info("restoring from checkpoint",
		"run_id", chkpt.RunID,
		"superstep", chkpt.Superstep)

	if err := p.applyCheckpointData(ctx, compiled, chkpt); err != nil {
		return err
	}

	p.restoreExecutionMetadata(ctx, chkpt)

	logger.Info("checkpoint restored successfully",
		"run_id", chkpt.RunID,
		"superstep", chkpt.Superstep)

	return nil
}

// loadCheckpoint loads a checkpoint from the configured source.
func (p *PregelExecutor[I, O]) loadCheckpoint(ctx context.Context, opts RunOptions) (*checkpoint.Checkpoint, error) {
	// Check if checkpoint was provided directly via WithCheckpoint()
	if opts.Checkpoint != nil {
		logger := logging.FromContext(ctx)
		logger.Info("using provided checkpoint",
			"run_id", opts.Checkpoint.RunID,
			"superstep", opts.Checkpoint.Superstep)
		return opts.Checkpoint, nil
	}

	// Original path: Load from checkpointer
	if opts.Checkpointer == nil || opts.RunID == "" || !opts.AutoRestore {
		return nil, nil
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
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	return chkpt, nil
}

// applyCheckpointData applies state and pending writes from a checkpoint.
func (p *PregelExecutor[I, O]) applyCheckpointData(ctx context.Context, compiled *Compiled[I, O], chkpt *checkpoint.Checkpoint) error {
	logger := logging.FromContext(ctx)

	// Restore state
	if len(chkpt.State) > 0 {
		if err := compiled.manager.ApplyUpdates(ctx, chkpt.State); err != nil {
			logger.Error("failed to apply checkpoint state",
				"run_id", chkpt.RunID,
				"error", err)
			return fmt.Errorf("failed to apply checkpoint state: %w", err)
		}
		logger.Info("restored state from checkpoint",
			"run_id", chkpt.RunID,
			"state_keys", len(chkpt.State))
	}

	// Apply pending writes if present
	if len(chkpt.PendingWrites) > 0 {
		logger.Info("applying pending writes from checkpoint",
			"run_id", chkpt.RunID,
			"pending_writes", len(chkpt.PendingWrites))

		updates := make(map[string]any)
		for _, pw := range chkpt.PendingWrites {
			updates[pw.Channel] = pw.Value
			logger.Debug("applying pending write",
				"node", pw.NodeName,
				"channel", pw.Channel,
				"timestamp", pw.Timestamp)
		}

		if err := compiled.manager.ApplyUpdates(ctx, updates); err != nil {
			logger.Error("failed to apply pending writes",
				"run_id", chkpt.RunID,
				"error", err)
			return fmt.Errorf("failed to apply pending writes: %w", err)
		}

		logger.Info("pending writes applied successfully",
			"run_id", chkpt.RunID,
			"writes_applied", len(chkpt.PendingWrites))
	}

	return nil
}

// restoreExecutionMetadata restores completed and paused nodes tracking.
func (p *PregelExecutor[I, O]) restoreExecutionMetadata(ctx context.Context, chkpt *checkpoint.Checkpoint) {
	if p.metrics == nil {
		return
	}

	// Restore completed nodes
	for _, nodeName := range chkpt.CompletedNodes {
		p.metrics.AddCompleted(nodeName)
	}

	// Resume paused nodes - they were paused but now we're resuming them
	// Clear them from the paused list so they can execute
	for _, nodeName := range chkpt.PausedNodes {
		p.metrics.ResumePaused(nodeName)
	}

	logger := logging.FromContext(ctx)
	logger.Info("restored execution metadata",
		"run_id", chkpt.RunID,
		"completed_nodes", len(chkpt.CompletedNodes),
		"resumed_nodes", len(chkpt.PausedNodes))
}

// saveCheckpoint creates and saves a checkpoint.
func (p *PregelExecutor[I, O]) saveCheckpoint(ctx context.Context, compiled *Compiled[I, O], opts RunOptions, superstep int64, worker *checkpointWorker) {
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
	vsnap, err := compiled.manager.Snapshot(ctx, map[string]string{
		"run_id":    opts.RunID,
		"superstep": fmt.Sprintf("%d", superstep),
	})
	if err != nil {
		// Log error but don't fail the superstep
		_ = err
	}

	// Capture execution metadata from runtime metrics
	var completedNodes, pausedNodes []string
	if p.metrics != nil {
		snapshot := p.metrics.Snapshot()
		completedNodes = snapshot.CompletedNodes
		pausedNodes = snapshot.PausedNodes
	}

	chkpt := &checkpoint.Checkpoint{
		RunID:          opts.RunID,
		Superstep:      superstep,
		Timestamp:      vsnap.Timestamp,
		Version:        0, // Manager handles versioning internally
		State:          vsnap.Data,
		CompletedNodes: completedNodes,
		PausedNodes:    pausedNodes,
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
func (p *PregelExecutor[I, O]) saveCheckpointSync(ctx context.Context, opts RunOptions, chkpt *checkpoint.Checkpoint) error {
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
func (p *PregelExecutor[I, O]) startCheckpointWorker(ctx context.Context, opts RunOptions) *checkpointWorker {
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
func (p *PregelExecutor[I, O]) stopCheckpointWorker(worker *checkpointWorker) {
	if worker == nil || worker.queue == nil {
		return
	}

	close(worker.queue)
	worker.wg.Wait()
}
