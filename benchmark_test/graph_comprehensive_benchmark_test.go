package benchmark_testpackage benchmark_test



import (import (

	"context"	"context"

	"fmt"	"fmt"

	"testing"	"testing"



	"github.com/hupe1980/agentmesh/pkg/exec"	"github.com/hupe1980/agentmesh/pkg/exec"

	"github.com/hupe1980/agentmesh/pkg/graph"	"github.com/hupe1980/agentmesh/pkg/graph"

	"github.com/hupe1980/agentmesh/pkg/message"	"github.com/hupe1980/agentmesh/pkg/message"

	"github.com/hupe1980/agentmesh/pkg/state"	"github.com/hupe1980/agentmesh/pkg/state"

))



// BenchmarkComprehensiveWorkflow benchmarks a realistic agent workflow// BenchmarkComprehensiveWorkflow benchmarks a realistic agent workflow

func BenchmarkComprehensiveWorkflow(b *testing.B) {func BenchmarkComprehensiveWorkflow(b *testing.B) {

	ctx := context.Background()	ctx := context.Background()



	// Define typed keys	stateManager, err := newTestState()

	processedKey := state.NewKey[int]("processed")	if err != nil {

	analyzedKey := state.NewKey[int]("analyzed")		b.Fatal(err)

	validKey := state.NewKey[bool]("valid")	}

	resultKey := state.NewKey[string]("result")

	// Create a realistic multi-step workflow

	st := state.NewState()	g, err := graph.NewGraph(stateManager)

	state.Register(st, processedKey)	if err != nil {

	state.Register(st, analyzedKey)		b.Fatal(err)

	state.Register(st, validKey)	}

	state.Register(st, resultKey)

	state.Register(st, state.MessagesKey.Key)	// Node 1: Input processing

	g.AddNode(&graph.Node{

	// Create a realistic multi-step workflow		Name: "preprocess",

	g, err := graph.NewGraph(st)		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

	if err != nil {			msgs := s.MessagesSnapshot()

		b.Fatal(err)			return &graph.NodeResult{

	}				Updates: map[string]any{"processed": len(msgs)},

			}, nil

	// Node 1: Input processing		},

	g.AddNode(&graph.Node{	})

		Name: "preprocess",

		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {	// Node 2: Analysis (parallel with node 3)

			msgs := state.GetMessages(view)	g.AddNode(&graph.Node{

			return &graph.NodeResult{		Name: "analyze",

				Updates: map[string]any{"processed": len(msgs)},		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

			}, nil			count := s.Get("processed").(int)

		},			return &graph.NodeResult{

	})				Updates: map[string]any{"analyzed": count * 2},

			}, nil

	// Node 2: Analysis (parallel with node 3)		},

	g.AddNode(&graph.Node{	})

		Name: "analyze",

		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {	// Node 3: Validate (parallel with node 2)

			count := state.GetFromView(view, processedKey)	g.AddNode(&graph.Node{

			return &graph.NodeResult{		Name: "validate",

				Updates: map[string]any{"analyzed": count * 2},		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

			}, nil			count := s.Get("processed").(int)

		},			return &graph.NodeResult{

	})				Updates: map[string]any{"valid": count > 0},

			}, nil

	// Node 3: Validate (parallel with node 2)		},

	g.AddNode(&graph.Node{	})

		Name: "validate",

		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {	// Node 4: Aggregate results

			count := state.GetFromView(view, processedKey)	g.AddNode(&graph.Node{

			return &graph.NodeResult{		Name: "aggregate",

				Updates: map[string]any{"valid": count > 0},		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

			}, nil			analyzed := s.Get("analyzed").(int)

		},			valid := s.Get("valid").(bool)

	})			return &graph.NodeResult{

				Updates: map[string]any{

	// Node 4: Aggregate results					"result": fmt.Sprintf("analyzed=%d valid=%v", analyzed, valid),

	g.AddNode(&graph.Node{				},

		Name: "aggregate",			}, nil

		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {		},

			analyzed := state.GetFromView(view, analyzedKey)	})

			valid := state.GetFromView(view, validKey)

			return &graph.NodeResult{	// Build topology

				Updates: map[string]any{	g.AddEdge(graph.StartNode, "preprocess")

					"result": fmt.Sprintf("analyzed=%d valid=%v", analyzed, valid),	g.AddEdge("preprocess", "analyze")

				},	g.AddEdge("preprocess", "validate")

			}, nil	g.AddEdge("analyze", "aggregate")

		},	g.AddEdge("validate", "aggregate")

	})	g.AddEdge("aggregate", graph.EndNode)



	// Build topology	compiled, err := exec.CompileGraph(g)

	g.AddEdge(graph.StartNode, "preprocess")	if err != nil {

	g.AddEdge("preprocess", "analyze")		b.Fatal(err)

	g.AddEdge("preprocess", "validate")	}

	g.AddEdge("analyze", "aggregate")

	g.AddEdge("validate", "aggregate")	initialMessages := []message.Message{

	g.AddEdge("aggregate", graph.EndNode)		message.NewHumanMessageFromText("test input"),

	}

	compiled, err := exec.CompileGraph(g)

	if err != nil {	b.ReportAllocs()

		b.Fatal(err)

	}	for b.Loop() {

		for range compiled.Run(ctx, initialMessages) {

	initialMessages := []message.Message{		}

		message.NewHumanMessageFromText("test input"),	}

	}}



	b.ReportAllocs()// BenchmarkDeepChain benchmarks a long sequential chain

	b.ResetTimer()func BenchmarkDeepChain(b *testing.B) {

	ctx := context.Background()

	for i := 0; i < b.N; i++ {

		for range compiled.Run(ctx, initialMessages) {	depths := []int{5, 10, 20, 50}

		}

	}	for _, depth := range depths {

}		b.Run(fmt.Sprintf("depth-%d", depth), func(b *testing.B) {

			stateManager, err := newTestState()

// BenchmarkDeepChain benchmarks a long sequential chain			if err != nil {

func BenchmarkDeepChain(b *testing.B) {				b.Fatal(err)

	ctx := context.Background()			}



	depths := []int{5, 10, 20, 50}			g, err := graph.NewGraph(stateManager)

			if err != nil {

	for _, depth := range depths {				b.Fatal(err)

		b.Run(fmt.Sprintf("depth-%d", depth), func(b *testing.B) {			}

			stepKey := state.NewKey[string]("step")

						// Create chain of nodes

			st := state.NewState()			for i := range depth {

			state.Register(st, stepKey)				nodeName := fmt.Sprintf("node-%d", i)

			state.Register(st, state.MessagesKey.Key)				g.AddNode(&graph.Node{

					Name: nodeName,

			g, err := graph.NewGraph(st)					RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

			if err != nil {						return &graph.NodeResult{

				b.Fatal(err)							Updates: map[string]any{"step": nodeName},

			}						}, nil

					},

			// Create chain of nodes				})

			for i := 0; i < depth; i++ {

				nodeName := fmt.Sprintf("node-%d", i)				if i == 0 {

				g.AddNode(&graph.Node{					g.AddEdge(graph.StartNode, nodeName)

					Name: nodeName,				} else {

					RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {					g.AddEdge(fmt.Sprintf("node-%d", i-1), nodeName)

						return &graph.NodeResult{				}

							Updates: map[string]any{"step": nodeName},

						}, nil				if i == depth-1 {

					},					g.AddEdge(nodeName, graph.EndNode)

				})				}

			}

				if i == 0 {

					g.AddEdge(graph.StartNode, nodeName)			compiled, err := exec.CompileGraph(g)

				} else {			if err != nil {

					g.AddEdge(fmt.Sprintf("node-%d", i-1), nodeName)				b.Fatal(err)

				}			}



				if i == depth-1 {			initialMessages := []message.Message{

					g.AddEdge(nodeName, graph.EndNode)				message.NewHumanMessageFromText("test"),

				}			}

			}

			b.ResetTimer()

			compiled, err := exec.CompileGraph(g)			b.ReportAllocs()

			if err != nil {

				b.Fatal(err)			for b.Loop() {

			}				for range compiled.Run(ctx, initialMessages) {

				}

			initialMessages := []message.Message{			}

				message.NewHumanMessageFromText("test"),		})

			}	}

}

			b.ReportAllocs()

			b.ResetTimer()// BenchmarkWideParallel benchmarks wide parallel execution

func BenchmarkWideParallel(b *testing.B) {

			for i := 0; i < b.N; i++ {	ctx := context.Background()

				for range compiled.Run(ctx, initialMessages) {

				}	widths := []int{2, 5, 10, 20}

			}

		})	for _, width := range widths {

	}		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {

}			stateManager, err := newTestState()

			if err != nil {

// BenchmarkWideParallel benchmarks wide parallel execution				b.Fatal(err)

func BenchmarkWideParallel(b *testing.B) {			}

	ctx := context.Background()

			g, err := graph.NewGraph(stateManager)

	widths := []int{2, 5, 10, 20}			if err != nil {

				b.Fatal(err)

	for _, width := range widths {			}

		b.Run(fmt.Sprintf("width-%d", width), func(b *testing.B) {

			st := state.NewState()			// Create parallel nodes

			state.Register(st, state.MessagesKey.Key)			for i := range width {

							nodeName := fmt.Sprintf("parallel-%d", i)

			// Register all parallel node keys				g.AddNode(&graph.Node{

			for i := 0; i < width; i++ {					Name: nodeName,

				nodeName := fmt.Sprintf("parallel-%d", i)					RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

				key := state.NewKey[int](nodeName)						return &graph.NodeResult{

				state.Register(st, key)							Updates: map[string]any{nodeName: i},

			}						}, nil

					},

			g, err := graph.NewGraph(st)				})

			if err != nil {				g.AddEdge(graph.StartNode, nodeName)

				b.Fatal(err)				g.AddEdge(nodeName, graph.EndNode)

			}			}



			// Create parallel nodes			compiled, err := exec.CompileGraph(g)

			for i := 0; i < width; i++ {			if err != nil {

				nodeName := fmt.Sprintf("parallel-%d", i)				b.Fatal(err)

				idx := i			}

				g.AddNode(&graph.Node{

					Name: nodeName,			initialMessages := []message.Message{

					RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {				message.NewHumanMessageFromText("test"),

						return &graph.NodeResult{			}

							Updates: map[string]any{nodeName: idx},

						}, nil			b.ResetTimer()

					},			b.ReportAllocs()

				})

				g.AddEdge(graph.StartNode, nodeName)			for b.Loop() {

				g.AddEdge(nodeName, graph.EndNode)				for range compiled.Run(ctx, initialMessages) {

			}				}

			}

			compiled, err := exec.CompileGraph(g)		})

			if err != nil {	}

				b.Fatal(err)}

			}

