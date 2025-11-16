package benchmark_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// BenchmarkComprehensiveWorkflow benchmarks a realistic agent workflow
func BenchmarkComprehensiveWorkflow(b *testing.B) {
	ctx := context.Background()

	stateManager, err := state.NewStateManager(0)
	if err != nil {
		b.Fatal(err)
	}

	// Create a realistic multi-step workflow
	g, err := graph.NewGraph(stateManager)
	if err != nil {
		b.Fatal(err)
	}

	// Node 1: Input processing
	g.AddNode(&graph.Node{
		Name: "preprocess",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			msgs := s.MessagesSnapshot()
			return &graph.NodeResult{
				Updates: map[string]any{"processed": len(msgs)},
			}, nil
		},
	})

	// Node 2: Analysis (parallel with node 3)
	g.AddNode(&graph.Node{
		Name: "analyze",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			count := s.Get("processed").(int)
			return &graph.NodeResult{
				Updates: map[string]any{"analyzed": count * 2},
			}, nil
		},
	})

	// Node 3: Validate (parallel with node 2)
	g.AddNode(&graph.Node{
		Name: "validate",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			count := s.Get("processed").(int)
			return &graph.NodeResult{
				Updates: map[string]any{"valid": count > 0},
			}, nil
		},
	})

	// Node 4: Aggregate results
	g.AddNode(&graph.Node{
		Name: "aggregate",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			analyzed := s.Get("analyzed").(int)
			valid := s.Get("valid").(bool)
			return &graph.NodeResult{
				Updates: map[string]any{
					"result": fmt.Sprintf("analyzed=%d valid=%v", analyzed, valid),
				},
			}, nil
		},
	})

	// Build topology
	g.AddEdge(graph.StartNode, "preprocess")
	g.AddEdge("preprocess", "analyze")
	g.AddEdge("preprocess", "validate")
	g.AddEdge("analyze", "aggregate")
	g.AddEdge("validate", "aggregate")
	g.AddEdge("aggregate", graph.EndNode)

	compiled, err := exec.CompileGraph(g)
	if err != nil {
		b.Fatal(err)
	}

	initialMessages := []message.Message{
		message.NewHumanMessageFromText("test input"),
	}

	b.ReportAllocs()

	for b.Loop() {
		for range compiled.Run(ctx, initialMessages) {
		}
	}
}

// BenchmarkDeepChain benchmarks a long sequential chain
func BenchmarkDeepChain(b *testing.B) {
	ctx := context.Background()

	depths := []int{5, 10, 20, 50}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth-%d", depth), func(b *testing.B) {
			stateManager, err := state.NewStateManager(0)
			if err != nil {
				b.Fatal(err)
			}

			g, err := graph.NewGraph(stateManager)
			if err != nil {
				b.Fatal(err)
			}

			// Create chain of nodes
			for i := range depth {
				nodeName := fmt.Sprintf("node-%d", i)
				g.AddNode(&graph.Node{
					Name: nodeName,
					RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
						return &graph.NodeResult{
							Updates: map[string]any{"step": nodeName},
						}, nil
					},
				})

				if i == 0 {
					g.AddEdge(graph.StartNode, nodeName)
				} else {
					g.AddEdge(fmt.Sprintf("node-%d", i-1), nodeName)
				}

				if i == depth-1 {
					g.AddEdge(nodeName, graph.EndNode)
				}
			}

			compiled, err := exec.CompileGraph(g)
			if err != nil {
				b.Fatal(err)
			}

			initialMessages := []message.Message{
				message.NewHumanMessageFromText("test"),
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				for range compiled.Run(ctx, initialMessages) {
				}
			}
		})
	}
}

// BenchmarkWideParallel benchmarks wide parallel execution
func BenchmarkWideParallel(b *testing.B) {
	ctx := context.Background()

	widths := []int{2, 5, 10, 20}

	for _, width := range widths {
		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {
			stateManager, err := state.NewStateManager(0)
			if err != nil {
				b.Fatal(err)
			}

			g, err := graph.NewGraph(stateManager)
			if err != nil {
				b.Fatal(err)
			}

			// Create parallel nodes
			for i := range width {
				nodeName := fmt.Sprintf("parallel-%d", i)
				g.AddNode(&graph.Node{
					Name: nodeName,
					RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
						return &graph.NodeResult{
							Updates: map[string]any{nodeName: i},
						}, nil
					},
				})
				g.AddEdge(graph.StartNode, nodeName)
				g.AddEdge(nodeName, graph.EndNode)
			}

			compiled, err := exec.CompileGraph(g)
			if err != nil {
				b.Fatal(err)
			}

			initialMessages := []message.Message{
				message.NewHumanMessageFromText("test"),
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				for range compiled.Run(ctx, initialMessages) {
				}
			}
		})
	}
}

