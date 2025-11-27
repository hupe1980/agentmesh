package graph

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// TestApprovalGuard tests approval guard evaluation
func TestApprovalGuard(t *testing.T) {
	t.Run("guard returns true for sensitive content", func(t *testing.T) {
		mgr := state.NewManager()
		contentKey := state.NewKey("content", "")
		state.RegisterKey(mgr, contentKey)

		ctx := context.Background()
		mgr.ApplyUpdates(ctx, state.Updates{
			contentKey.Name(): "This contains confidential information",
		})

		view, _ := mgr.CreateReadView(ctx)

		guard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
			content := state.GetFromView(view, contentKey)
			if len(content) > 0 && content != "" {
				return true, "Contains sensitive data", nil
			}
			return false, "", nil
		}

		needsApproval, reason, err := guard(ctx, view)
		if err != nil {
			t.Fatalf("guard failed: %v", err)
		}
		if !needsApproval {
			t.Error("expected guard to require approval")
		}
		if reason != "Contains sensitive data" {
			t.Errorf("expected reason 'Contains sensitive data', got %q", reason)
		}
	})

	t.Run("guard returns false for normal content", func(t *testing.T) {
		mgr := state.NewManager()
		contentKey := state.NewKey("content", "")
		state.RegisterKey(mgr, contentKey)

		ctx := context.Background()
		mgr.ApplyUpdates(ctx, state.Updates{
			contentKey.Name(): "",
		})

		view, _ := mgr.CreateReadView(ctx)

		guard := func(ctx context.Context, view state.ReadView) (bool, string, error) {
			content := state.GetFromView(view, contentKey)
			if len(content) > 0 && content != "" {
				return true, "Contains sensitive data", nil
			}
			return false, "", nil
		}

		needsApproval, reason, err := guard(ctx, view)
		if err != nil {
			t.Fatalf("guard failed: %v", err)
		}
		if needsApproval {
			t.Error("expected guard to allow auto-continue")
		}
		if reason != "" {
			t.Errorf("expected empty reason, got %q", reason)
		}
	})
}

// TestApprovalResponse tests approval response creation and usage
func TestApprovalResponse(t *testing.T) {
	t.Run("creates valid approval response", func(t *testing.T) {
		approval := &ApprovalResponse{
			Decision:  ApprovalApproved,
			Reason:    "Looks good",
			User:      "alice@example.com",
			Timestamp: time.Now(),
			Edits: state.Updates{
				"field1": "value1",
			},
			Annotations: map[string]any{
				"department": "engineering",
			},
		}

		if approval.Decision != ApprovalApproved {
			t.Errorf("expected APPROVED, got %s", approval.Decision)
		}
		if approval.User != "alice@example.com" {
			t.Errorf("expected alice@example.com, got %s", approval.User)
		}
		if len(approval.Edits) != 1 {
			t.Errorf("expected 1 edit, got %d", len(approval.Edits))
		}
	})

	t.Run("supports all decision types", func(t *testing.T) {
		decisions := []ApprovalDecision{
			ApprovalApproved,
			ApprovalRejected,
			ApprovalEdit,
			ApprovalSkip,
		}

		for _, decision := range decisions {
			approval := &ApprovalResponse{
				Decision: decision,
			}
			if approval.Decision != decision {
				t.Errorf("expected %s, got %s", decision, approval.Decision)
			}
		}
	})
}

