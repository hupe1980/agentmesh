package graph

import (
	"context"
	"fmt"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/logging"
	"github.com/hupe1980/agentmesh/pkg/message"
)

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

	if err := p.applyCheckpointData(ctx, compiled, chkpt, opts); err != nil {
		return err
	}

	p.restoreExecutionMetadata(ctx, compiled, chkpt)

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
func (p *PregelExecutor[I, O]) applyCheckpointData(ctx context.Context, compiled *Compiled[I, O], chkpt *checkpoint.Checkpoint, opts RunOptions) error {
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

	// Apply pending writes if present and not yet committed
	// Two-phase commit: only apply if Committed=false to prevent double-application
	if len(chkpt.PendingWrites) > 0 && !chkpt.Committed {
		logger.Info("applying uncommitted pending writes from checkpoint",
			"run_id", chkpt.RunID,
			"pending_writes", len(chkpt.PendingWrites),
			"committed", chkpt.Committed)

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
	} else if len(chkpt.PendingWrites) > 0 && chkpt.Committed {
		logger.Debug("skipping already-committed pending writes",
			"run_id", chkpt.RunID,
			"pending_writes", len(chkpt.PendingWrites))
	}

	// Process approval responses if provided (when resuming with approvals)
	if err := p.processApprovalResponses(ctx, compiled, chkpt, opts); err != nil {
		logger.Error("failed to process approval responses",
			"run_id", chkpt.RunID,
			"error", err)
		return fmt.Errorf("failed to process approval responses: %w", err)
	}

	return nil
}

// restoreExecutionMetadata restores completed and paused nodes tracking.
func (p *PregelExecutor[I, O]) restoreExecutionMetadata(ctx context.Context, compiled *Compiled[I, O], chkpt *checkpoint.Checkpoint) {
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

	// Calculate resume entry points from completed nodes using graph topology
	// This follows BSP principles: checkpoint stores state, executor derives next steps
	if len(chkpt.CompletedNodes) > 0 {
		p.resumeEntryPoints = calculateResumePoints(compiled, chkpt.CompletedNodes)
		logger := logging.FromContext(ctx)
		logger.Debug("calculated resume entry points",
			"completed_nodes", chkpt.CompletedNodes,
			"resume_from", p.resumeEntryPoints)
	}

	logger := logging.FromContext(ctx)
	logger.Info("restored execution metadata",
		"run_id", chkpt.RunID,
		"completed_nodes", len(chkpt.CompletedNodes),
		"resumed_nodes", len(chkpt.PausedNodes))
}

// calculateResumePoints determines which nodes should execute when resuming from a checkpoint.
// Called during checkpoint restore to calculate resume entry points from CompletedNodes.
// This follows BSP principles: checkpoints store state, the executor derives next execution steps.
//
// Logic:
//  1. If no nodes completed yet → start from entry points (graph.EntryPoints)
//  2. If some nodes completed → return immediate successors that aren't already completed
//  3. Filters out END node (execution done) and already-completed nodes
//  4. Deduplicates nodes (multiple completed nodes might point to same successor)
//
// Example:
//
//	step_1 → step_2 → step_3
//	If CompletedNodes = [step_1], returns [step_2]
//	If CompletedNodes = [step_1, step_2], returns [step_3]
func calculateResumePoints[I, O any](compiled *Compiled[I, O], completedNodes []string) []string {
	if len(completedNodes) == 0 {
		// No nodes completed → start from entry points
		return compiled.Graph().EntryPoints
	}

	// Create set of completed nodes for quick lookup
	completedSet := make(map[string]bool, len(completedNodes))
	for _, nodeName := range completedNodes {
		completedSet[nodeName] = true
	}

	// Collect all immediate successors of completed nodes
	// We need to get targets from the actual nodes, not topology.outgoing
	// because topology.outgoing is only populated for StartNode
	successorSet := make(map[string]bool)
	for _, nodeName := range completedNodes {
		node := compiled.Graph().Nodes[nodeName]
		if node == nil {
			continue
		}
		// Get targets declared by this node
		targets := node.Targets()
		for _, target := range targets {
			// Don't include END node or already-completed nodes
			if target != EndNode && !completedSet[target] {
				successorSet[target] = true
			}
		}
	}

	// Convert set to slice
	resumePoints := make([]string, 0, len(successorSet))
	for node := range successorSet {
		resumePoints = append(resumePoints, node)
	}

	return resumePoints
}

