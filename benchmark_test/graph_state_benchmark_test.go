package benchmark_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	stateif "github.com/hupe1980/agentmesh/pkg/state"
)

// BenchmarkGraphExecution measures performance of basic graph execution
func BenchmarkGraphExecution(b *testing.B) {
	sizes := []int{10, 50, 100}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("nodes_%d", size), func(b *testing.B) {
			state, err := graph.NewStateManager(0)
			if err != nil {
				b.Fatal(err)
			}
			g, err := graph.NewGraph(state)
			if err != nil {
				b.Fatal(err)
			}

			// Create a chain of nodes
			for i := 0; i < size; i++ {
				nodeNum := i
				err := g.AddNode(&graph.Node{
					Name: fmt.Sprintf("node_%d", nodeNum),
					RunFunc: func(ctx context.Context, s stateif.Writer) (*graph.NodeResult, error) {
						// Simple computation
						val := s.Get("counter")
						count, ok := val.(int)
						if !ok {
							count = 0
						}
						return &graph.NodeResult{
							Updates: map[string]any{
								"counter": count + 1,
							},
						}, nil
					},
				})
				if err != nil {
					b.Fatal(err)
				}

				if i == 0 {
					g.AddEdge(graph.StartNode, fmt.Sprintf("node_%d", i))
				} else {
					g.AddEdge(fmt.Sprintf("node_%d", i-1), fmt.Sprintf("node_%d", i))
				}
			}

			compiled, err := g.Compile()
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for b.Loop() {
				_, err := graph.Last(compiled.Run(context.Background(), nil))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkParallelExecution measures parallel node execution performance
func BenchmarkParallelExecution(b *testing.B) {
	parallelSizes := []int{5, 10, 20}

	for _, size := range parallelSizes {
		b.Run(fmt.Sprintf("parallel_%d", size), func(b *testing.B) {
			state, err := graph.NewStateManager(0)
			if err != nil {
				b.Fatal(err)
			}
			g, err := graph.NewGraph(state)
			if err != nil {
				b.Fatal(err)
			}

			// Create parallel nodes (all from START)
			for i := 0; i < size; i++ {
				nodeNum := i
				err := g.AddNode(&graph.Node{
					Name: fmt.Sprintf("parallel_%d", nodeNum),
					RunFunc: func(ctx context.Context, s stateif.Writer) (*graph.NodeResult, error) {
						// Simulate some work
						sum := 0
						for j := range 1000 {
							sum += j
						}
						return &graph.NodeResult{
							Updates: map[string]any{
								fmt.Sprintf("result_%d", nodeNum): sum,
							},
						}, nil
					},
				})
				if err != nil {
					b.Fatal(err)
				}
				g.AddEdge(graph.StartNode, fmt.Sprintf("parallel_%d", i))
			}

			compiled, err := g.Compile()
			if err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for b.Loop() {
				_, err := graph.Last(compiled.Run(context.Background(), nil))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkStateOperations measures state access performance
func BenchmarkStateOperations(b *testing.B) {
	b.Run("Set", func(b *testing.B) {
		state, err := graph.NewStateManager(0)
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for b.Loop() {
			state.Set(fmt.Sprintf("key_%d", b.N%100), b.N)
		}
	})

	b.Run("Get", func(b *testing.B) {
		state, err := graph.NewStateManager(0)
		if err != nil {
			b.Fatal(err)
		}
		for i := range 100 {
			state.Set(fmt.Sprintf("key_%d", i), i)
		}
		b.ResetTimer()
		for b.Loop() {
			_ = state.Get(fmt.Sprintf("key_%d", b.N%100))
		}
	})

	b.Run("GetAll", func(b *testing.B) {
		state, err := graph.NewStateManager(0)
		if err != nil {
			b.Fatal(err)
		}
		for i := range 100 {
			state.Set(fmt.Sprintf("key_%d", i), i)
		}
		b.ResetTimer()
		for b.Loop() {
			_ = state.GetAll()
		}
	})
}

// BenchmarkScheduler measures scheduler performance
func BenchmarkScheduler(b *testing.B) {
	state, err := graph.NewStateManager(0)
	if err != nil {
		b.Fatal(err)
	}
	g, err := graph.NewGraph(state)
	if err != nil {
		b.Fatal(err)
	}

	// Create a moderately complex graph
	for i := range 50 {
		err := g.AddNode(&graph.Node{
			Name: fmt.Sprintf("node_%d", i),
			RunFunc: func(ctx context.Context, s stateif.Writer) (*graph.NodeResult, error) {
				return &graph.NodeResult{Updates: map[string]any{}}, nil
			},
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	// Create some edges
	for i := range 49 {
		g.AddEdge(fmt.Sprintf("node_%d", i), fmt.Sprintf("node_%d", i+1))
	}
	g.AddEdge(graph.StartNode, "node_0")

	compiled, err := g.Compile()
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		_, err := graph.Last(compiled.Run(context.Background(), nil))
		if err != nil {
			b.Fatal(err)
		}
	}
}