// BenchmarkConditionalBranching benchmarks conditional routing
func BenchmarkConditionalBranching(b *testing.B) {
	ctx := context.Background()

	stateManager, err := state.NewStateManager(0)
	if err != nil {
		b.Fatal(err)
	}

	g, err := graph.NewGraph(stateManager)
	if err != nil {
		b.Fatal(err)
	}

	g.AddNode(&graph.Node{
		Name: "router",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"route": "branch_a"},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "branch_a",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"result": "a"},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "branch_b",
		RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"result": "b"},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "router")
	g.AddConditionalEdges("router", func(ctx context.Context, s state.Reader) []string {
		route := s.Get("route").(string)
		if route == "branch_a" {
			return []string{"branch_a"}
		}
		return []string{"branch_b"}
	}, []string{"branch_a", "branch_b"})
	g.AddEdge("branch_a", graph.EndNode)
	g.AddEdge("branch_b", graph.EndNode)

	compiled, err := exec.CompileGraph(g)
	if err != nil {
		b.Fatal(err)
	}

	initialMessages := []message.Message{
		message.NewHumanMessageFromText("test"),
	}

	b.ReportAllocs()

	for b.Loop() {
		for range compiled.Run(ctx, initialMessages) {
		}
	}
}

// BenchmarkMessageThroughput benchmarks message handling
func BenchmarkMessageThroughput(b *testing.B) {
	ctx := context.Background()

	messageCounts := []int{1, 10, 50, 100}

	for _, count := range messageCounts {
		b.Run(fmt.Sprintf("messages-%d", count), func(b *testing.B) {
			stateManager, err := state.NewStateManager(0)
			if err != nil {
				b.Fatal(err)
			}

			g, err := graph.NewGraph(stateManager)
			if err != nil {
				b.Fatal(err)
			}

			g.AddNode(&graph.Node{
				Name: "processor",
				RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
					msgs := s.MessagesSnapshot()
					return &graph.NodeResult{
						Updates: map[string]any{"count": len(msgs)},
					}, nil
				},
			})

			g.AddEdge(graph.StartNode, "processor")
			g.AddEdge("processor", graph.EndNode)

			compiled, err := exec.CompileGraph(g)
			if err != nil {
				b.Fatal(err)
			}

			// Create many messages
			initialMessages := make([]message.Message, count)
			for i := range count {
				initialMessages[i] = message.NewHumanMessageFromText(fmt.Sprintf("message-%d", i))
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				for range compiled.Run(ctx, initialMessages) {
				}
			}
		})
	}
}

// BenchmarkStateUpdates benchmarks state read/write performance
func BenchmarkStateUpdates(b *testing.B) {
	ctx := context.Background()

	keyCounts := []int{1, 5, 10, 20}

	for _, keyCount := range keyCounts {
		b.Run(fmt.Sprintf("keys-%d", keyCount), func(b *testing.B) {
			stateManager, err := state.NewStateManager(0)
			if err != nil {
				b.Fatal(err)
			}

			g, err := graph.NewGraph(stateManager)
			if err != nil {
				b.Fatal(err)
			}

			g.AddNode(&graph.Node{
				Name: "writer",
				RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
					updates := make(map[string]any, keyCount)
					for i := range keyCount {
						updates[fmt.Sprintf("key-%d", i)] = i
					}
					return &graph.NodeResult{
						Updates: updates,
					}, nil
				},
			})

			g.AddNode(&graph.Node{
				Name: "reader",
				RunFunc: func(ctx context.Context, s state.Writer) (*graph.NodeResult, error) {
					sum := 0
					for i := 0; i < keyCount; i++ {
						val := s.Get(fmt.Sprintf("key-%d", i)).(int)
						sum += val
					}
					return &graph.NodeResult{
						Updates: map[string]any{"sum": sum},
					}, nil
				},
			})

			g.AddEdge(graph.StartNode, "writer")
			g.AddEdge("writer", "reader")
			g.AddEdge("reader", graph.EndNode)

			compiled, err := exec.CompileGraph(g)
			if err != nil {
				b.Fatal(err)
			}

			initialMessages := []message.Message{
				message.NewHumanMessageFromText("test"),
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				for range compiled.Run(ctx, initialMessages) {
				}
			}
		})
	}
}
