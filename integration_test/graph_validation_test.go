package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/compile"
	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraphValidation tests graph structure validation
func TestGraphValidation(t *testing.T) {
	t.Run("compiles graph with unreachable nodes", func(t *testing.T) {
		// Note: The current implementation doesn't validate unreachable nodes
		// This is acceptable - unreachable nodes simply won't execute
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		// Add reachable node
		reachable := &graph.Node{
			Name: "reachable",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		// Add unreachable node (no edges to it)
		unreachable := &graph.Node{
			Name: "unreachable",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		g.AddNode(reachable)
		g.AddNode(unreachable)
		g.AddEdge(graph.StartNode, "reachable")
		g.AddEdge("reachable", graph.EndNode)
		// "unreachable" has no incoming edges - it won't execute but graph compiles

		// Compilation succeeds (unreachable nodes are ignored during execution)
		_, err = exec.CompileGraph(g)
		require.NoError(t, err)
	})

	t.Run("accepts valid linear graph", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		step1 := &graph.Node{
			Name: "step1",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		step2 := &graph.Node{
			Name: "step2",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		g.AddNode(step1)
		g.AddNode(step2)
		g.AddEdge(graph.StartNode, "step1")
		g.AddEdge("step1", "step2")
		g.AddEdge("step2", graph.EndNode)

		_, err = exec.CompileGraph(g)
		require.NoError(t, err, "valid graph should compile without errors")
	})

	t.Run("accepts graph with conditional branches", func(t *testing.T) {
		routeKey := state.NewKey("route", "")

		stateManager := newTestManager()
		state.RegisterKey(stateManager, routeKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		router := &graph.Node{
			Name: "router",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		pathA := &graph.Node{
			Name: "pathA",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		pathB := &graph.Node{
			Name: "pathB",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		g.AddNode(router)
		g.AddNode(pathA)
		g.AddNode(pathB)

		g.AddEdge(graph.StartNode, "router")
		g.AddConditionalEdges("router", func(ctx context.Context, view *state.ReadView) []string {
			return []string{"pathA"}
		}, []string{"pathA", "pathB"})
		g.AddEdge("pathA", graph.EndNode)
		g.AddEdge("pathB", graph.EndNode)

		// Both branches are reachable via conditional
		_, err = exec.CompileGraph(g)
		require.NoError(t, err)
	})
}

// TestErrorHandling tests error propagation in graph execution
func TestErrorHandling(t *testing.T) {
	t.Run("propagates node errors", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		expectedErr := errors.New("node failed")
		failingNode := &graph.Node{
			Name: "failing",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return nil, expectedErr
			},
		}

		g.AddNode(failingNode)
		g.AddEdge(graph.StartNode, "failing")
		g.AddEdge("failing", graph.EndNode)

		compiled, err := exec.CompileGraph(g)
		require.NoError(t, err)

		ctx := context.Background()
		var gotErr error
		for _, err := range compiled.Run(ctx, nil) {
			if err != nil {
				gotErr = err
			}
		}

		require.Error(t, gotErr)
		assert.Contains(t, gotErr.Error(), "node failed")
	})

	t.Run("stops execution on error", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		node1Executed := false
		node1 := &graph.Node{
			Name: "node1",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				node1Executed = true
				return nil, errors.New("error in node1")
			},
		}

		node2Executed := false
		node2 := &graph.Node{
			Name: "node2",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				node2Executed = true
				return &graph.NodeResult{}, nil
			},
		}

		g.AddNode(node1)
		g.AddNode(node2)
		g.AddEdge(graph.StartNode, "node1")
		g.AddEdge("node1", "node2")
		g.AddEdge("node2", graph.EndNode)

		compiled, err := exec.CompileGraph(g, exec.WithExecutor(exec.NewSequential()))
		require.NoError(t, err)

		ctx := context.Background()
		for range compiled.Run(ctx, nil) {
		}

		assert.True(t, node1Executed, "node1 should execute")
		assert.False(t, node2Executed, "node2 should not execute after node1 error")
	})
}

// TestComplexGraphPatterns tests more complex graph structures
func TestComplexGraphPatterns(t *testing.T) {
	t.Run("diamond pattern execution", func(t *testing.T) {
		splitDoneKey := state.NewKey("split_done", false)
		leftDoneKey := state.NewKey("left_done", false)
		rightDoneKey := state.NewKey("right_done", false)

		stateManager := newTestManager()
		state.RegisterKey(stateManager, splitDoneKey)
		state.RegisterKey(stateManager, leftDoneKey)
		state.RegisterKey(stateManager, rightDoneKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		var executionOrder []string
		var executionOrderMu sync.Mutex

		// Diamond pattern: start -> split -> (left, right) -> merge -> end
		split := &graph.Node{
			Name: "split",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "split")
				executionOrderMu.Unlock()
				return &graph.NodeResult{
					Updates: map[string]any{"split_done": true},
				}, nil
			},
		}

		left := &graph.Node{
			Name: "left",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "left")
				executionOrderMu.Unlock()
				return &graph.NodeResult{
					Updates: map[string]any{"left_done": true},
				}, nil
			},
		}

		right := &graph.Node{
			Name: "right",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "right")
				executionOrderMu.Unlock()
				return &graph.NodeResult{
					Updates: map[string]any{"right_done": true},
				}, nil
			},
		}

		merge := &graph.Node{
			Name: "merge",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "merge")
				executionOrderMu.Unlock()
				// Both left and right should have executed
				leftDone := state.GetFromView(view, leftDoneKey)
				rightDone := state.GetFromView(view, rightDoneKey)
				if !leftDone || !rightDone {
					return nil, errors.New("left and right should have executed before merge")
				}
				return &graph.NodeResult{}, nil
			},
		}

		g.AddNode(split)
		g.AddNode(left)
		g.AddNode(right)
		g.AddNode(merge)

		g.AddEdge(graph.StartNode, "split")
		g.AddConditionalEdges("split", func(ctx context.Context, view *state.ReadView) []string {
			return []string{"left", "right"}
		}, []string{"left", "right"})
		g.AddEdge("left", "merge")
		g.AddEdge("right", "merge")
		g.AddEdge("merge", graph.EndNode)

		compiled, err := exec.CompileGraph(g)
		require.NoError(t, err)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		// Verify all nodes executed
		assert.Contains(t, executionOrder, "split")
		assert.Contains(t, executionOrder, "left")
		assert.Contains(t, executionOrder, "right")
		assert.Contains(t, executionOrder, "merge")

		// Verify merge executed after split
		splitIdx := -1
		mergeIdx := -1
		for i, name := range executionOrder {
			if name == "split" {
				splitIdx = i
			}
			if name == "merge" {
				mergeIdx = i
			}
		}
		assert.Less(t, splitIdx, mergeIdx, "merge should execute after split")
	})

	t.Run("cyclic pattern with loop", func(t *testing.T) {
		counterKey := state.NewKey("counter", 0)

		stateManager := newTestManager()
		state.RegisterKey(stateManager, counterKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		counter := 0
		maxIterations := 3

		loop := &graph.Node{
			Name: "loop",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				counter++
				return &graph.NodeResult{
					Updates: map[string]any{"counter": counter},
				}, nil
			},
		}

		g.AddNode(loop)
		g.AddEdge(graph.StartNode, "loop")

		// Conditional: loop back or exit
		g.AddConditionalEdges("loop", func(ctx context.Context, view *state.ReadView) []string {
			c := state.GetFromView(view, counterKey)
			if c >= maxIterations {
				return []string{graph.EndNode}
			}
			return []string{"loop"}
		}, []string{"loop", graph.EndNode})

		compiled, err := exec.CompileGraph(g, exec.WithExecutor(exec.NewSequential()))
		require.NoError(t, err)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		assert.Equal(t, maxIterations, counter, "should have looped exactly maxIterations times")
	})
}

