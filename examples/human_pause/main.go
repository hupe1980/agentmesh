// Package main demonstrates human pause.

package main

import (
	"context"
	"fmt"
	"time"

	graphstate "github.com/hupe1980/agentmesh/pkg/state"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/graph"
)

func main() {
	state, err := graph.NewStateManager(0) // Unlimited messages
	if err != nil {
		panic(err)
	}

	// Initialize state values using LastValueChannel (auto-created by Set)
	if err := state.Set("current_task", "Impact of AI on climate change"); err != nil {
		panic(err)
	}
	// Note: Don't initialize optional fields to nil - they'll be nil by default
	// when reading from non-existent channels. Setting nil values will return an error
	// because LastValueChannel uses atomic.Value which cannot store nil.

	// For action_history, we want accumulation behavior (append semantics)
	// Use TopicChannel for this instead of a reducer
	state.AddChannel(channel.NewTopicChannel("action_history", 0))
	state.ApplyUpdates(map[string]any{
		"action_history": "Task initiated",
	}, nil)

	g, err := graph.NewGraph(state)
	if err != nil {
		panic(err)
	}
	mustAddNode := func(n *graph.Node) {
		if err := g.AddNode(n); err != nil {
			panic(err)
		}
	}

	mustAddNode(&graph.Node{
		Name: "research",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			fmt.Println("research")
			topic, _ := s.Get("current_task").(string)
			return &graph.NodeResult{
				Updates: map[string]any{
					"action_history": []string{
						fmt.Sprintf("Researched '%s'", topic),
						fmt.Sprintf("Summarized findings for '%s'", topic),
					},
					"current_task": fmt.Sprintf("Write report for %s", topic),
				},
			}, nil
		},
	})

	mustAddNode(&graph.Node{
		Name: "write",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			fmt.Println("write")
			if s.Get("human_input") == nil {
				fmt.Println("write paused: awaiting human approval")
				return nil, graph.ErrHumanInterrupt
			}
			task, _ := s.Get("current_task").(string)
			return &graph.NodeResult{
				Updates: map[string]any{
					"action_history": []string{fmt.Sprintf("Drafted report for '%s'", task)},
					"draft":          "draft report content",
				},
			}, nil
		},
	})

	mustAddNode(&graph.Node{
		Name: "review",
		RunFunc: func(ctx context.Context, s graphstate.Writer) (*graph.NodeResult, error) {
			fmt.Println("review")
			return &graph.NodeResult{
				Updates: map[string]any{
					"action_history": []string{"Reviewed draft"},
					"final_report":   "final report content",
				},
			}, nil
		},
	})

	g.AddConditionalEdges("write", func(_ context.Context, s graphstate.Reader) []string {
		if _, ok := s.Get("draft").(string); ok {
			return []string{"review"}
		}
		return nil
	}, []string{"review"})

	g.AddEdge(graph.StartNode, "research")
	g.AddEdge("research", "write")

	compiled, err := g.Compile()
	if err != nil {
		fmt.Println("compile error:", err)
		return
	}

	fmt.Println("=== First Run ===")
	if _, err := graph.Last(compiled.Run(context.Background(), nil)); err != nil {
		fmt.Println("run paused:", err)
	}
	fmt.Println("state after first run:", state.GetAll())

	compiled.ApplyState(map[string]any{
		"human_input": "Approved draft",
		"action_history": []string{
			fmt.Sprintf("Human provided feedback at %s", time.Now().Format(time.RFC3339)),
		},
	}, nil)

	fmt.Println("\n=== Resume ===")
	if _, err := graph.Last(compiled.Run(context.Background(), nil, graph.WithInitialSuperstep(compiled.CurrentSuperstep()))); err != nil {
		fmt.Println("resume error:", err)
		return
	}
	fmt.Println("state after resume:", state.GetAll())
}
