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

// Benchmark state operations with state v3 API

func BenchmarkState_GetFromView(b *testing.B) {
	st := state.NewState()

	// Define and register typed keys with default values
	key1 := state.NewKey("key1", "")
	key2 := state.NewKey("key2", 0)
	key3 := state.NewKey("key3", []string{})

	state.Register(st, key1)
	state.Register(st, key2)
	state.Register(st, key3)
	state.RegisterList(st, state.MessagesKey)

	// Set values
	ctx := context.Background()
	_ = st.ApplyUpdates(ctx, map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": []string{"a", "b", "c"},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := st.Snapshot()
		view := state.NewReadView(snap)
		_ = state.GetFromView(view, key1)
	}
}

func BenchmarkState_ApplyUpdates(b *testing.B) {
	st := state.NewState()

	key1 := state.NewKey("key1", "")
	key2 := state.NewKey("key2", 0)
	key3 := state.NewKey("key3", []string{})

	state.Register(st, key1)
	state.Register(st, key2)
	state.Register(st, key3)
	state.RegisterList(st, state.MessagesKey)

	updates := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": []string{"a", "b", "c"},
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = st.ApplyUpdates(ctx, updates)
	}
}

// Benchmark message operations

func BenchmarkState_AddMessages(b *testing.B) {
	st := state.NewState()
	state.RegisterList(st, state.MessagesKey)

	msgs := []state.ExecutionResult{
		*state.NewExecutionResult(message.NewHumanMessageFromText("Hello"), "", ""),
		*state.NewExecutionResult(message.NewAIMessageFromText("Hi there"), "", ""),
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = st.ApplyUpdates(ctx, map[string]any{
			state.MessagesKey.Key.Name(): msgs,
		})
	}
}

func BenchmarkState_GetMessages(b *testing.B) {
	st := state.NewState()
	state.RegisterList(st, state.MessagesKey)

	ctx := context.Background()
	// Add 100 messages
	for i := 0; i < 100; i++ {
		msgs := []state.ExecutionResult{
			*state.NewExecutionResult(message.NewHumanMessageFromText("Message"), "", ""),
		}
		_ = st.ApplyUpdates(ctx, map[string]any{
			state.MessagesKey.Key.Name(): msgs,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		snap := st.Snapshot()
		view := state.NewReadView(snap)
		_ = state.GetFromView(view, state.MessagesKey.Key)
	}
}

// Benchmark graph execution

func BenchmarkGraph_SimpleExecution(b *testing.B) {
	countKey := state.NewKey("count", 0)

	createSimpleGraph := func() graph.MessageRunnable {
		st := state.NewState()
		state.Register(st, countKey)
		state.RegisterList(st, state.MessagesKey)

		ctx := context.Background()
		_ = st.ApplyUpdates(ctx, map[string]any{"count": 0})

		g, _ := graph.NewGraph(st)

		g.AddNode(&graph.Node{
			Name: "increment",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				count := state.GetFromView(view, countKey)
				return &graph.NodeResult{Updates: map[string]any{"count": count + 1}}, nil
			},
		})

		g.AddEdge(graph.StartNode, "increment")
		g.AddEdge("increment", graph.EndNode)

		compiled, _ := exec.CompileGraph(g)
		return compiled
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiled := createSimpleGraph()
		for range compiled.Run(context.Background(), nil) {
		}
	}
}

func BenchmarkGraph_LinearChain(b *testing.B) {
	valueKey := state.NewKey("value", 0)

	createChainGraph := func(length int) graph.MessageRunnable {
		st := state.NewState()
		state.Register(st, valueKey)
		state.RegisterList(st, state.MessagesKey)

		ctx := context.Background()
		_ = st.ApplyUpdates(ctx, map[string]any{"value": 0})

		g, _ := graph.NewGraph(st)

		for i := 0; i < length; i++ {
			name := fmt.Sprintf("node_%d", i)
			g.AddNode(&graph.Node{
				Name: name,
				RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
					val := state.GetFromView(view, valueKey)
					return &graph.NodeResult{Updates: map[string]any{"value": val + 1}}, nil
				},
			})

			if i == 0 {
				g.AddEdge(graph.StartNode, name)
			} else {
				prevName := fmt.Sprintf("node_%d", i-1)
				g.AddEdge(prevName, name)
			}

			if i == length-1 {
				g.AddEdge(name, graph.EndNode)
			}
		}

		compiled, _ := exec.CompileGraph(g)
		return compiled
	}

	b.Run("Length5", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled := createChainGraph(5)
			for range compiled.Run(context.Background(), nil) {
			}
		}
	})

	b.Run("Length10", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled := createChainGraph(10)
			for range compiled.Run(context.Background(), nil) {
			}
		}
	})
}

func BenchmarkGraph_Compile(b *testing.B) {
	createGraph := func() *graph.Graph {
		st := state.NewState()
		state.RegisterList(st, state.MessagesKey)

		g, _ := graph.NewGraph(st)

		for i := 0; i < 10; i++ {
			name := fmt.Sprintf("node_%d", i)
			g.AddNode(&graph.Node{
				Name: name,
				RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
					return &graph.NodeResult{}, nil
				},
			})
			if i > 0 {
				prevName := fmt.Sprintf("node_%d", i-1)
				g.AddEdge(prevName, name)
			}
		}
		g.AddEdge(graph.StartNode, "node_0")
		g.AddEdge("node_9", graph.EndNode)

		return g
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g := createGraph()
		_, _ = exec.CompileGraph(g)
	}
}