// TestApprovalContext tests context-based approval access
func TestApprovalContext(t *testing.T) {
	t.Run("stores and retrieves approval from context", func(t *testing.T) {
		ctx := context.Background()

		approval := &ApprovalResponse{
			Decision: ApprovalApproved,
			User:     "test@example.com",
		}

		ctx = WithApprovalResponse(ctx, "node1", approval)

		retrieved := ApprovalFromContext(ctx, "node1")
		if retrieved == nil {
			t.Fatal("expected approval to be in context")
		}
		if retrieved.Decision != ApprovalApproved {
			t.Errorf("expected APPROVED, got %s", retrieved.Decision)
		}
		if retrieved.User != "test@example.com" {
			t.Errorf("expected test@example.com, got %s", retrieved.User)
		}
	})

	t.Run("returns nil for missing node", func(t *testing.T) {
		ctx := context.Background()

		approval := &ApprovalResponse{
			Decision: ApprovalApproved,
		}

		ctx = WithApprovalResponse(ctx, "node1", approval)

		retrieved := ApprovalFromContext(ctx, "node2")
		if retrieved != nil {
			t.Error("expected nil for missing node")
		}
	})

	t.Run("supports multiple approvals", func(t *testing.T) {
		ctx := context.Background()

		approval1 := &ApprovalResponse{
			Decision: ApprovalApproved,
			User:     "user1",
		}
		approval2 := &ApprovalResponse{
			Decision: ApprovalRejected,
			User:     "user2",
		}

		ctx = WithApprovalResponse(ctx, "node1", approval1)
		ctx = WithApprovalResponse(ctx, "node2", approval2)

		retrieved1 := ApprovalFromContext(ctx, "node1")
		retrieved2 := ApprovalFromContext(ctx, "node2")

		if retrieved1.User != "user1" {
			t.Errorf("expected user1, got %s", retrieved1.User)
		}
		if retrieved2.User != "user2" {
			t.Errorf("expected user2, got %s", retrieved2.User)
		}
	})
}

// TestApprovalError tests approval-required error handling
func TestApprovalError(t *testing.T) {
	t.Run("creates approval required error", func(t *testing.T) {
		info := &ApprovalInfo{
			NodeName:    "test_node",
			Reason:      "needs review",
			RequestedAt: time.Now(),
		}
		err := NewApprovalRequiredError(info)

		if !IsApprovalRequired(err) {
			t.Error("expected error to be approval required")
		}

		extractedInfo := ApprovalInfoFromError(err)
		if extractedInfo == nil {
			t.Fatal("expected approval info")
		}
		if extractedInfo.NodeName != "test_node" {
			t.Errorf("expected test_node, got %s", extractedInfo.NodeName)
		}
		if extractedInfo.Reason != "needs review" {
			t.Errorf("expected 'needs review', got %s", extractedInfo.Reason)
		}
	})

	t.Run("returns false for non-approval error", func(t *testing.T) {
		err := context.Canceled

		if IsApprovalRequired(err) {
			t.Error("expected non-approval error")
		}

		info := ApprovalInfoFromError(err)
		if info != nil {
			t.Error("expected nil info for non-approval error")
		}
	})
}

