package benchmark_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// BenchmarkComprehensiveWorkflow benchmarks a realistic agent workflow
func BenchmarkComprehensiveWorkflow(b *testing.B) {
	ctx := context.Background()

	// Create a realistic multi-step workflow
	builder := graph.NewBuilder()

	// Node 1: Input processing
	builder.Node("preprocess", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		msgs := s.MessagesSnapshot()
		return &graph.NodeResult{
			Updates: map[string]any{"processed": len(msgs)},
		}, nil
	})

	// Node 2: Analysis (parallel with node 3)
	builder.Node("analyze", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		count := s.Get("processed").(int)
		return &graph.NodeResult{
			Updates: map[string]any{"analyzed": count * 2},
		}, nil
	})

	// Node 3: Validate (parallel with node 2)
	builder.Node("validate", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		count := s.Get("processed").(int)
		return &graph.NodeResult{
			Updates: map[string]any{"valid": count > 0},
		}, nil
	})

	// Node 4: Aggregate results
	builder.Node("aggregate", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		analyzed := s.Get("analyzed").(int)
		valid := s.Get("valid").(bool)
		return &graph.NodeResult{
			Updates: map[string]any{
				"result": fmt.Sprintf("analyzed=%d valid=%v", analyzed, valid),
			},
		}, nil
	})

	// Build topology
	builder.AddEdge(graph.StartNode, "preprocess")
	builder.AddEdge("preprocess", "analyze")
	builder.AddEdge("preprocess", "validate")
	builder.AddEdge("analyze", "aggregate")
	builder.AddEdge("validate", "aggregate")
	builder.AddEdge("aggregate", graph.EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		b.Fatal(err)
	}

	initialMessages := []message.Message{
		message.NewHumanMessageFromText("test input"),
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := compiled.Invoke(ctx, initialMessages)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDeepChain benchmarks a long sequential chain
func BenchmarkDeepChain(b *testing.B) {
	ctx := context.Background()

	depths := []int{5, 10, 20, 50}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth-%d", depth), func(b *testing.B) {
			builder := graph.NewBuilder()

			// Create chain of nodes
			for i := range depth {
				nodeName := fmt.Sprintf("node-%d", i)
				builder.Node(nodeName, func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
					return &graph.NodeResult{
						Updates: map[string]any{"step": nodeName},
					}, nil
				})

				if i == 0 {
					builder.AddEdge(graph.StartNode, nodeName)
				} else {
					builder.AddEdge(fmt.Sprintf("node-%d", i-1), nodeName)
				}

				if i == depth-1 {
					builder.AddEdge(nodeName, graph.EndNode)
				}
			}

			compiled, err := builder.Compile()
			if err != nil {
				b.Fatal(err)
			}

			initialMessages := []message.Message{
				message.NewHumanMessageFromText("test"),
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				_, err := compiled.Invoke(ctx, initialMessages)
				if err != nil {
					b.Fatal(err)
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
			builder := graph.NewBuilder()

			// Create parallel nodes
			for i := range width {
				nodeName := fmt.Sprintf("parallel-%d", i)
				builder.Node(nodeName, func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
					return &graph.NodeResult{
						Updates: map[string]any{nodeName: i},
					}, nil
				})
				builder.AddEdge(graph.StartNode, nodeName)
				builder.AddEdge(nodeName, graph.EndNode)
			}

			compiled, err := builder.Compile()
			if err != nil {
				b.Fatal(err)
			}

			initialMessages := []message.Message{
				message.NewHumanMessageFromText("test"),
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				_, err := compiled.Invoke(ctx, initialMessages)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkConditionalBranching benchmarks conditional routing
func BenchmarkConditionalBranching(b *testing.B) {
	ctx := context.Background()
	builder := graph.NewBuilder()

	builder.Node("router", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		return &graph.NodeResult{
			Updates: map[string]any{"route": "branch_a"},
		}, nil
	})

	builder.Node("branch_a", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		return &graph.NodeResult{
			Updates: map[string]any{"result": "a"},
		}, nil
	})

	builder.Node("branch_b", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
		return &graph.NodeResult{
			Updates: map[string]any{"result": "b"},
		}, nil
	})

	builder.AddEdge(graph.StartNode, "router")
	builder.AddConditionalEdges("router", func(ctx context.Context, s graph.StateReader) []string {
		route := s.Get("route").(string)
		if route == "branch_a" {
			return []string{"branch_a"}
		}
		return []string{"branch_b"}
	}, []string{"branch_a", "branch_b"})
	builder.AddEdge("branch_a", graph.EndNode)
	builder.AddEdge("branch_b", graph.EndNode)

	compiled, err := builder.Compile()
	if err != nil {
		b.Fatal(err)
	}

	initialMessages := []message.Message{
		message.NewHumanMessageFromText("test"),
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := compiled.Invoke(ctx, initialMessages)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMessageThroughput benchmarks message handling
func BenchmarkMessageThroughput(b *testing.B) {
	ctx := context.Background()

	messageCounts := []int{1, 10, 50, 100}

	for _, count := range messageCounts {
		b.Run(fmt.Sprintf("messages-%d", count), func(b *testing.B) {
			builder := graph.NewBuilder()

			builder.Node("processor", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				msgs := s.MessagesSnapshot()
				return &graph.NodeResult{
					Updates: map[string]any{"count": len(msgs)},
				}, nil
			})

			builder.AddEdge(graph.StartNode, "processor")
			builder.AddEdge("processor", graph.EndNode)

			compiled, err := builder.Compile()
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
				_, err := compiled.Invoke(ctx, initialMessages)
				if err != nil {
					b.Fatal(err)
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
			builder := graph.NewBuilder()

			builder.Node("writer", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				updates := make(map[string]any, keyCount)
				for i := range keyCount {
					updates[fmt.Sprintf("key-%d", i)] = i
				}
				return &graph.NodeResult{
					Updates: updates,
				}, nil
			})

			builder.Node("reader", func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				sum := 0
				for i := 0; i < keyCount; i++ {
					val := s.Get(fmt.Sprintf("key-%d", i)).(int)
					sum += val
				}
				return &graph.NodeResult{
					Updates: map[string]any{"sum": sum},
				}, nil
			})

			builder.AddEdge(graph.StartNode, "writer")
			builder.AddEdge("writer", "reader")
			builder.AddEdge("reader", graph.EndNode)

			compiled, err := builder.Compile()
			if err != nil {
				b.Fatal(err)
			}

			initialMessages := []message.Message{
				message.NewHumanMessageFromText("test"),
			}

			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				_, err := compiled.Invoke(ctx, initialMessages)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
