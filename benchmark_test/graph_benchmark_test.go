package benchmark_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Benchmark state operations

func BenchmarkState_Get(b *testing.B) {
	state := graph.NewGraphState(0)
	state.Set("key1", "value1")
	state.Set("key2", 42)
	state.Set("key3", []string{"a", "b", "c"})

	for b.Loop() {
		_ = state.Get("key1")
	}
}

func BenchmarkState_Set(b *testing.B) {
	state := graph.NewGraphState(0)

	for i := 0; b.Loop(); i++ {
		state.Set("key", i)
	}
}

func BenchmarkState_GetAll(b *testing.B) {
	state := graph.NewGraphState(0)
	for i := range 100 {
		state.Set(string(rune('a'+i%26)), i)
	}

	for b.Loop() {
		_ = state.GetAll()
	}
}

func BenchmarkState_ApplyUpdates(b *testing.B) {
	state := graph.NewGraphState(0)
	updates := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": []string{"a", "b", "c"},
	}

	for b.Loop() {
		state.ApplyUpdates(updates, nil)
	}
}

func BenchmarkState_ApplyUpdatesWithReducer(b *testing.B) {
	appendReducer := func(oldValue, newValue any) any {
		oldSlice, _ := oldValue.([]int)
		newSlice, _ := newValue.([]int)
		return append(oldSlice, newSlice...)
	}

	state := graph.NewGraphState(0)
	// Use BinaryOpChannel for accumulation with custom reducer
	state.AddChannel(channel.NewBinaryOpChannel("items", []int{}, appendReducer))

	updates := map[string]any{"items": []int{1, 2, 3}}

	for b.Loop() {
		state.ApplyUpdates(updates, nil)
	}
}

// Benchmark message operations

func BenchmarkState_AddMessages(b *testing.B) {
	state := graph.NewGraphState(0)
	msgs := []message.Message{
		message.NewHumanMessageFromText("Hello"),
		message.NewAIMessageFromText("Hi there"),
	}

	for b.Loop() {
		state.AddMessages(msgs)
	}
}

func BenchmarkState_MessagesSnapshot(b *testing.B) {
	state := graph.NewGraphState(0)
	for range 100 {
		state.AddMessages([]message.Message{
			message.NewHumanMessageFromText("Message"),
		})
	}

	for b.Loop() {
		_ = state.MessagesSnapshot()
	}
}

func BenchmarkState_MessagesWithCompaction(b *testing.B) {
	state := graph.NewGraphState(100) // Enable compaction with max 100 messages

	msgs := []message.Message{
		message.NewHumanMessageFromText("Hello"),
	}

	for b.Loop() {
		state.AddMessages(msgs)
	}
}

// Benchmark parallel state access

func BenchmarkState_ParallelReads(b *testing.B) {
	state := graph.NewGraphState(0)
	state.Set("key1", "value1")
	state.Set("key2", 42)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = state.Get("key1")
		}
	})
}

func BenchmarkState_ParallelWrites(b *testing.B) {
	state := graph.NewGraphState(0)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			state.Set("key", i)
			i++
		}
	})
}

func BenchmarkState_ParallelMixed(b *testing.B) {
	state := graph.NewGraphState(0)
	state.Set("key1", "value1")
	state.Set("key2", 42)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				state.Set("key", i)
			} else {
				_ = state.Get("key1")
			}
			i++
		}
	})
}

// Benchmark graph execution

func BenchmarkGraph_SimpleExecution(b *testing.B) {
	createSimpleGraph := func() *graph.CompiledGraph {
		state := graph.NewGraphState(0)
		state.Set("count", 0)
		g := graph.NewGraph(state)

		g.AddNode(&graph.Node{
			Name: "increment",
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				count, _ := s.Get("count").(int)
				return &graph.NodeResult{Updates: map[string]any{"count": count + 1}}, nil
			},
		})

		g.AddEdge(graph.StartNode, "increment")
		g.AddEdge("increment", graph.EndNode)

		compiled, _ := g.Compile()
		return compiled
	}

	for b.Loop() {
		graph := createSimpleGraph()
		_, _ = graph.Invoke(context.Background(), nil)
	}
}

func BenchmarkGraph_LinearChain(b *testing.B) {
	createChainGraph := func(length int) *graph.CompiledGraph {
		state := graph.NewGraphState(0)
		state.Set("value", 0) // Fixed
		g := graph.NewGraph(state)

		for i := range length {
			name := string(rune('a' + i%26))
			g.AddNode(&graph.Node{
				Name: name,
				RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
					val, _ := s.Get("value").(int)
					return &graph.NodeResult{Updates: map[string]any{"value": val + 1}}, nil
				},
			})

			if i == 0 {
				g.AddEdge(graph.StartNode, name)
			} else {
				prevName := string(rune('a' + (i-1)%26))
				g.AddEdge(prevName, name)
			}

			if i == length-1 {
				g.AddEdge(name, graph.EndNode)
			}
		}

		compiled, _ := g.Compile()
		return compiled
	}

	b.Run("Length5", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			graph := createChainGraph(5)
			_, _ = graph.Invoke(context.Background(), nil)
		}
	})

	b.Run("Length10", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			graph := createChainGraph(10)
			_, _ = graph.Invoke(context.Background(), nil)
		}
	})

	b.Run("Length20", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			graph := createChainGraph(20)
			_, _ = graph.Invoke(context.Background(), nil)
		}
	})
}