// TestApprovalWorkflowIntegration tests end-to-end approval workflow
func TestApprovalWorkflowIntegration(t *testing.T) {
	t.Run("approval workflow with guard and response", func(t *testing.T) {
		ctx := context.Background()

		// Setup state
		mgr := state.NewManager()
		contentKey := state.NewKey("content", "")
		approvedKey := state.NewKey("approved", false)
		state.RegisterKey(mgr, contentKey)
		state.RegisterKey(mgr, approvedKey)

		mgr.ApplyUpdates(ctx, state.Updates{
			contentKey.Name(): "confidential data",
		})

		// Build graph
		g, _ := NewGraph(mgr)

		processNode := &BaseNode{
			NodeName:        "process",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				approval := ApprovalFromContext(ctx, "process")

				if approval != nil {
					if approval.Decision == ApprovalApproved {
						return []string{EndNode}, state.Updates{
							approvedKey.Name(): true,
						}, nil
					} else {
						return []string{EndNode}, state.Updates{
							approvedKey.Name(): false,
						}, nil
					}
				}

				// No approval - proceed normally
				return []string{EndNode}, state.Updates{
					approvedKey.Name(): true,
				}, nil
			},
		}

		g.AddNode(processNode)
		g.SetEntryPoint("process")

		// Add interrupt with guard
		g.AddInterruptBefore("process",
			WithApprovalGuard(func(ctx context.Context, view state.ReadView) (bool, string, error) {
				content := state.GetFromView(view, contentKey)
				if content == "confidential data" {
					return true, "Contains confidential data", nil
				}
				return false, "", nil
			}),
		)

		compiled, _ := Compile(g, NewMessagePregelExecutor())
		checkpointer := checkpoint.NewInMemoryCheckpointer()
		runID := "test-approval-001"

		// Step 1: Run until interrupt
		for _, err := range compiled.Run(ctx, []message.Message{},
			WithRunID(runID),
			WithCheckpointOptions(
				checkpoint.WithCheckpointer(checkpointer),
				checkpoint.WithSaveInterval(1),
			),
		) {
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}
		}

		// Verify checkpoint has pending approval
		cp, _ := checkpointer.Load(ctx, runID)
		if cp.ApprovalMetadata == nil {
			t.Fatal("expected approval metadata")
		}
		if len(cp.ApprovalMetadata.PendingApprovals) != 1 {
			t.Errorf("expected 1 pending approval, got %d", len(cp.ApprovalMetadata.PendingApprovals))
		}

		pending, ok := cp.ApprovalMetadata.PendingApprovals["process"]
		if !ok {
			t.Fatal("expected pending approval for 'process' node")
		}
		if pending.Reason != "Contains confidential data" {
			t.Errorf("expected 'Contains confidential data', got %s", pending.Reason)
		}

		// Step 2: Resume with approval
		approval := &ApprovalResponse{
			Decision:  ApprovalApproved,
			Reason:    "Reviewed and approved",
			User:      "tester@example.com",
			Timestamp: time.Now(),
		}

		for _, err := range compiled.Run(ctx, []message.Message{},
			WithCheckpoint(cp),
			WithApproval("process", approval),
			WithCheckpointOptions(
				checkpoint.WithCheckpointer(checkpointer),
			),
		) {
			if err != nil {
				t.Fatalf("resume failed: %v", err)
			}
		}

		// Verify approval was recorded
		history, _ := checkpointer.GetApprovalHistory(ctx, runID)
		if len(history) != 1 {
			t.Fatalf("expected 1 approval record, got %d", len(history))
		}

		record := history[0]
		if record.NodeName != "process" {
			t.Errorf("expected 'process', got %s", record.NodeName)
		}
		if record.Decision != "APPROVED" {
			t.Errorf("expected APPROVED, got %s", record.Decision)
		}
		if record.User != "tester@example.com" {
			t.Errorf("expected tester@example.com, got %s", record.User)
		}

		// Verify final state
		view, _ := mgr.CreateReadView(ctx)
		approved := state.GetFromView(view, approvedKey)
		if !approved {
			t.Error("expected approved to be true")
		}
	})

	t.Run("rejection workflow", func(t *testing.T) {
		ctx := context.Background()

		mgr := state.NewManager()
		processedKey := state.NewKey("processed", false)
		state.RegisterKey(mgr, processedKey)

		g, _ := NewGraph(mgr)

		node := &BaseNode{
			NodeName:        "action",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				approval := ApprovalFromContext(ctx, "action")
				if approval != nil && approval.Decision == ApprovalRejected {
					return []string{EndNode}, state.Updates{
						processedKey.Name(): false,
					}, nil
				}
				return []string{EndNode}, state.Updates{
					processedKey.Name(): true,
				}, nil
			},
		}

		g.AddNode(node)
		g.SetEntryPoint("action")
		g.AddInterruptBefore("action",
			WithApprovalGuard(func(ctx context.Context, view state.ReadView) (bool, string, error) {
				return true, "Always requires approval", nil
			}),
		)

		compiled, _ := Compile(g, NewMessagePregelExecutor())
		checkpointer := checkpoint.NewInMemoryCheckpointer()
		runID := "test-reject-001"

		// Run until interrupt
		for _, err := range compiled.Run(ctx, []message.Message{},
			WithRunID(runID),
			WithCheckpointOptions(
				checkpoint.WithCheckpointer(checkpointer),
				checkpoint.WithSaveInterval(1),
			),
		) {
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}
		}

		cp, _ := checkpointer.Load(ctx, runID)

		// Reject
		rejection := &ApprovalResponse{
			Decision:  ApprovalRejected,
			Reason:    "Not approved",
			User:      "reviewer@example.com",
			Timestamp: time.Now(),
		}

		for _, err := range compiled.Run(ctx, []message.Message{},
			WithCheckpoint(cp),
			WithApproval("action", rejection),
			WithCheckpointOptions(
				checkpoint.WithCheckpointer(checkpointer),
			),
		) {
			if err != nil {
				t.Fatalf("resume failed: %v", err)
			}
		}

		// Verify rejection was recorded
		history, _ := checkpointer.GetApprovalHistory(ctx, runID)
		if len(history) != 1 {
			t.Fatalf("expected 1 record, got %d", len(history))
		}
		if history[0].Decision != "REJECTED" {
			t.Errorf("expected REJECTED, got %s", history[0].Decision)
		}

		// Verify action was not processed
		view, _ := mgr.CreateReadView(ctx)
		processed := state.GetFromView(view, processedKey)
		if processed {
			t.Error("expected processed to be false after rejection")
		}
	})

	t.Run("approval with state edits", func(t *testing.T) {
		ctx := context.Background()

		mgr := state.NewManager()
		draftKey := state.NewKey("draft", "")
		finalKey := state.NewKey("final", "")
		state.RegisterKey(mgr, draftKey)
		state.RegisterKey(mgr, finalKey)

		mgr.ApplyUpdates(ctx, state.Updates{
			draftKey.Name(): "Original draft",
		})

		g, _ := NewGraph(mgr)

		node := &BaseNode{
			NodeName:        "publish",
			DeclaredTargets: []string{EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				draft := state.GetFromView(view, draftKey)
				return []string{EndNode}, state.Updates{
					finalKey.Name(): draft,
				}, nil
			},
		}

		g.AddNode(node)
		g.SetEntryPoint("publish")
		g.AddInterruptBefore("publish")

		compiled, _ := Compile(g, NewMessagePregelExecutor())
		checkpointer := checkpoint.NewInMemoryCheckpointer()
		runID := "test-edit-001"

		// Run until interrupt
		for _, err := range compiled.Run(ctx, []message.Message{},
			WithRunID(runID),
			WithCheckpointOptions(
				checkpoint.WithCheckpointer(checkpointer),
				checkpoint.WithSaveInterval(1),
			),
		) {
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}
		}

		cp, _ := checkpointer.Load(ctx, runID)

		// Approve with edits
		approval := &ApprovalResponse{
			Decision:  ApprovalApproved,
			Reason:    "Approved with edits",
			User:      "editor@example.com",
			Timestamp: time.Now(),
			Edits: state.Updates{
				draftKey.Name(): "Edited draft",
			},
		}

		for _, err := range compiled.Run(ctx, []message.Message{},
			WithCheckpoint(cp),
			WithApproval("publish", approval),
			WithCheckpointOptions(
				checkpoint.WithCheckpointer(checkpointer),
			),
		) {
			if err != nil {
				t.Fatalf("resume failed: %v", err)
			}
		}

		// Verify edits were applied
		view, _ := mgr.CreateReadView(ctx)
		draft := state.GetFromView(view, draftKey)
		final := state.GetFromView(view, finalKey)

		if draft != "Edited draft" {
			t.Errorf("expected 'Edited draft', got %s", draft)
		}
		if final != "Edited draft" {
			t.Errorf("expected final to be 'Edited draft', got %s", final)
		}

		// Verify history includes edits
		history, _ := checkpointer.GetApprovalHistory(ctx, runID)
		if len(history[0].StateEdits) == 0 {
			t.Error("expected state edits in history")
		}
	})
}
