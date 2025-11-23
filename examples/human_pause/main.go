// Package main demonstrates human pause.

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"
)

func main() {
	currentTaskKey := graphstate.NewKey("current_task", "Impact of AI on climate change")
	actionHistoryKey := graphstate.NewListKey[string]("action_history", 0)
	humanInputKey := graphstate.NewKey("human_input", "")
	draftKey := graphstate.NewKey("draft", "")
	finalReportKey := graphstate.NewKey("final_report", "")

	mgr := graphstate.NewManager()
	if err := agent.RegisterMessagesKey(mgr); err != nil {
		log.Fatal(err)
	}
	graphstate.RegisterKey(mgr, currentTaskKey)
	graphstate.RegisterKey(mgr, actionHistoryKey.Key)
	graphstate.RegisterKey(mgr, humanInputKey)
	graphstate.RegisterKey(mgr, draftKey)
	graphstate.RegisterKey(mgr, finalReportKey)

	// Initialize action history
	if err := mgr.ApplyUpdates(context.Background(), graphstate.Updates{
		actionHistoryKey.Name(): []string{"Task initiated"},
	}); err != nil {
		panic(err)
	}

	g, err := graph.NewGraph(mgr)
	if err != nil {
		panic(err)
	}
	mustAddNode := func(n graph.Node) {
		if err := g.AddNode(n); err != nil {
			panic(err)
		}
	}

	mustAddNode(&graph.BaseCommandNode{
		NodeName:        "research",
		DeclaredTargets: graph.NewTargetSet("write"),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("research")
			topic := graphstate.GetFromView(view, currentTaskKey)
			builder := graphstate.NewUpdateBuilder()
			graphstate.AppendUpdate(builder, actionHistoryKey,
				fmt.Sprintf("Researched '%s'", topic),
				fmt.Sprintf("Summarized findings for '%s'", topic),
			)
			graphstate.SetUpdate(builder, currentTaskKey, fmt.Sprintf("Write report for %s", topic))
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.Goto("write", updates), nil
		},
	})

	mustAddNode(&graph.BaseCommandNode{
		NodeName:        "write",
		DeclaredTargets: graph.NewTargetSet("review"),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("write")
			humanInput := graphstate.GetFromView(view, humanInputKey)
			if humanInput == "" {
				fmt.Println("write paused: awaiting human approval")
				return nil, graph.ErrHumanInterrupt
			}
			task := graphstate.GetFromView(view, currentTaskKey)
			builder := graphstate.NewUpdateBuilder()
			graphstate.AppendUpdate(builder, actionHistoryKey, fmt.Sprintf("Drafted report for '%s'", task))
			graphstate.SetUpdate(builder, draftKey, "draft report content")
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.Goto("review", updates), nil
		},
	})

	mustAddNode(&graph.BaseCommandNode{
		NodeName:        "review",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view graphstate.ReadView) (*graph.Command, error) {
			fmt.Println("review")
			builder := graphstate.NewUpdateBuilder()
			graphstate.AppendUpdate(builder, actionHistoryKey, "Reviewed draft")
			graphstate.SetUpdate(builder, finalReportKey, "final report content")
			updates, err := builder.Build()
			if err != nil {
				return nil, err
			}
			return graph.End(updates), nil
		},
	})

	// Entry point
	if err := g.SetEntryPoint("research"); err != nil {
		panic(err)
	}

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	if err != nil {
		fmt.Println("compile error:", err)
		return
	}

	fmt.Println("=== First Run ===")
	ctx := context.Background()
	if _, err := graph.Last(compiled.Run(ctx, nil)); err != nil {
		fmt.Println("run paused:", err)
	}
	view, err := mgr.CreateReadView(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("state after first run - action history:", graphstate.GetFromView(view, actionHistoryKey.Key))

	// Simulate human providing input
	fmt.Println("\n🧑 Human providing input...")

	// Get current action history to append to it
	currentHistory := graphstate.GetFromView(view, actionHistoryKey.Key)

	if err := compiled.ApplyState(context.Background(), graphstate.Updates{
		humanInputKey.Name():    "Approved draft",
		actionHistoryKey.Name(): append(currentHistory, "Human approved: Approved draft"),
	}); err != nil {
		fmt.Println("failed to apply state:", err)
		return
	}

	fmt.Println("\n=== Resume ===")
	fmt.Printf("Resuming from superstep %d\n", compiled.CurrentSuperstep())
	// Resume from the current superstep to continue execution
	for range compiled.Run(ctx, nil, graph.WithInitialSuperstep(compiled.CurrentSuperstep())) {
		// Just consume the iterator
	}

	view2, err := mgr.CreateReadView(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("\nFinal state:")
	fmt.Println("  Action history:", graphstate.GetFromView(view2, actionHistoryKey.Key))
	fmt.Println("  Human input:", graphstate.GetFromView(view2, humanInputKey))
	fmt.Println("  Final report:", graphstate.GetFromView(view2, finalReportKey))
	fmt.Println("\n✓ Human pause/resume completed successfully!")
}
