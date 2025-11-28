// Package main demonstrates advanced human-in-the-loop workflows with approval guards.
//
// This example shows the new approval workflow features:
// - ApprovalGuard: Conditional approval based on state evaluation
// - WithApprovalGuard: Configure when approval is needed dynamically
// - ApprovalResponse: Structured approval decisions with metadata
// - State edits: Modify state when approving
// - Feedback annotations: Record decisions in message history
// - Approval history: Track all approval decisions in checkpoints
//
// Workflow:
// 1. Draft an email automatically
// 2. Approval guard evaluates if content needs review (checks for sensitive keywords)
// 3. If guard returns true, execution pauses for human approval
// 4. User reviews and provides structured ApprovalResponse (approve/reject/edit)
// 5. Resume with WithApproval() - state edits applied automatically
// 6. Node accesses approval decision via ApprovalFromContext()
// 7. Feedback annotation added to message history
// 8. Approval recorded in checkpoint history for audit

package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Advanced Approval Workflow with Guards ===")

	// Scenario 1: Sensitive content triggers approval
	runSensitiveContentWorkflow(ctx)

	fmt.Println("\n" + strings.Repeat("=", 60))

	// Scenario 2: Non-sensitive content auto-continues
	runNormalContentWorkflow(ctx)

	fmt.Println("\n" + strings.Repeat("=", 60))

	// Scenario 3: Rejection workflow
	runRejectionWorkflow(ctx)
}

// runSensitiveContentWorkflow demonstrates approval guard detecting sensitive content
func runSensitiveContentWorkflow(ctx context.Context) {
	fmt.Println("\n=== Scenario 1: Sensitive Content Requires Approval ===")

	// Create state manager
	builder := state.NewManagerBuilder()
	topicKey := state.NewKey("topic", "")
	draftKey := state.NewKey("draft", "")
	sentKey := state.NewKey("sent", false)

	state.RegisterKey(builder, topicKey)
	state.RegisterKey(builder, draftKey)
	state.RegisterKey(builder, sentKey)
	mgr := builder.Build()

	// Initialize with sensitive topic
	mgr.ApplyUpdates(ctx, state.Updates{
		topicKey.Name(): "Confidential: Q4 Layoff Plans",
	})

	// Build graph
	g, _ := graph.NewGraph(mgr)

	// Draft node
	draftNode := &graph.BaseNode{
		NodeName:        "draft_email",
		DeclaredTargets: []string{"send_email"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			topic := state.GetFromView(view, topicKey)
			fmt.Printf("📝 Drafting email about: %s\n", topic)

			draft := fmt.Sprintf("Subject: %s\n\nDear Team,\n\nImportant update regarding: %s\n\nBest regards", topic, topic)

			return []string{"send_email"}, state.Updates{
				draftKey.Name(): draft,
			}, nil
		},
	}

	// Send node with approval context access
	sendNode := &graph.BaseNode{
		NodeName:        "send_email",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Check for approval response from context
			approval := graph.ApprovalFromContext(ctx, "send_email")

			if approval != nil {
				fmt.Printf("✅ Approval received: %s (by %s)\n", approval.Decision, approval.User)
				fmt.Printf("   Reason: %s\n", approval.Reason)

				switch approval.Decision {
				case graph.ApprovalRejected:
					fmt.Println("❌ Email sending cancelled by user")
					return []string{graph.EndNode}, state.Updates{
						sentKey.Name(): false,
					}, nil

				case graph.ApprovalApproved:
					// State edits from approval.Edits are already applied automatically
					// We just need to send the email
					draft := state.GetFromView(view, draftKey)
					fmt.Printf("📧 Sending email:\n%s\n", draft)
					return []string{graph.EndNode}, state.Updates{
						sentKey.Name(): true,
					}, nil
				}
			}

			// No approval needed - send directly
			draft := state.GetFromView(view, draftKey)
			fmt.Printf("📧 Sending email:\n%s\n", draft)
			return []string{graph.EndNode}, state.Updates{
				sentKey.Name(): true,
			}, nil
		},
	}

	g.AddNode(draftNode)
	g.AddNode(sendNode)
	g.SetEntryPoint("draft_email")

	// Add interrupt with approval guard
	g.AddInterruptBefore("send_email",
		graph.WithApprovalGuard(func(ctx context.Context, view state.ReadView) (bool, string, error) {
			// Check if draft contains sensitive keywords
			draftStr := state.GetFromView(view, draftKey)
			sensitiveKeywords := []string{"confidential", "layoff", "termination", "secret", "classified"}
			for _, keyword := range sensitiveKeywords {
				if strings.Contains(strings.ToLower(draftStr), keyword) {
					return true, fmt.Sprintf("Contains sensitive keyword: %s", keyword), nil
				}
			}

			return false, "", nil // No approval needed
		}),
		graph.WithFeedbackAnnotation(true), // Record approval in message history
	)

	compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())

	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "sensitive-email-001"

	// Step 1: Run until approval guard triggers
	fmt.Println("→ Running workflow...")
	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithRunID(runID),
		graph.WithCheckpointOptions(
			checkpoint.WithCheckpointer(checkpointer),
			checkpoint.WithSaveInterval(1),
		),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	fmt.Println("⏸️  Execution paused - approval required")

	// Step 2: Load checkpoint and show approval details
	cp, _ := checkpointer.Load(ctx, runID)

	if cp.ApprovalMetadata != nil && len(cp.ApprovalMetadata.PendingApprovals) > 0 {
		for nodeName, pending := range cp.ApprovalMetadata.PendingApprovals {
			fmt.Printf("📋 Pending Approval for: %s\n", nodeName)
			fmt.Printf("   Reason: %s\n", pending.Reason)
			fmt.Printf("   Requested: %v\n", pending.RequestedAt)
		}
	}

	fmt.Printf("\n📄 Current Draft:\n%s\n\n", cp.State["draft"])

	// Step 3: User reviews and provides structured approval
	fmt.Println("→ User reviewing and editing draft...")

	editedDraft := "Subject: Confidential: Q4 Layoff Plans\n\nDear Team,\n\n[REVIEWED] Important update regarding: Q4 Organizational Changes\n\n**This content has been reviewed and approved for internal distribution only.**\n\nBest regards"

	approval := &graph.ApprovalResponse{
		Decision:  graph.ApprovalApproved,
		Reason:    "Reviewed and approved with disclaimer added",
		User:      "alice@example.com",
		Timestamp: time.Now(),
		Edits: state.Updates{
			draftKey.Name(): editedDraft,
		},
		Annotations: map[string]any{
			"department": "HR",
			"risk_level": "high",
		},
	}

	fmt.Printf("✅ Approval Decision: %s\n", approval.Decision)
	fmt.Printf("   By: %s\n", approval.User)
	fmt.Printf("   With edits: Yes\n\n")

	// Step 4: Resume with approval
	fmt.Println("→ Resuming workflow with approval...")
	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithCheckpoint(cp),
		graph.WithApproval("send_email", approval),
		graph.WithCheckpointOptions(
			checkpoint.WithCheckpointer(checkpointer), // Need checkpointer to save approval history
		),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	// Step 5: Check approval history
	fmt.Println("\n📊 Approval History:")
	history, _ := checkpointer.GetApprovalHistory(ctx, runID)
	for i, record := range history {
		fmt.Printf("  %d. Node: %s, Decision: %s, User: %s\n", i+1, record.NodeName, record.Decision, record.User)
		fmt.Printf("     Reason: %s\n", record.Reason)
		fmt.Printf("     Timestamp: %v\n", record.Timestamp)
		if len(record.StateEdits) > 0 {
			fmt.Printf("     Had edits: Yes\n")
		}
	}
}

