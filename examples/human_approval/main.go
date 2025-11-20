// Package main demonstrates human-in-the-loop workflow with execution interrupts and resume values.
//
// This example shows:
// - AddInterruptBefore: Pause execution before a node runs
// - WithCheckpoint: Resume from a saved checkpoint
// - WithResumeValue: Inject user decisions when resuming
// - ResumeValueFromContext: Access user input in nodes
//
// Workflow:
// 1. Draft an email automatically
// 2. Interrupt before sending (human review)
// 3. User reviews draft and decides (approve/reject/edit)
// 4. Resume with user's decision
// 5. Send node receives decision and acts accordingly

package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== Human-in-the-Loop with Interrupts Example ===")
	fmt.Println("This demonstrates:")
	fmt.Println("  • Interrupting before critical actions")
	fmt.Println("  • User review and approval")
	fmt.Println("  • Resume with user decisions")
	fmt.Println("  • Handling rejection and edits")
	fmt.Println()

	// Run approval workflow
	runApprovalWorkflow(ctx)

	// Run rejection workflow
	fmt.Println("\n" + strings.Repeat("=", 50))
	runRejectionWorkflow(ctx)
}

func runApprovalWorkflow(ctx context.Context) {
	fmt.Println("=== Scenario 1: User Approves with Edits ===")

	// Define state keys
	topicKey := graphstate.NewKey("topic", "")
	draftKey := graphstate.NewKey("draft", "")
	sentKey := graphstate.NewKey("sent", false)
	statusKey := graphstate.NewKey("status", "")

	// Create state manager and register keys
	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, topicKey)
	graphstate.RegisterKey(mgr, draftKey)
	graphstate.RegisterKey(mgr, sentKey)
	graphstate.RegisterKey(mgr, statusKey)

	// Initialize state
	if err := mgr.ApplyUpdates(ctx, graphstate.Updates{
		topicKey.Name(): "Quarterly Report Deadline",
	}); err != nil {
		log.Fatal(err)
	}

	// Build graph
	g, err := graph.NewGraph(mgr)
	if err != nil {
		log.Fatal(err)
	}

	// Node 1: Draft email
	draftNode := graph.NewBaseNode("draft_email",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			topic := graphstate.GetFromView(view, topicKey)
			fmt.Printf("→ Drafting email about: %s\n", topic)

			draft := fmt.Sprintf("Dear Team,\n\nThis is a reminder about: %s\n\nBest regards", topic)

			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, draftKey, draft)
			graphstate.SetUpdate(builder, statusKey, "drafted")
			return builder.Build()
		},
	)

	// Node 2: Send email (with interrupt handling)
	sendNode := graph.NewBaseNode("send_email",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			// Check for resume value (user decision)
			resumeVals := graph.ResumeValueFromContext(ctx)

			if resumeVals != nil {
				fmt.Println("→ Resume values received from user")

				// Check if user rejected
				if approved, ok := resumeVals["approved"].(bool); ok && !approved {
					fmt.Println("  ❌ User rejected - email not sent")
					builder := graphstate.NewUpdateBuilder()
					graphstate.SetUpdate(builder, sentKey, false)
					graphstate.SetUpdate(builder, statusKey, "rejected")
					return builder.Build()
				}

				// Check if user edited the draft
				if editedDraft, ok := resumeVals["edited_draft"].(string); ok {
					fmt.Println("  ✏️  User edited draft - sending edited version")
					builder := graphstate.NewUpdateBuilder()
					graphstate.SetUpdate(builder, draftKey, editedDraft)
					graphstate.SetUpdate(builder, sentKey, true)
					graphstate.SetUpdate(builder, statusKey, "sent_edited")
					return builder.Build()
				}

				// User approved without edits
				fmt.Println("  ✅ User approved - sending original draft")
			}

			// Get draft and send
			draft := graphstate.GetFromView(view, draftKey)
			fmt.Printf("→ Sending email:\n%s\n", draft)

			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, sentKey, true)
			graphstate.SetUpdate(builder, statusKey, "sent")
			return builder.Build()
		},
	)

	// Add nodes to graph
	if err := g.AddNode(draftNode); err != nil {
		log.Fatal(err)
	}
	if err := g.AddNode(sendNode); err != nil {
		log.Fatal(err)
	}

	// Add edges
	g.AddEdge(graph.StartNode, "draft_email")
	g.AddEdge("draft_email", "send_email")
	g.AddEdge("send_email", graph.EndNode)

	// Add interrupt before send for human review
	g.AddInterruptBefore("send_email")

	// Compile graph
	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		log.Fatal(err)
	}

	// Create checkpointer
	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "email-approval-001"

	// === STEP 1: Run until interrupt ===
	fmt.Println("\n--- Step 1: Running until interrupt ---")

	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithRunID(runID),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1,
		}),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	fmt.Println("⏸️  Execution paused at interrupt point")

	// === STEP 2: User reviews checkpoint ===
	fmt.Println("\n--- Step 2: User reviewing draft ---")

	cp, err := checkpointer.Load(ctx, runID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nCheckpoint Info:\n")
	fmt.Printf("  Paused nodes: %v\n", cp.PausedNodes)
	fmt.Printf("  Completed nodes: %v\n", cp.CompletedNodes)
	fmt.Printf("\nCurrent State:\n")
	fmt.Printf("  Topic: %v\n", cp.State["topic"])
	fmt.Printf("  Draft:\n%v\n", cp.State["draft"])
	fmt.Printf("  Status: %v\n", cp.State["status"])

	// === STEP 3: User edits and approves ===
	fmt.Println("\n--- Step 3: User editing and approving ---")

	editedDraft := "Dear Team,\n\n[URGENT] This is a critical reminder about: Quarterly Report Deadline\nPlease submit by EOD Friday.\n\nBest regards"
	fmt.Printf("User edited draft:\n%s\n", editedDraft)

	userDecision := map[string]any{
		"approved":     true,
		"edited_draft": editedDraft,
	}

	// === STEP 4: Resume with user decision ===
	fmt.Println("\n--- Step 4: Resuming with approval ---")

	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithCheckpoint(cp),
		graph.WithResumeValue(userDecision),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	// Verify final state
	view, _ := mgr.CreateReadView(ctx)
	fmt.Printf("\n✅ Final State:\n")
	fmt.Printf("  Sent: %v\n", graphstate.GetFromView(view, sentKey))
	fmt.Printf("  Status: %v\n", graphstate.GetFromView(view, statusKey))
}