// saveCheckpoint creates and saves a checkpoint with two-phase commit semantics.
// It captures PendingWrites BEFORE they are applied to state, ensuring transactional
// integrity: if a crash occurs, the checkpoint contains all necessary information to
// resume without data loss.
//
// Returns an error if FailOnCheckpointErr is true and checkpoint operations fail.
// If FailOnCheckpointErr is false, errors are logged but not propagated.
func (p *PregelExecutor[I, O]) saveCheckpoint(ctx context.Context, compiled *Compiled[I, O], opts RunOptions, superstep int64, adapter *pregelGraphAdapter[I, O]) error {
	if opts.Checkpointer == nil || opts.RunID == "" {
		return nil
	}

	logger := logging.FromContext(ctx)

	// Check if we should save based on interval
	shouldSave := opts.CheckpointInterval <= 0 || superstep%int64(opts.CheckpointInterval) == 0

	if !shouldSave {
		// Skip checkpoint save, but still apply pending updates
		logger.Debug("skipping checkpoint save (interval check), applying updates only",
			"run_id", opts.RunID,
			"superstep", superstep,
			"interval", opts.CheckpointInterval)
		p.applyPendingUpdates(ctx, compiled, adapter)
		return nil
	}
	logger.Debug("saving checkpoint with two-phase commit",
		"run_id", opts.RunID,
		"superstep", superstep)

	// Create checkpoint using Manager's Snapshot (current state BEFORE new updates)
	vsnap, err := compiled.manager.Snapshot(ctx, map[string]string{
		"run_id":    opts.RunID,
		"superstep": fmt.Sprintf("%d", superstep),
	})
	if err != nil {
		logger.Error("failed to create state snapshot for checkpoint",
			"run_id", opts.RunID,
			"superstep", superstep,
			"error", err)
		if opts.FailOnCheckpointErr {
			return fmt.Errorf("checkpoint snapshot failed at superstep %d: %w", superstep, err)
		}
		// Skip checkpoint save for this superstep, but still apply pending updates
		p.applyPendingUpdates(ctx, compiled, adapter)
		return nil
	}

	// Capture execution metadata from runtime metrics
	var completedNodes, pausedNodes []string
	if p.metrics != nil {
		snapshot := p.metrics.Snapshot()
		completedNodes = snapshot.CompletedNodes
		pausedNodes = snapshot.PausedNodes
	}

	// Collect pending writes from this superstep (BEFORE application)
	adapter.updatesMu.Lock()
	pendingWrites := make([]checkpoint.PendingWrite, len(adapter.pendingUpdates))
	copy(pendingWrites, adapter.pendingUpdates)
	adapter.updatesMu.Unlock()

	chkpt := &checkpoint.Checkpoint{
		RunID:          opts.RunID,
		Superstep:      superstep,
		Timestamp:      vsnap.Timestamp,
		Version:        0, // Manager handles versioning internally
		State:          vsnap.Data,
		PendingWrites:  pendingWrites,
		Committed:      false, // Not yet applied
		CompletedNodes: completedNodes,
		PausedNodes:    pausedNodes,
		Metadata:       map[string]any{},
	}

	// Preserve approval metadata from latest checkpoint (in case approvals were just processed)
	if latestCP, err := opts.Checkpointer.Load(ctx, opts.RunID); err == nil && latestCP != nil && latestCP.ApprovalMetadata != nil {
		chkpt.ApprovalMetadata = latestCP.ApprovalMetadata
		logger.Debug("preserving approval metadata from latest checkpoint",
			"run_id", opts.RunID,
			"history_count", len(latestCP.ApprovalMetadata.ApprovalHistory),
			"pending_count", len(latestCP.ApprovalMetadata.PendingApprovals))
	}

	logger.Debug("checkpoint prepared with pending writes",
		"run_id", opts.RunID,
		"superstep", superstep,
		"pending_writes", len(pendingWrites))

	// PHASE 1: Save checkpoint with PendingWrites (not yet applied)
	// This must be synchronous to ensure proper two-phase commit
	if err := p.saveCheckpointSync(ctx, opts, chkpt); err != nil {
		logger.Error("checkpoint save failed in two-phase commit",
			"run_id", opts.RunID,
			"superstep", superstep,
			"error", err)
		// Don't apply updates if checkpoint save failed
		return err
	}

	// PHASE 2: Apply PendingWrites to state (now safe - checkpoint is durable)
	p.applyPendingUpdates(ctx, compiled, adapter)

	// PHASE 3: Update checkpoint with applied state and mark as committed
	// Take a new snapshot that includes the applied updates
	vsnap2, err := compiled.manager.Snapshot(ctx, map[string]string{
		"run_id":    opts.RunID,
		"superstep": fmt.Sprintf("%d", superstep),
	})
	if err != nil {
		logger.Warn("failed to snapshot state after applying updates",
			"run_id", opts.RunID,
			"superstep", superstep,
			"error", err)
		// Non-fatal, but checkpoint will have stale state
	} else {
		chkpt.State = vsnap2.Data // Update with applied state
	}

	chkpt.Committed = true
	chkpt.PendingWrites = nil // Clear pending writes since they're now committed

	if err := p.saveCheckpointSync(ctx, opts, chkpt); err != nil {
		logger.Warn("failed to update checkpoint committed status",
			"run_id", opts.RunID,
			"superstep", superstep,
			"error", err)
		// Non-fatal: updates were applied, so execution can continue.
		// Worst case: resume will attempt to replay PendingWrites but state
		// will already contain the updates (idempotent if updates are idempotent).
	}

	logger.Debug("two-phase commit completed",
		"run_id", opts.RunID,
		"superstep", superstep)
	return nil
}

