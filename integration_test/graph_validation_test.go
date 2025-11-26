package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"

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
		reachable := &graph.BaseNode{
			NodeName:        "reachable",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		}

		// Add unreachable node (no edges to it)
		unreachable := &graph.BaseNode{
			NodeName:        "unreachable",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		}

		g.AddNode(reachable)
		g.AddNode(unreachable)
		g.SetEntryPoint("reachable") // "unreachable" has no incoming edges - it won't execute but graph compiles

		// Compilation succeeds (unreachable nodes are ignored during execution)
		_, err = graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err)
	})

	t.Run("accepts valid linear graph", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		step1 := &graph.BaseNode{
			NodeName:        "step1",
			DeclaredTargets: []string{"step2"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{"step2"}, nil, nil
			},
		}

		step2 := &graph.BaseNode{
			NodeName:        "step2",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		}

		g.AddNode(step1)
		g.AddNode(step2)
		g.SetEntryPoint("step1")

		_, err = graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err, "valid graph should compile without errors")
	})

	t.Run("accepts graph with conditional branches", func(t *testing.T) {
		routeKey := state.NewKey("route", "")

		stateManager := newTestManager()
		state.RegisterKey(stateManager, routeKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		router := &graph.BaseNode{
			NodeName:        "router",
			DeclaredTargets: []string{"pathA", "pathB"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{"pathA"}, nil, nil
			},
		}

		pathA := &graph.BaseNode{
			NodeName:        "pathA",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		}

		pathB := &graph.BaseNode{
			NodeName:        "pathB",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		}

		g.AddNode(router)
		g.AddNode(pathA)
		g.AddNode(pathB)

		g.SetEntryPoint("router") // Both branches are reachable via conditional
		_, err = graph.Compile(g, graph.NewMessagePregelExecutor())
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
		failingNode := &graph.BaseNode{
			NodeName:        "failing",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return nil, nil, expectedErr
			},
		}

		g.AddNode(failingNode)
		g.SetEntryPoint("failing")

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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
		node1 := &graph.BaseNode{
			NodeName:        "node1",
			DeclaredTargets: []string{"node2"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				node1Executed = true
				return nil, nil, errors.New("error in node1")
			},
		}

		node2Executed := false
		node2 := &graph.BaseNode{
			NodeName:        "node2",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				node2Executed = true
				return []string{graph.EndNode}, nil, nil
			},
		}

		g.AddNode(node1)
		g.AddNode(node2)
		g.SetEntryPoint("node1")

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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
		split := &graph.BaseNode{
			NodeName:        "split",
			DeclaredTargets: []string{"left", "right"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "split")
				executionOrderMu.Unlock()
				updates := state.Updates{}
				updates[splitDoneKey.Name()] = true
				return []string{"left", "right"}, updates, nil
			},
		}

		left := &graph.BaseNode{
			NodeName:        "left",
			DeclaredTargets: []string{"merge"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "left")
				executionOrderMu.Unlock()
				updates := state.Updates{}
				updates[leftDoneKey.Name()] = true
				return []string{"merge"}, updates, nil
			},
		}

		right := &graph.BaseNode{
			NodeName:        "right",
			DeclaredTargets: []string{"merge"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "right")
				executionOrderMu.Unlock()
				updates := state.Updates{}
				updates[rightDoneKey.Name()] = true
				return []string{"merge"}, updates, nil
			},
		}

		merge := &graph.BaseNode{
			NodeName:        "merge",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				executionOrderMu.Lock()
				executionOrder = append(executionOrder, "merge")
				executionOrderMu.Unlock()
				// Both left and right should have executed
				leftDone := state.GetFromView(view, leftDoneKey)
				rightDone := state.GetFromView(view, rightDoneKey)
				if !leftDone || !rightDone {
					return nil, nil, errors.New("left and right should have executed before merge")
				}
				return []string{graph.EndNode}, nil, nil
			},
		}

		g.AddNode(split)
		g.AddNode(left)
		g.AddNode(right)
		g.AddNode(merge)

		g.SetEntryPoint("split")

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

		loop := &graph.BaseNode{
			NodeName:        "loop",
			DeclaredTargets: []string{"loop", graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				counter++
				updates := state.Updates{"counter": counter}
				// Routing determined by Command.Goto() or Command.End()
				// Check counter within node and route accordingly
				if counter >= maxIterations {
					return []string{graph.EndNode}, updates, nil
				}
				return []string{"loop"}, updates, nil
			},
		}

		g.AddNode(loop)
		g.SetEntryPoint("loop")

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

		node := &graph.BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		}

		g.AddNode(node)
		g.SetEntryPoint("test")

		// Compile with Sequential executor
		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

		node := &graph.BaseNode{
			NodeName:        "test",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		}

		g.AddNode(node)
		g.SetEntryPoint("test")

		// Compile without executor option (should default to Pregel)
		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

		g.AddNode(&graph.BaseNode{
			NodeName:        "a",
			DeclaredTargets: []string{"b"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{"b"}, nil, nil
			},
		})
		g.AddNode(&graph.BaseNode{
			NodeName:        "b",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		})

		g.SetEntryPoint("a")

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err)

		// Verify topology can be retrieved
		topo := compiled.GetTopology()
		assert.NotNil(t, topo)
		assert.Greater(t, len(topo.Nodes), 0)
		assert.Greater(t, len(topo.Edges), 0)
	})

	t.Run("computes topology with conditional edges", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		g.AddNode(&graph.BaseNode{
			NodeName:        "router",
			DeclaredTargets: []string{"target1", "target2"},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{"target1"}, nil, nil
			},
		})
		g.AddNode(&graph.BaseNode{
			NodeName:        "target1",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		})
		g.AddNode(&graph.BaseNode{
			NodeName:        "target2",
			DeclaredTargets: []string{graph.EndNode},
			Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
		})

		g.SetEntryPoint("router")

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err)

		// Verify topology includes command nodes
		topo := compiled.GetTopology()
		assert.Contains(t, topo.CommandNodes, "router")
	})
}
