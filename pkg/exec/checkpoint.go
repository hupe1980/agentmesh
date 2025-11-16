package exec

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
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
		for key, value := range chkpt.State {
			if err := compiled.StateManager.Set(key, value); err != nil {
				logger.Error("failed to restore state key",
					"key", key,
					"error", err)
				return fmt.Errorf("failed to restore state key %q: %w", key, err)
			}
		}
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

	// Create checkpoint
	chkpt := &checkpoint.Checkpoint{
		RunID:     opts.RunID,
		Superstep: superstep,
		Timestamp: time.Now(),
		State:     compiled.StateManager.GetAll(),
		// TODO: Add messages, completed nodes, paused nodes
		Messages:       []message.Message{},
		CompletedNodes: []string{},
		PausedNodes:    []string{},
		Metadata:       map[string]any{},
	}

	// Queue or save checkpoint
	if worker != nil && worker.queue != nil {
		select {
		case worker.queue <- chkpt:
			logger.Debug("checkpoint queued for async save",
				"run_id", opts.RunID,
				"superstep", superstep)
		default:
			logger.Warn("checkpoint queue full, saving synchronously",
				"run_id", opts.RunID,
				"superstep", superstep)
			if err := p.saveCheckpointSync(ctx, opts, chkpt); err != nil {
				logger.Error("synchronous checkpoint save failed",
					"run_id", opts.RunID,
					"superstep", superstep,
					"error", err)
			}
		}
	} else {
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

	logger := logging.FromContext(ctx)
	logger.Debug("starting async checkpoint worker", "run_id", opts.RunID)

	worker := &checkpointWorker{
		queue: make(chan *checkpoint.Checkpoint, 1),
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