// runNormalContentWorkflow demonstrates guard allowing auto-continue
func runNormalContentWorkflow(ctx context.Context) {
	fmt.Println("\n=== Scenario 2: Normal Content Auto-Continues ===")

	builder := state.NewManagerBuilder()
	topicKey := state.NewKey("topic", "")
	draftKey := state.NewKey("draft", "")
	sentKey := state.NewKey("sent", false)

	state.RegisterKey(builder, topicKey)
	state.RegisterKey(builder, draftKey)
	state.RegisterKey(builder, sentKey)
	mgr := builder.Build()

	// Non-sensitive topic
	mgr.ApplyUpdates(ctx, state.Updates{
		topicKey.Name(): "Weekly Team Standup Notes",
	})

	g, _ := graph.NewGraph(mgr)

	draftNode := &graph.BaseNode{
		NodeName:        "draft_email",
		DeclaredTargets: []string{"send_email"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			topic := state.GetFromView(view, topicKey)
			fmt.Printf("📝 Drafting email about: %s\n", topic)
			draft := fmt.Sprintf("Subject: %s\n\nHi team,\n\nHere are this week's updates...\n\nCheers", topic)
			return []string{"send_email"}, state.Updates{draftKey.Name(): draft}, nil
		},
	}

	sendNode := &graph.BaseNode{
		NodeName:        "send_email",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			draft := state.GetFromView(view, draftKey)
			fmt.Printf("📧 Sending email (no approval needed):\n%s\n", draft)
			return []string{graph.EndNode}, state.Updates{sentKey.Name(): true}, nil
		},
	}

	g.AddNode(draftNode)
	g.AddNode(sendNode)
	g.SetEntryPoint("draft_email")

	// Same approval guard - will return false for non-sensitive content
	g.AddInterruptBefore("send_email",
		graph.WithApprovalGuard(func(ctx context.Context, view state.ReadView) (bool, string, error) {
			draftStr := state.GetFromView(view, draftKey)
			sensitiveKeywords := []string{"confidential", "layoff", "termination"}
			for _, keyword := range sensitiveKeywords {
				if strings.Contains(strings.ToLower(draftStr), keyword) {
					return true, fmt.Sprintf("Contains sensitive keyword: %s", keyword), nil
				}
			}
			return false, "", nil // Guard says: no approval needed!
		}),
	)

	compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())

	fmt.Println("→ Running workflow...")
	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithRunID("normal-email-001"),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	fmt.Println("✅ Workflow completed without approval (guard allowed auto-continue)")
}