// BenchmarkConditionalBranching benchmarks conditional routing

			initialMessages := []message.Message{func BenchmarkConditionalBranching(b *testing.B) {

				message.NewHumanMessageFromText("test"),	ctx := context.Background()

			}

	stateManager, err := newTestState()

			b.ReportAllocs()	if err != nil {

			b.ResetTimer()		b.Fatal(err)

	}

			for i := 0; i < b.N; i++ {

				for range compiled.Run(ctx, initialMessages) {	g, err := graph.NewGraph(stateManager)

				}	if err != nil {

			}		b.Fatal(err)

		})	}

	}

}	g.AddNode(&graph.Node{

		Name: "router",

// BenchmarkConditionalBranching benchmarks conditional routing		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

func BenchmarkConditionalBranching(b *testing.B) {			return &graph.NodeResult{

	ctx := context.Background()				Updates: map[string]any{"route": "branch_a"},

			}, nil

	routeKey := state.NewKey[string]("route")		},

	resultKey := state.NewKey[string]("result")	})



	st := state.NewState()	g.AddNode(&graph.Node{

	state.Register(st, routeKey)		Name: "branch_a",

	state.Register(st, resultKey)		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

	state.Register(st, state.MessagesKey.Key)			return &graph.NodeResult{

				Updates: map[string]any{"result": "a"},

	g, err := graph.NewGraph(st)			}, nil

	if err != nil {		},

		b.Fatal(err)	})

	}

	g.AddNode(&graph.Node{

	g.AddNode(&graph.Node{		Name: "branch_b",

		Name: "router",		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {			return &graph.NodeResult{

			return &graph.NodeResult{				Updates: map[string]any{"result": "b"},

				Updates: map[string]any{"route": "branch_a"},			}, nil

			}, nil		},

		},	})

	})

	g.AddEdge(graph.StartNode, "router")

	g.AddNode(&graph.Node{	g.AddConditionalEdges("router", func(ctx context.Context, s *state.ReadView) []string {

		Name: "branch_a",		route := s.Get("route").(string)

		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {		if route == "branch_a" {

			return &graph.NodeResult{			return []string{"branch_a"}

				Updates: map[string]any{"result": "a"},		}

			}, nil		return []string{"branch_b"}

		},	}, []string{"branch_a", "branch_b"})

	})	g.AddEdge("branch_a", graph.EndNode)

	g.AddEdge("branch_b", graph.EndNode)

	g.AddNode(&graph.Node{

		Name: "branch_b",	compiled, err := exec.CompileGraph(g)

		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {	if err != nil {

			return &graph.NodeResult{		b.Fatal(err)

				Updates: map[string]any{"result": "b"},	}

			}, nil

		},	initialMessages := []message.Message{

	})		message.NewHumanMessageFromText("test"),

	}

	g.AddEdge(graph.StartNode, "router")

	g.AddConditionalEdges("router", func(ctx context.Context, view *state.ReadView) []string {	b.ReportAllocs()

		route := state.GetFromView(view, routeKey)

		if route == "branch_a" {	for b.Loop() {

			return []string{"branch_a"}		for range compiled.Run(ctx, initialMessages) {

		}		}

		return []string{"branch_b"}	}

	}, []string{"branch_a", "branch_b"})}

	g.AddEdge("branch_a", graph.EndNode)

	g.AddEdge("branch_b", graph.EndNode)// BenchmarkMessageThroughput benchmarks message handling