// TestCompileOptions tests compilation options
func TestCompileOptions(t *testing.T) {
	t.Run("uses custom executor", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		node := &graph.Node{
			Name: "test",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		g.AddNode(node)
		g.AddEdge(graph.StartNode, "test")
		g.AddEdge("test", graph.EndNode)

		// Compile with Sequential executor
		compiled, err := exec.CompileGraph(g, exec.WithExecutor(exec.NewSequential()))
		require.NoError(t, err)
		require.NotNil(t, compiled)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}
	})

	t.Run("uses default Pregel executor when no option provided", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		node := &graph.Node{
			Name: "test",
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				return &graph.NodeResult{}, nil
			},
		}

		g.AddNode(node)
		g.AddEdge(graph.StartNode, "test")
		g.AddEdge("test", graph.EndNode)

		// Compile without executor option (should default to Pregel)
		compiled, err := exec.CompileGraph(g)
		require.NoError(t, err)
		require.NotNil(t, compiled)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}
	})
}

// TestTopologyComputation tests the compile package's topology building
func TestTopologyComputation(t *testing.T) {
	t.Run("computes correct topology for linear graph", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		g.AddNode(&graph.Node{Name: "a", RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		}})
		g.AddNode(&graph.Node{Name: "b", RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		}})

		g.AddEdge(graph.StartNode, "a")
		g.AddEdge("a", "b")
		g.AddEdge("b", graph.EndNode)

		compiled, err := compile.Compile(g, stateManager)
		require.NoError(t, err)

		// Verify topology
		assert.Equal(t, compile.StartNode, compiled.StartNode)
		assert.Equal(t, compile.EndNode, compiled.EndNode)

		// Check outgoing edges
		assert.Contains(t, compiled.Topology.Outgoing[compile.StartNode], "a")
		assert.Contains(t, compiled.Topology.Outgoing["a"], "b")
		assert.Contains(t, compiled.Topology.Outgoing["b"], compile.EndNode)
	})

	t.Run("computes topology with conditional edges", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		g.AddNode(&graph.Node{Name: "router", RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		}})
		g.AddNode(&graph.Node{Name: "target1", RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		}})
		g.AddNode(&graph.Node{Name: "target2", RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{}, nil
		}})

		g.AddEdge(graph.StartNode, "router")
		g.AddConditionalEdges("router", func(ctx context.Context, view *state.ReadView) []string {
			return []string{"target1"}
		}, []string{"target1", "target2"})
		g.AddEdge("target1", graph.EndNode)
		g.AddEdge("target2", graph.EndNode)

		compiled, err := compile.Compile(g, stateManager)
		require.NoError(t, err)

		// Verify conditional edges are recorded
		assert.Contains(t, compiled.Topology.ConditionalByFrom, "router")
		conditionals := compiled.Topology.ConditionalByFrom["router"]
		assert.NotEmpty(t, conditionals)
	})
}
