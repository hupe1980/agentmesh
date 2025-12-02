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
	CountKey = graph.NewKey("count", 0)
	ValueKey = graph.NewKey("value", 0)
	TextKey  = graph.NewKey("text", "")
)

// Benchmark graph execution

func BenchmarkGraph_SimpleExecution(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g := graph.New[int, int](CountKey)

		g.Node("increment", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			count := graph.Get(view, CountKey)
			return graph.Set(CountKey, count+1).End()
		}, graph.END)

		g.Start("increment")

		compiled, _ := g.Build()
		for range compiled.Run(ctx, 0) {
		}
	}
}

func BenchmarkGraph_LinearChain(b *testing.B) {
	createChainGraph := func(length int) *graph.CompiledGraph[int, int] {
		g := graph.New[int, int](ValueKey)

		for i := range length {
			name := fmt.Sprintf("node_%d", i)
			nextNode := graph.END
			if i < length-1 {
				nextNode = fmt.Sprintf("node_%d", i+1)
			}

			// Capture loop variables
			nodeName := name
			next := nextNode

			g.Node(nodeName, func(ctx context.Context, view graph.View) (*graph.Command, error) {
				val := graph.Get(view, ValueKey)
				return graph.Set(ValueKey, val+1).To(next)
			}, next)
		}

		g.Start("node_0")

		compiled, _ := g.Build()
		return compiled
	}

	b.Run("Length5", func(b *testing.B) {
		ctx := context.Background()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			compiled := createChainGraph(5)
			for range compiled.Run(ctx, 0) {
			}
		}
	})

	b.Run("Length10", func(b *testing.B) {
		ctx := context.Background()
		b.ResetTimer()
		for b.Loop() {
			compiled := createChainGraph(10)
			for range compiled.Run(ctx, 0) {
			}
		}
	})

	b.Run("Length20", func(b *testing.B) {
		ctx := context.Background()
		b.ResetTimer()
		for b.Loop() {
			compiled := createChainGraph(20)
			for range compiled.Run(ctx, 0) {
			}
		}
	})
}

func BenchmarkGraph_Build(b *testing.B) {

	for b.Loop() {
		g := graph.New[int, int](ValueKey)

		for j := range 10 {
			name := fmt.Sprintf("node_%d", j)
			nextNode := graph.END
			if j < 9 {
				nextNode = fmt.Sprintf("node_%d", j+1)
			}

			nodeName := name
			next := nextNode

			g.Node(nodeName, func(ctx context.Context, view graph.View) (*graph.Command, error) {
				return graph.Cmd().To(next)
			}, next)
		}

		g.Start("node_0")
		_, _ = g.Build()
	}
}

func BenchmarkGraph_ParallelNodes(b *testing.B) {
	ctx := context.Background()

	for b.Loop() {
		g := graph.New[int, int](ValueKey)

		g.Node("start", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			return graph.Cmd().To("worker1", "worker2", "worker3")
		}, "worker1", "worker2", "worker3")

		for _, name := range []string{"worker1", "worker2", "worker3"} {
			workerName := name
			g.Node(workerName, func(ctx context.Context, view graph.View) (*graph.Command, error) {
				val := graph.Get(view, ValueKey)
				return graph.Set(ValueKey, val+1).To("merge")
			}, "merge")
		}

		g.Node("merge", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			val := graph.Get(view, ValueKey)
			return graph.Set(ValueKey, val).End()
		}, graph.END)

		g.Start("start")

		compiled, _ := g.Build()
		for range compiled.Run(ctx, 0) {
		}
	}
}

// Benchmark message-based graph execution

var MessagesKey = graph.NewListKey[message.Message]("messages")

func BenchmarkGraph_MessageExecution(b *testing.B) {
	ctx := context.Background()

	for b.Loop() {
		g := graph.New[[]message.Message, message.Message](MessagesKey)

		g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
			var msg message.Message = message.NewAIMessageFromText("Response")
			return graph.Append(MessagesKey, msg).End()
		}, graph.END)

		g.Start("process")

		compiled, _ := g.Build()

		input := []message.Message{
			message.NewHumanMessageFromText("Hello"),
		}

		for range compiled.Run(ctx, input) {
		}
	}
}

func BenchmarkGraph_MessageChain(b *testing.B) {
	ctx := context.Background()

	b.Run("3Nodes", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			g := graph.New[[]message.Message, message.Message](MessagesKey)

			g.Node("node1", func(ctx context.Context, view graph.View) (*graph.Command, error) {
				var msg message.Message = message.NewAIMessageFromText("From node1")
				return graph.Append(MessagesKey, msg).To("node2")
			}, "node2")

			g.Node("node2", func(ctx context.Context, view graph.View) (*graph.Command, error) {
				var msg message.Message = message.NewAIMessageFromText("From node2")
				return graph.Append(MessagesKey, msg).To("node3")
			}, "node3")

			g.Node("node3", func(ctx context.Context, view graph.View) (*graph.Command, error) {
				var msg message.Message = message.NewAIMessageFromText("Final")
				return graph.Append(MessagesKey, msg).End()
			}, graph.END)

			g.Start("node1")

			compiled, _ := g.Build()

			input := []message.Message{
				message.NewHumanMessageFromText("Start"),
			}

			for range compiled.Run(ctx, input) {
			}
		}
	})
}

func BenchmarkGraph_PrebuiltExecution(b *testing.B) {
	// Build graph once outside the benchmark loop
	g := graph.New[int, int](ValueKey)

	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		val := graph.Get(view, ValueKey)
		return graph.Set(ValueKey, val+1).End()
	}, graph.END)

	g.Start("process")

	compiled, _ := g.Build()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range compiled.Run(ctx, 0) {
		}
	}
}

func BenchmarkGraph_PrebuiltMessageExecution(b *testing.B) {
	// Build graph once outside the benchmark loop
	g := graph.New[[]message.Message, message.Message](MessagesKey)

	g.Node("process", func(ctx context.Context, view graph.View) (*graph.Command, error) {
		var msg message.Message = message.NewAIMessageFromText("Response")
		return graph.Append(MessagesKey, msg).End()
	}, graph.END)

	g.Start("process")

	compiled, _ := g.Build()

	ctx := context.Background()
	input := []message.Message{
		message.NewHumanMessageFromText("Hello"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for range compiled.Run(ctx, input) {
		}
	}
}