func BenchmarkMessageThroughput(b *testing.B) {

	compiled, err := exec.CompileGraph(g)	ctx := context.Background()

	if err != nil {

		b.Fatal(err)	messageCounts := []int{1, 10, 50, 100}

	}

	for _, count := range messageCounts {

	initialMessages := []message.Message{		b.Run(fmt.Sprintf("messages-%d", count), func(b *testing.B) {

		message.NewHumanMessageFromText("test"),			stateManager, err := newTestState()

	}			if err != nil {

				b.Fatal(err)

	b.ReportAllocs()			}

	b.ResetTimer()

			g, err := graph.NewGraph(stateManager)

	for i := 0; i < b.N; i++ {			if err != nil {

		for range compiled.Run(ctx, initialMessages) {				b.Fatal(err)

		}			}

	}

}			g.AddNode(&graph.Node{

				Name: "processor",

// BenchmarkMessageThroughput benchmarks message handling				RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

func BenchmarkMessageThroughput(b *testing.B) {					msgs := s.MessagesSnapshot()

	ctx := context.Background()					return &graph.NodeResult{

						Updates: map[string]any{"count": len(msgs)},

	messageCounts := []int{1, 10, 50, 100}					}, nil

				},

	for _, count := range messageCounts {			})

		b.Run(fmt.Sprintf("messages-%d", count), func(b *testing.B) {

			countKey := state.NewKey[int]("count")			g.AddEdge(graph.StartNode, "processor")

						g.AddEdge("processor", graph.EndNode)

			st := state.NewState()

			state.Register(st, countKey)			compiled, err := exec.CompileGraph(g)

			state.Register(st, state.MessagesKey.Key)			if err != nil {

				b.Fatal(err)

			g, err := graph.NewGraph(st)			}

			if err != nil {

				b.Fatal(err)			// Create many messages

			}			initialMessages := make([]message.Message, count)

			for i := range count {

			g.AddNode(&graph.Node{				initialMessages[i] = message.NewHumanMessageFromText(fmt.Sprintf("message-%d", i))

				Name: "processor",			}

				RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {

					msgs := state.GetMessages(view)			b.ResetTimer()

					return &graph.NodeResult{			b.ReportAllocs()

						Updates: map[string]any{"count": len(msgs)},

					}, nil			for b.Loop() {

				},				for range compiled.Run(ctx, initialMessages) {

			})				}

			}

			g.AddEdge(graph.StartNode, "processor")		})

			g.AddEdge("processor", graph.EndNode)	}

}

			compiled, err := exec.CompileGraph(g)

			if err != nil {// BenchmarkStateUpdates benchmarks state read/write performance

				b.Fatal(err)func BenchmarkStateUpdates(b *testing.B) {

			}	ctx := context.Background()



			// Create many messages	keyCounts := []int{1, 5, 10, 20}

			initialMessages := make([]message.Message, count)

			for i := 0; i < count; i++ {	for _, keyCount := range keyCounts {

				initialMessages[i] = message.NewHumanMessageFromText(fmt.Sprintf("message-%d", i))		b.Run(fmt.Sprintf("keys-%d", keyCount), func(b *testing.B) {

			}			stateManager, err := newTestState()

			if err != nil {

			b.ReportAllocs()				b.Fatal(err)

			b.ResetTimer()			}



			for i := 0; i < b.N; i++ {			g, err := graph.NewGraph(stateManager)

				for range compiled.Run(ctx, initialMessages) {			if err != nil {

				}				b.Fatal(err)

			}			}

		})

	}			g.AddNode(&graph.Node{

}				Name: "writer",

				RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

// BenchmarkStateUpdates benchmarks state read/write performance					updates := make(map[string]any, keyCount)

func BenchmarkStateUpdates(b *testing.B) {					for i := range keyCount {

	ctx := context.Background()						updates[fmt.Sprintf("key-%d", i)] = i

					}

	keyCounts := []int{1, 5, 10, 20}					return &graph.NodeResult{

						Updates: updates,

	for _, keyCount := range keyCounts {					}, nil

		b.Run(fmt.Sprintf("keys-%d", keyCount), func(b *testing.B) {				},

			st := state.NewState()			})

			state.Register(st, state.MessagesKey.Key)

						g.AddNode(&graph.Node{

			// Register all keys and create typed key slice				Name: "reader",

			keys := make([]state.Key[int], keyCount)				RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {

			for i := 0; i < keyCount; i++ {					sum := 0

				keys[i] = state.NewKey[int](fmt.Sprintf("key-%d", i))					for i := 0; i < keyCount; i++ {

				state.Register(st, keys[i])						val := s.Get(fmt.Sprintf("key-%d", i)).(int)

			}						sum += val

			sumKey := state.NewKey[int]("sum")					}

			state.Register(st, sumKey)					return &graph.NodeResult{

						Updates: map[string]any{"sum": sum},

			g, err := graph.NewGraph(st)					}, nil

			if err != nil {				},

				b.Fatal(err)			})

			}

			g.AddEdge(graph.StartNode, "writer")

			g.AddNode(&graph.Node{			g.AddEdge("writer", "reader")

				Name: "writer",			g.AddEdge("reader", graph.EndNode)

				RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {

					updates := make(map[string]any, keyCount)			compiled, err := exec.CompileGraph(g)

					for i := 0; i < keyCount; i++ {			if err != nil {

						updates[fmt.Sprintf("key-%d", i)] = i				b.Fatal(err)

					}			}

					return &graph.NodeResult{

						Updates: updates,			initialMessages := []message.Message{

					}, nil				message.NewHumanMessageFromText("test"),

				},			}

			})

			b.ResetTimer()

			g.AddNode(&graph.Node{			b.ReportAllocs()

				Name: "reader",

				RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {			for b.Loop() {

					sum := 0				for range compiled.Run(ctx, initialMessages) {

					for i := 0; i < keyCount; i++ {				}

						val := state.GetFromView(view, keys[i])			}

						sum += val		})

					}	}

					return &graph.NodeResult{}

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

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				for range compiled.Run(ctx, initialMessages) {
				}
			}
		})
	}
}