func BenchmarkGraph_ParallelNodes(b *testing.B) {
	createParallelGraph := func(parallelism int) *graph.CompiledGraph {
		appendReducer := func(oldValue, newValue any) any {
			oldSlice, _ := oldValue.([]int)
			newSlice, _ := newValue.([]int)
			return append(oldSlice, newSlice...)
		}

		state := graph.NewGraphState(0)
		// Use BinaryOpChannel for results accumulation
		state.AddChannel(channel.NewBinaryOpChannel("results", []int{}, appendReducer))
		g := graph.NewGraph(state)

		g.AddNode(&graph.Node{
			Name: "start",
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})

		for i := range parallelism {
			name := string(rune('a' + i%26))
			idx := i
			g.AddNode(&graph.Node{
				Name: name,
				RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
					return &graph.NodeResult{Updates: map[string]any{"results": []int{idx}}}, nil
				},
			})
			g.AddEdge("start", name)
			g.AddEdge(name, graph.EndNode)
		}

		g.AddEdge(graph.StartNode, "start")

		compiled, _ := g.Compile()
		return compiled
	}

	b.Run("Parallel2", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			graph := createParallelGraph(2)
			_, _ = graph.Invoke(context.Background(), nil)
		}
	})

	b.Run("Parallel5", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			graph := createParallelGraph(5)
			_, _ = graph.Invoke(context.Background(), nil)
		}
	})

	b.Run("Parallel10", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			graph := createParallelGraph(10)
			_, _ = graph.Invoke(context.Background(), nil)
		}
	})
}

func BenchmarkGraph_ConditionalRouting(b *testing.B) {
	createConditionalGraph := func() *graph.CompiledGraph {
		state := graph.NewGraphState(0)
		state.Set("route", "left") // Fixed
		g := graph.NewGraph(state)

		g.AddNode(&graph.Node{
			Name: "router",
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		})

		g.AddNode(&graph.Node{
			Name: "left",
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				return &graph.NodeResult{Updates: map[string]any{"result": "left"}}, nil
			},
		})

		g.AddNode(&graph.Node{
			Name: "right",
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				return &graph.NodeResult{Updates: map[string]any{"result": "right"}}, nil
			},
		})

		g.AddEdge(graph.StartNode, "router")
		g.AddConditionalEdges("router", func(ctx context.Context, s graph.StateReader) []string {
			route, _ := s.Get("route").(string)
			return []string{route}
		}, []string{"left", "right"})

		g.AddEdge("left", graph.EndNode)
		g.AddEdge("right", graph.EndNode)

		compiled, _ := g.Compile()
		return compiled
	}

	for b.Loop() {
		graph := createConditionalGraph()
		_, _ = graph.Invoke(context.Background(), nil)
	}
}

// Benchmark message cloning (critical path in state operations)

func BenchmarkCloneMessages_Small(b *testing.B) {
	msgs := []message.Message{
		message.NewHumanMessageFromText("Hello"),
		message.NewAIMessageFromText("Hi there"),
	}

	for b.Loop() {
		_ = cloneMessagesHelper(msgs)
	}
}

func BenchmarkCloneMessages_Large(b *testing.B) {
	msgs := make([]message.Message, 100)
	for i := range 100 {
		msgs[i] = message.NewHumanMessageFromText("Message content here")
	}

	for b.Loop() {
		_ = cloneMessagesHelper(msgs)
	}
}

// Benchmark graph compilation

func BenchmarkGraph_Compile(b *testing.B) {
	createGraph := func() *graph.Graph {
		state := graph.NewGraphState(0)
		g := graph.NewGraph(state)

		for i := 0; i < 10; i++ {
			name := string(rune('a' + i))
			g.AddNode(&graph.Node{
				Name: name,
				RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
					return &graph.NodeResult{}, nil
				},
			})
			if i > 0 {
				prevName := string(rune('a' + i - 1))
				g.AddEdge(prevName, name)
			}
		}
		g.AddEdge(graph.StartNode, "a")
		g.AddEdge("j", graph.EndNode)

		return g
	}

	for b.Loop() {
		g := createGraph()
		_, _ = g.Compile()
	}
}

// Helper function to clone messages
func cloneMessagesHelper(msgs []message.Message) []message.Message {
	if len(msgs) == 0 {
		return nil
	}
	clone := make([]message.Message, len(msgs))
	copy(clone, msgs)
	return clone
}
