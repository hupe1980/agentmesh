// Package main demonstrates human pause.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/hupe1980/agentmesh/pkg/agent"
	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

func main() {
	currentTaskKey := graphstate.NewKey("current_task", "Impact of AI on climate change")
	actionHistoryKey := graphstate.NewListKey[string]("action_history", 0)
	humanInputKey := graphstate.NewKey("human_input", "")
	draftKey := graphstate.NewKey("draft", "")
	finalReportKey := graphstate.NewKey("final_report", "")

	mgr := graphstate.NewManager()
	graphstate.RegisterKey(mgr, agent.MessagesKey.Key)
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

	mustAddNode(graph.NewBaseNode("research",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			fmt.Println("research")
			topic := graphstate.GetFromView(view, currentTaskKey)
			return graphstate.Updates{
				actionHistoryKey.Name(): []string{
					fmt.Sprintf("Researched '%s'", topic),
					fmt.Sprintf("Summarized findings for '%s'", topic),
				},
				currentTaskKey.Name(): fmt.Sprintf("Write report for %s", topic),
			}, nil
		},
	))

	mustAddNode(graph.NewBaseNode("write",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			fmt.Println("write")
			humanInput := graphstate.GetFromView(view, humanInputKey)
			if humanInput == "" {
				fmt.Println("write paused: awaiting human approval")
				return nil, graph.ErrHumanInterrupt
			}
			task := graphstate.GetFromView(view, currentTaskKey)
			return graphstate.Updates{
				actionHistoryKey.Name(): []string{fmt.Sprintf("Drafted report for '%s'", task)},
				draftKey.Name():         "draft report content",
			}, nil
		},
	))

	mustAddNode(graph.NewBaseNode("review",
		func(ctx context.Context, view *graphstate.ReadView) (graphstate.Updates, error) {
			fmt.Println("review")
			return graphstate.Updates{
				actionHistoryKey.Name(): []string{"Reviewed draft"},
				finalReportKey.Name():   "final report content",
			}, nil
		},
	))

	g.AddConditionalEdges("write", func(_ context.Context, view *graphstate.ReadView) []string {
		draft := graphstate.GetFromView(view, draftKey)
		if draft != "" {
			return []string{"review"}
		}
		return nil
	}, []string{"review"})

	g.AddEdge(graph.StartNode, "research")
	g.AddEdge("research", "write")

	compiled, err := exec.CompileGraph(g, exec.NewPregelExecutor())
	if err != nil {
		fmt.Println("compile error:", err)
		return
	}

	// Type assert to access RunnableGraph methods
	rg, ok := compiled.(*exec.RunnableGraph[[]message.Message, message.Message])
	if !ok {
		fmt.Println("failed to cast to RunnableGraph")
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

	if err := rg.ApplyState(context.Background(), graphstate.Updates{
		humanInputKey.Name(): "Approved draft",
		actionHistoryKey.Name(): []string{
			fmt.Sprintf("Human provided feedback at %s", time.Now().Format(time.RFC3339)),
		},
	}); err != nil {
		fmt.Println("failed to apply state:", err)
		return
	}

	fmt.Println("\n=== Resume ===")
	if _, err := graph.Last(compiled.Run(ctx, nil, graph.WithInitialSuperstep(rg.CurrentSuperstep()))); err != nil {
		fmt.Println("resume error:", err)
		return
	}
	view2, err := mgr.CreateReadView(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("state after resume - action history:", graphstate.GetFromView(view2, actionHistoryKey.Key))
}