// runRejectionWorkflow demonstrates user rejection
func runRejectionWorkflow(ctx context.Context) {
	fmt.Println("\n=== Scenario 3: User Rejects ===")

	builder := state.NewManagerBuilder()
	topicKey := state.NewKey("topic", "")
	draftKey := state.NewKey("draft", "")
	sentKey := state.NewKey("sent", false)

	state.RegisterKey(builder, topicKey)
	state.RegisterKey(builder, draftKey)
	state.RegisterKey(builder, sentKey)
	mgr := builder.Build()

	mgr.ApplyUpdates(ctx, state.Updates{
		topicKey.Name(): "Secret Project Alpha Launch",
	})

	g, _ := graph.NewGraph(mgr)

	draftNode := &graph.BaseNode{
		NodeName:        "draft_email",
		DeclaredTargets: []string{"send_email"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			topic := state.GetFromView(view, topicKey)
			fmt.Printf("📝 Drafting email about: %s\n", topic)
			draft := fmt.Sprintf("Subject: %s\n\nAll hands announcement...", topic)
			return []string{"send_email"}, state.Updates{draftKey.Name(): draft}, nil
		},
	}

	sendNode := &graph.BaseNode{
		NodeName:        "send_email",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			approval := graph.ApprovalFromContext(ctx, "send_email")

			if approval != nil && approval.Decision == graph.ApprovalRejected {
				fmt.Printf("❌ Sending cancelled: %s\n", approval.Reason)
				return []string{graph.EndNode}, state.Updates{sentKey.Name(): false}, nil
			}

			return []string{graph.EndNode}, state.Updates{sentKey.Name(): true}, nil
		},
	}

	g.AddNode(draftNode)
	g.AddNode(sendNode)
	g.SetEntryPoint("draft_email")
	g.AddInterruptBefore("send_email",
		graph.WithApprovalGuard(func(ctx context.Context, view state.ReadView) (bool, string, error) {
			return true, "Always requires approval", nil // Always interrupt
		}),
		graph.WithFeedbackAnnotation(true),
	)

	compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "reject-email-001"

	fmt.Println("→ Running until approval...")
	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithRunID(runID),
		graph.WithCheckpointOptions(
			checkpoint.WithCheckpointer(checkpointer),
			checkpoint.WithSaveInterval(1),
		),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	cp, _ := checkpointer.Load(ctx, runID)

	fmt.Println("\n→ User rejecting draft...")

	rejection := &graph.ApprovalResponse{
		Decision:  graph.ApprovalRejected,
		Reason:    "Project not yet announced publicly - content too sensitive",
		User:      "security@example.com",
		Timestamp: time.Now(),
		Annotations: map[string]any{
			"security_risk": "critical",
			"policy":        "pre-announcement-block",
		},
	}

	fmt.Printf("❌ Rejection Decision: %s\n", rejection.Reason)
	fmt.Printf("   By: %s\n\n", rejection.User)

	fmt.Println("→ Resuming with rejection...")
	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithCheckpoint(cp),
		graph.WithApproval("send_email", rejection),
		graph.WithCheckpointOptions(
			checkpoint.WithCheckpointer(checkpointer), // Need checkpointer to save approval history
		),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	// Check final state
	view, _ := mgr.CreateReadView(ctx)
	sent := state.GetFromView(view, sentKey)
	fmt.Printf("\n✅ Final Result: Email sent = %v\n", sent)

	// Check approval history
	history, _ := checkpointer.GetApprovalHistory(ctx, runID)
	fmt.Printf("📊 Approval History: %d record(s)\n", len(history))
	for _, record := range history {
		fmt.Printf("   - %s by %s: %s\n", record.Decision, record.User, record.Reason)
	}

	fmt.Println("\n=== All Scenarios Complete ===")
}