func runRejectionWorkflow(ctx context.Context) {
	fmt.Println("\n=== Scenario 2: User Rejects ===")

	// Define state keys
	topicKey := graphstate.NewKey("topic", "")
	draftKey := graphstate.NewKey("draft", "")
	sentKey := graphstate.NewKey("sent", false)
	statusKey := graphstate.NewKey("status", "")

	// Create fresh state manager
	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, topicKey)
	graphstate.RegisterKey(mgr, draftKey)
	graphstate.RegisterKey(mgr, sentKey)
	graphstate.RegisterKey(mgr, statusKey)

	if err := mgr.ApplyUpdates(ctx, graphstate.Updates{
		topicKey.Name(): "Marketing Campaign Launch",
	}); err != nil {
		log.Fatal(err)
	}

	// Build graph (same structure)
	g, _ := graph.NewGraph(mgr)

	draftNode := graph.NewBaseNode("draft_email",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			topic := graphstate.GetFromView(view, topicKey)
			fmt.Printf("→ Drafting email about: %s\n", topic)
			draft := fmt.Sprintf("Dear Team,\n\nExciting news about: %s\n\nBest regards", topic)
			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, draftKey, draft)
			graphstate.SetUpdate(builder, statusKey, "drafted")
			return builder.Build()
		},
	)

	sendNode := graph.NewBaseNode("send_email",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			resumeVals := graph.ResumeValueFromContext(ctx)
			if resumeVals != nil {
				if approved, ok := resumeVals["approved"].(bool); ok && !approved {
					reason := resumeVals["reason"].(string)
					fmt.Printf("  ❌ User rejected: %s\n", reason)
					builder := graphstate.NewUpdateBuilder()
					graphstate.SetUpdate(builder, sentKey, false)
					graphstate.SetUpdate(builder, statusKey, fmt.Sprintf("rejected: %s", reason))
					return builder.Build()
				}
			}
			builder := graphstate.NewUpdateBuilder()
			graphstate.SetUpdate(builder, sentKey, true)
			graphstate.SetUpdate(builder, statusKey, "sent")
			return builder.Build()
		},
	)

	g.AddNode(draftNode)
	g.AddNode(sendNode)
	g.AddEdge(graph.StartNode, "draft_email")
	g.AddEdge("draft_email", "send_email")
	g.AddEdge("send_email", graph.EndNode)
	g.AddInterruptBefore("send_email")

	compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())

	checkpointer := checkpoint.NewInMemoryCheckpointer()
	runID := "email-reject-001"

	// Run until interrupt
	fmt.Println("\n--- Running until interrupt ---")
	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithRunID(runID),
		graph.WithCheckpointConfig(checkpoint.Config{
			Checkpointer: checkpointer,
			SaveInterval: 1,
		}),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	// User rejects
	cp, _ := checkpointer.Load(ctx, runID)
	fmt.Println("\n--- User rejecting draft ---")

	userRejection := map[string]any{
		"approved": false,
		"reason":   "Tone is too casual for this audience",
	}

	// Resume with rejection
	fmt.Println("\n--- Resuming with rejection ---")
	for _, err := range compiled.Run(ctx, []message.Message{},
		graph.WithCheckpoint(cp),
		graph.WithResumeValue(userRejection),
	) {
		if err != nil {
			log.Printf("Error: %v", err)
		}
	}

	// Verify final state
	view, _ := mgr.CreateReadView(ctx)
	fmt.Printf("\n✅ Final State:\n")
	fmt.Printf("  Sent: %v\n", graphstate.GetFromView(view, sentKey))
	fmt.Printf("  Status: %v\n", graphstate.GetFromView(view, statusKey))

	fmt.Println("\n=== Example Complete ===")
}
