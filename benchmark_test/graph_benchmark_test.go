package benchmark_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/agent"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
)

// Benchmark state operations with state v3 API

func BenchmarkState_GetFromView(b *testing.B) {
	builder := state.NewManagerBuilder()

	// Define and register typed keys with default values
	key1 := state.NewKey("key1", "")
	key2 := state.NewKey("key2", 0)
	key3 := state.NewKey("key3", []string{})

	state.RegisterKey(builder, key1)
	state.RegisterKey(builder, key2)
	state.RegisterKey(builder, key3)
	state.RegisterListKey(builder, agent.MessagesKey)
	mgr := builder.Build()

	// Set values
	ctx := context.Background()
	_ = state.Set(ctx, mgr, key1, "value1")
	_ = state.Set(ctx, mgr, key2, 42)
	_ = state.Set(ctx, mgr, key3, []string{"a", "b", "c"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		view, _ := mgr.CreateReadView(ctx)
		_ = state.GetFromView(view, key1)
	}
}

func BenchmarkState_ApplyUpdates(b *testing.B) {
	builder := state.NewManagerBuilder()

	key1 := state.NewKey("key1", "")
	key2 := state.NewKey("key2", 0)
	key3 := state.NewKey("key3", []string{})

	state.RegisterKey(builder, key1)
	state.RegisterKey(builder, key2)
	state.RegisterKey(builder, key3)
	state.RegisterListKey(builder, agent.MessagesKey)
	mgr := builder.Build()

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = state.Set(ctx, mgr, key1, "value1")
		_ = state.Set(ctx, mgr, key2, 42)
		_ = state.Set(ctx, mgr, key3, []string{"a", "b", "c"})
	}
}

// Benchmark message operations

func BenchmarkState_AddMessages(b *testing.B) {
	builder := state.NewManagerBuilder()
	state.RegisterListKey(builder, agent.MessagesKey)
	mgr := builder.Build()

	msgs := []message.Message{
		message.NewHumanMessageFromText("Hello"),
		message.NewAIMessageFromText("Hi there"),
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, msg := range msgs {
			_ = state.Append(ctx, mgr, agent.MessagesKey, msg)
		}
	}
}

func BenchmarkState_GetMessages(b *testing.B) {
	builder := state.NewManagerBuilder()
	state.RegisterListKey(builder, agent.MessagesKey)
	mgr := builder.Build()

	ctx := context.Background()
	// Add 100 messages
	for i := 0; i < 100; i++ {
		var msg message.Message = message.NewHumanMessageFromText("Message")
		_ = state.Append(ctx, mgr, agent.MessagesKey, msg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		view, _ := mgr.CreateReadView(ctx)
		_ = state.GetFromView(view, agent.MessagesKey.Key)
	}
}

// Benchmark graph execution

func BenchmarkGraph_SimpleExecution(b *testing.B) {
	countKey := state.NewKey("count", 0)

	createSimpleGraph := func() graph.Runnable[[]message.Message, message.Message] {
		builder := state.NewManagerBuilder()
		state.RegisterKey(builder, countKey)
		state.RegisterListKey(builder, agent.MessagesKey)
		mgr := builder.Build()

		ctx := context.Background()
		_ = state.Set(ctx, mgr, countKey, 0)

		g, _ := graph.NewGraph(mgr)

		g.AddNode(&graph.BaseNode{
			NodeName:        "increment",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				count := state.GetFromView(view, countKey)
				updates := state.Updates{countKey.Name(): count + 1}
				return []string{graph.EndNode}, updates, nil
			},
		})

		g.SetEntryPoint("increment")

		compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())
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

	createChainGraph := func(length int) graph.Runnable[[]message.Message, message.Message] {
		builder := state.NewManagerBuilder()
		state.RegisterKey(builder, valueKey)
		state.RegisterListKey(builder, agent.MessagesKey)
		mgr := builder.Build()

		ctx := context.Background()
		_ = state.Set(ctx, mgr, valueKey, 0)

		g, _ := graph.NewGraph(mgr)

		for i := 0; i < length; i++ {
			name := fmt.Sprintf("node_%d", i)
			nextNode := graph.EndNode
			if i < length-1 {
				nextNode = fmt.Sprintf("node_%d", i+1)
			}
			g.AddNode(&graph.BaseNode{
				NodeName:        name,
				DeclaredTargets: []string{nextNode},
				Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
					val := state.GetFromView(view, valueKey)
					updates := state.Updates{valueKey.Name(): val + 1}
					return []string{nextNode}, updates, nil
				},
			})

			if i == 0 {
				g.SetEntryPoint(name)
			}
		}

		compiled, _ := graph.Compile(g, graph.NewMessagePregelExecutor())
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
		builder := state.NewManagerBuilder()
		state.RegisterListKey(builder, agent.MessagesKey)
		mgr := builder.Build()

		g, _ := graph.NewGraph(mgr)

		for i := 0; i < 10; i++ {
			name := fmt.Sprintf("node_%d", i)
			nextNode := graph.EndNode
			if i < 9 {
				nextNode = fmt.Sprintf("node_%d", i+1)
			}
			g.AddNode(&graph.BaseNode{
				NodeName:        name,
				DeclaredTargets: []string{nextNode},
				Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
					return []string{nextNode}, nil, nil
				},
			})
		}
		g.SetEntryPoint("node_0")

		return g
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g := createGraph()
		_, _ = graph.Compile(g, graph.NewMessagePregelExecutor())
	}
}