// saveFinalCheckpoint saves a final committed checkpoint after successful execution.
// This checkpoint has all updates applied to State and empty PendingWrites.
// It provides a clean starting point for future resumes and matches test expectations.
func (p *PregelExecutor[I, O]) saveFinalCheckpoint(ctx context.Context, compiled *Compiled[I, O], opts RunOptions, superstep int64) {
	logger := logging.FromContext(ctx)

	// Create snapshot of fully applied state
	vsnap, err := compiled.manager.Snapshot(ctx, map[string]string{
		"run_id":    opts.RunID,
		"superstep": fmt.Sprintf("%d", superstep),
		"final":     "true",
	})
	if err != nil {
		logger.Error("failed to snapshot final state", "error", err)
		return
	}

	// Capture execution metadata
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
		Version:        0,
		State:          vsnap.Data, // Fully applied state
		PendingWrites:  nil,        // No pending writes - all committed
		Committed:      true,       // Mark as fully committed
		CompletedNodes: completedNodes,
		PausedNodes:    pausedNodes,
		Metadata: map[string]any{
			"final": true,
		},
	}

	// Preserve approval metadata from latest checkpoint (in case approvals were just processed)
	if latestCP, err := opts.Checkpointer.Load(ctx, opts.RunID); err == nil && latestCP != nil && latestCP.ApprovalMetadata != nil {
		chkpt.ApprovalMetadata = latestCP.ApprovalMetadata
		logger.Debug("preserving approval metadata in final checkpoint",
			"run_id", opts.RunID,
			"history_count", len(latestCP.ApprovalMetadata.ApprovalHistory),
			"pending_count", len(latestCP.ApprovalMetadata.PendingApprovals))
	}

	if err := p.saveCheckpointSync(ctx, opts, chkpt); err != nil {
		logger.Error("failed to save final checkpoint",
			"run_id", opts.RunID,
			"superstep", superstep,
			"error", err)
	} else {
		logger.Info("final committed checkpoint saved",
			"run_id", opts.RunID,
			"superstep", superstep)
	}
}

// applyPendingUpdates applies collected updates to state and clears the pending list.
// This is called after checkpoint save (if enabled) or at superstep end (if no checkpointing).
//
// IMPORTANT: Each pending write is applied individually to preserve ordering and ensure
// that multiple writes to the same channel (e.g., parallel nodes appending to messages)
// are all processed correctly. The state manager handles list-key appending, so we must
// not collapse writes into a map which would cause later writes to overwrite earlier ones.
func (p *PregelExecutor[I, O]) applyPendingUpdates(ctx context.Context, compiled *Compiled[I, O], adapter *pregelGraphAdapter[I, O]) {
	logger := logging.FromContext(ctx)

	// Get pending updates (thread-safe)
	adapter.updatesMu.Lock()
	pendingWrites := adapter.pendingUpdates
	adapter.pendingUpdates = adapter.pendingUpdates[:0] // Clear for next superstep
	adapter.updatesMu.Unlock()

	if len(pendingWrites) == 0 {
		return
	}

	// Apply each pending write individually to preserve ordering.
	// This ensures that multiple writes to the same channel (e.g., parallel tool calls
	// appending to messages) are all applied correctly rather than overwriting each other.
	for _, pw := range pendingWrites {
		if err := compiled.manager.ApplyUpdates(ctx, map[string]any{pw.Channel: pw.Value}); err != nil {
			logger.Error("failed to apply pending write",
				"error", err,
				"channel", pw.Channel,
				"node", pw.NodeName)
			// Continue applying remaining writes to preserve as much state as possible
			continue
		}
	}

	logger.Debug("pending writes applied successfully",
		"writes_applied", len(pendingWrites))
}

