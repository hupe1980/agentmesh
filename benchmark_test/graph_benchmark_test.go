package benchmark_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/message"
)

// Common keys for benchmarks
var (
	CountKey = graph.NewKey[int]("count")
	ValueKey = graph.NewKey[int]("value")
	TextKey  = graph.NewKey[string]("text")
)

// Benchmark graph execution

func BenchmarkGraph_SimpleExecution(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder := graph.New[int, int](CountKey)

		builder.Node("increment", func(ctx context.Context, scope graph.Scope[int]) (*graph.Command, error) {
			count := graph.Get(scope, CountKey)
			return graph.Set(CountKey, count+1).End()
		}, graph.END)

		builder.Start("increment")

		g, _ := builder.Build()
		for range g.Run(ctx, 0) {
		}
	}
}

func BenchmarkGraph_LinearChain(b *testing.B) {
	createChainGraph := func(length int) *graph.Graph[int, int] {
		builder := graph.New[int, int](ValueKey)

		for i := range length {
			name := fmt.Sprintf("node_%d", i)
			nextNode := graph.END
			if i < length-1 {
				nextNode = fmt.Sprintf("node_%d", i+1)
			}

			// Capture loop variables
			nodeName := name
			next := nextNode

			builder.Node(nodeName, func(ctx context.Context, scope graph.Scope[int]) (*graph.Command, error) {
				val := graph.Get(scope, ValueKey)
				return graph.Set(ValueKey, val+1).To(next)
			}, next)
		}

		builder.Start("node_0")

		g, _ := builder.Build()
		return g
	}

	b.Run("Length5", func(b *testing.B) {
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			g := createChainGraph(5)
			for range g.Run(ctx, 0) {
			}
		}
	})

	b.Run("Length10", func(b *testing.B) {
		ctx := context.Background()
		b.ResetTimer()
		for b.Loop() {
			g := createChainGraph(10)
			for range g.Run(ctx, 0) {
			}
		}
	})

	b.Run("Length20", func(b *testing.B) {
		ctx := context.Background()
		b.ResetTimer()
		for b.Loop() {
			g := createChainGraph(20)
			for range g.Run(ctx, 0) {
			}
		}
	})
}

func BenchmarkGraph_Build(b *testing.B) {

	for b.Loop() {
		builder := graph.New[int, int](ValueKey)

		for j := range 10 {
			name := fmt.Sprintf("node_%d", j)
			nextNode := graph.END
			if j < 9 {
				nextNode = fmt.Sprintf("node_%d", j+1)
			}

			nodeName := name
			next := nextNode

			builder.Node(nodeName, func(ctx context.Context, scope graph.Scope[int]) (*graph.Command, error) {
				return graph.Cmd().To(next)
			}, next)
		}

		builder.Start("node_0")
		_, _ = builder.Build()
	}
}

func BenchmarkGraph_ParallelNodes(b *testing.B) {
	ctx := context.Background()

	for b.Loop() {
		builder := graph.New[int, int](ValueKey)

		builder.Node("start", func(ctx context.Context, scope graph.Scope[int]) (*graph.Command, error) {
			return graph.Cmd().To("worker1", "worker2", "worker3")
		}, "worker1", "worker2", "worker3")

		for _, name := range []string{"worker1", "worker2", "worker3"} {
			workerName := name
			builder.Node(workerName, func(ctx context.Context, scope graph.Scope[int]) (*graph.Command, error) {
				val := graph.Get(scope, ValueKey)
				return graph.Set(ValueKey, val+1).To("merge")
			}, "merge")
		}

		builder.Node("merge", func(ctx context.Context, scope graph.Scope[int]) (*graph.Command, error) {
			val := graph.Get(scope, ValueKey)
			return graph.Set(ValueKey, val).End()
		}, graph.END)

		builder.Start("start")

		g, _ := builder.Build()
		for range g.Run(ctx, 0) {
		}
	}
}

// Benchmark message-based graph execution

var MessagesKey = graph.NewListKey[message.Message]("messages")

func BenchmarkGraph_MessageExecution(b *testing.B) {
	ctx := context.Background()

	for b.Loop() {
		builder := graph.New[[]message.Message, message.Message](MessagesKey)

		builder.Node("process", func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
			var msg message.Message = message.NewAIMessageFromText("Response")
			return graph.Set(MessagesKey, []message.Message{msg}).End()
		}, graph.END)

		builder.Start("process")

		g, _ := builder.Build()

		input := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		for range g.Run(ctx, input) {
		}
	}
}

func BenchmarkGraph_MessageChain(b *testing.B) {
	ctx := context.Background()

	b.Run("3Nodes", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			builder := graph.New[[]message.Message, message.Message](MessagesKey)

			builder.Node("node1", func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
				var msg message.Message = message.NewAIMessageFromText("From node1")
				return graph.Set(MessagesKey, []message.Message{msg}).To("node2")
			}, "node2")

			builder.Node("node2", func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
				var msg message.Message = message.NewAIMessageFromText("From node2")
				return graph.Set(MessagesKey, []message.Message{msg}).To("node3")
			}, "node3")

			builder.Node("node3", func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
				var msg message.Message = message.NewAIMessageFromText("Final")
				return graph.Set(MessagesKey, []message.Message{msg}).End()
			}, graph.END)

			builder.Start("node1")

			g, _ := builder.Build()

			input := []message.Message{
				message.NewHumanMessageFromText("Start"),
			}

			for range g.Run(ctx, input) {
			}
		}
	})
}

func BenchmarkGraph_PrebuiltExecution(b *testing.B) {
	// Build graph once outside the benchmark loop
	builder := graph.New[int, int](ValueKey)

	builder.Node("process", func(ctx context.Context, scope graph.Scope[int]) (*graph.Command, error) {
		val := graph.Get(scope, ValueKey)
		return graph.Set(ValueKey, val+1).End()
	}, graph.END)

	builder.Start("process")

	g, _ := builder.Build()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range g.Run(ctx, 0) {
		}
	}
}

func BenchmarkGraph_PrebuiltMessageExecution(b *testing.B) {
	// Build graph once outside the benchmark loop
	builder := graph.New[[]message.Message, message.Message](MessagesKey)

	builder.Node("process", func(ctx context.Context, scope graph.Scope[message.Message]) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("Response")
		return graph.Set(MessagesKey, []message.Message{msg}).End()
	}, graph.END)

	builder.Start("process")

	g, _ := builder.Build()

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range g.Run(ctx, input) {
		}
	}
}