// processApprovalResponses handles approval responses provided during resume.
// This applies state edits from approved nodes and records approval history in the checkpoint.
func (p *PregelExecutor[I, O]) processApprovalResponses(ctx context.Context, compiled *Compiled[I, O], chkpt *checkpoint.Checkpoint, opts RunOptions) error {
	// Check if checkpoint has pending approvals
	if chkpt.ApprovalMetadata == nil || len(chkpt.ApprovalMetadata.PendingApprovals) == 0 {
		return nil // No approvals needed
	}

	logger := logging.FromContext(ctx)
	processedAny := false

	// Process each pending approval by checking context for response
	for nodeName := range chkpt.ApprovalMetadata.PendingApprovals {
		approval := ApprovalFromContext(ctx, nodeName)
		if approval == nil {
			logger.Debug("no approval found for node", "node", nodeName)
			continue // Node not approved yet - will remain pending
		}

		logger.Info("processing approval response",
			"node", nodeName,
			"decision", approval.Decision,
			"user", approval.User)

		// Apply state edits if approved and edits are provided
		if approval.Decision == ApprovalApproved && len(approval.Edits) > 0 {
			logger.Info("applying approval edits to state", "node", nodeName, "edits", len(approval.Edits))
			if err := compiled.manager.ApplyUpdates(ctx, approval.Edits); err != nil {
				return fmt.Errorf("failed to apply approval edits for node %s: %w", nodeName, err)
			}
		}

		// Record approval in history
		record := checkpoint.ApprovalRecord{
			NodeName:    nodeName,
			Decision:    string(approval.Decision),
			Reason:      approval.Reason,
			User:        approval.User,
			Timestamp:   approval.Timestamp,
			StateEdits:  approval.Edits,
			Annotations: approval.Annotations,
		}

		// Update checkpoint metadata
		if chkpt.ApprovalMetadata.ApprovalHistory == nil {
			chkpt.ApprovalMetadata.ApprovalHistory = []checkpoint.ApprovalRecord{}
		}
		chkpt.ApprovalMetadata.ApprovalHistory = append(chkpt.ApprovalMetadata.ApprovalHistory, record)

		// Remove from pending approvals
		delete(chkpt.ApprovalMetadata.PendingApprovals, nodeName)

		// Add feedback annotation if configured
		if config, ok := compiled.graph.ApprovalConfigs[nodeName]; ok && config.FeedbackAnnotation {
			p.addApprovalFeedback(ctx, compiled, nodeName, approval)
		}

		logger.Info("approval processed successfully",
			"node", nodeName,
			"decision", approval.Decision,
			"had_edits", len(approval.Edits) > 0)
		processedAny = true
	}

	// Save updated checkpoint with approval history if we processed any approvals
	if !processedAny {
		return nil
	}

	if opts.Checkpointer == nil {
		logger.Warn("checkpointer is nil - cannot save approval history!",
			"run_id", chkpt.RunID)
		return nil
	}

	logger.Info("saving checkpoint with updated approval history",
		"run_id", chkpt.RunID,
		"history_count", len(chkpt.ApprovalMetadata.ApprovalHistory))

	if err := opts.Checkpointer.Save(ctx, chkpt); err != nil {
		logger.Error("failed to save checkpoint after approval processing",
			"run_id", chkpt.RunID,
			"error", err)
		return fmt.Errorf("failed to save checkpoint after approval: %w", err)
	}

	logger.Info("checkpoint saved successfully with approval history",
		"run_id", chkpt.RunID)

	return nil
}

// addApprovalFeedback appends approval decision to message history as a system message.
func (p *PregelExecutor[I, O]) addApprovalFeedback(ctx context.Context, compiled *Compiled[I, O], nodeName string, approval *ApprovalResponse) {
	logger := logging.FromContext(ctx)

	// Create feedback message with metadata
	content := fmt.Sprintf("Human approval for %s: %s - %s (by %s)",
		nodeName, approval.Decision, approval.Reason, approval.User)

	feedbackMsg := message.NewSystemMessageFromText(content,
		message.WithMetadata(map[string]any{
			"approval_node": nodeName,
			"decision":      string(approval.Decision),
			"user":          approval.User,
			"timestamp":     approval.Timestamp,
		}))

	// Append to messages
	if err := compiled.manager.ApplyUpdates(ctx, map[string]any{
		MessagesKeyName: []message.Message{feedbackMsg},
	}); err != nil {
		logger.Warn("failed to append feedback annotation", "error", err)
	} else {
		logger.Info("added approval feedback to message history", "node", nodeName)
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
