package integration_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBasicGraphBuilding tests basic graph construction and validation
func TestBasicGraphBuilding(t *testing.T) {
	t.Run("creates graph with valid state manager", func(t *testing.T) {
		stateManager := newTestManager()

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)
		require.NotNil(t, g)
	})

	t.Run("adds node successfully", func(t *testing.T) {
		stateManager := newTestManager()
		g, _ := graph.NewGraph(stateManager)

		node := graph.NewBaseNode("test_node", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"executed": true}, nil
		})

		err := g.AddNode(node)
		require.NoError(t, err)
		assert.Contains(t, g.Nodes, "test_node")
	})

	t.Run("adds edges successfully", func(t *testing.T) {
		stateManager := newTestManager()
		g, _ := graph.NewGraph(stateManager)

		node := graph.NewBaseNode("test", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return nil, nil
		})
		g.AddNode(node)

		g.AddEdge(graph.StartNode, "test")
		g.AddEdge("test", graph.EndNode)

		assert.Len(t, g.Edges, 2)
	})

	t.Run("adds conditional edges successfully", func(t *testing.T) {
		stateManager := newTestManager()
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(graph.NewBaseNode("source", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return nil, nil
		}))
		g.AddNode(graph.NewBaseNode("target1", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return nil, nil
		}))
		g.AddNode(graph.NewBaseNode("target2", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return nil, nil
		}))

		condition := func(ctx context.Context, s *state.ReadView) []string {
			return []string{"target1"}
		}

		g.AddConditionalEdges("source", condition, []string{"target1", "target2"})

		assert.Len(t, g.Branches, 1)
	})
}

// TestSimpleGraphExecution tests basic graph execution flow
func TestSimpleGraphExecution(t *testing.T) {
	t.Run("executes single node", func(t *testing.T) {
		resultKey := state.NewKey("result", "")

		stateManager := newTestManager()
		state.RegisterKey(stateManager, resultKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		executed := false
		node := graph.NewBaseNode("test", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			executed = true
			return map[string]any{"result": "success"}, nil
		})

		g.AddNode(node)
		g.AddEdge(graph.StartNode, "test")
		g.AddEdge("test", graph.EndNode)

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		assert.True(t, executed, "node should have been executed")
	})

	t.Run("executes sequential nodes", func(t *testing.T) {
		countKey := state.NewKey("count", 0)

		stateManager := newTestManager()
		state.RegisterKey(stateManager, countKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		var executionOrder []string

		node1 := graph.NewBaseNode("node1", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			executionOrder = append(executionOrder, "node1")
			return map[string]any{"count": 1}, nil
		})

		node2 := graph.NewBaseNode("node2", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			executionOrder = append(executionOrder, "node2")
			count := state.GetFromView(s, countKey)
			return map[string]any{"count": count + 1}, nil
		})

		g.AddNode(node1)
		g.AddNode(node2)
		g.AddEdge(graph.StartNode, "node1")
		g.AddEdge("node1", "node2")
		g.AddEdge("node2", graph.EndNode)

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		assert.Equal(t, []string{"node1", "node2"}, executionOrder)
	})
}

// TestConditionalRouting tests conditional edge routing
func TestConditionalRouting(t *testing.T) {
	t.Run("routes based on state with Sequential executor", func(t *testing.T) {
		choiceKey := state.NewKey("choice", "")
		resultKey := state.NewKey("result", "")

		stateManager := newTestManager()
		state.RegisterKey(stateManager, choiceKey)
		state.RegisterKey(stateManager, resultKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		// Decision node that sets routing choice
		decider := graph.NewBaseNode("decider", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"choice": "left"}, nil
		})

		leftExecuted := false
		leftNode := graph.NewBaseNode("left", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			leftExecuted = true
			return map[string]any{"result": "went_left"}, nil
		})

		rightExecuted := false
		rightNode := graph.NewBaseNode("right", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			rightExecuted = true
			return map[string]any{"result": "went_right"}, nil
		})

		g.AddNode(decider)
		g.AddNode(leftNode)
		g.AddNode(rightNode)

		g.AddEdge(graph.StartNode, "decider")

		// Conditional routing based on "choice" value
		g.AddConditionalEdges("decider", func(ctx context.Context, s *state.ReadView) []string {
			choice := state.GetFromView(s, choiceKey)
			if choice == "left" {
				return []string{"left"}
			}
			return []string{"right"}
		}, []string{"left", "right"})

		g.AddEdge("left", graph.EndNode)
		g.AddEdge("right", graph.EndNode)

		// Use Sequential executor for deterministic routing
		compiled, err := graph.Compile(g, graph.NewSequentialExecutor())
		require.NoError(t, err)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		assert.True(t, leftExecuted, "left node should have been executed")
		assert.False(t, rightExecuted, "right node should NOT have been executed")
	})

	t.Run("routes to multiple targets in parallel", func(t *testing.T) {
		broadcastKey := state.NewKey("broadcast", false)

		stateManager := newTestManager()
		state.RegisterKey(stateManager, broadcastKey)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		source := graph.NewBaseNode("source", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{"broadcast": true}, nil
		})

		target1Executed := false
		target1 := graph.NewBaseNode("target1", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			target1Executed = true
			return nil, nil
		})

		target2Executed := false
		target2 := graph.NewBaseNode("target2", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			target2Executed = true
			return nil, nil
		})

		g.AddNode(source)
		g.AddNode(target1)
		g.AddNode(target2)

		g.AddEdge(graph.StartNode, "source")

		// Route to both targets
		g.AddConditionalEdges("source", func(ctx context.Context, s *state.ReadView) []string {
			return []string{"target1", "target2"}
		}, []string{"target1", "target2"})

		g.AddEdge("target1", graph.EndNode)
		g.AddEdge("target2", graph.EndNode)

		// Use Sequential to avoid repeated executions
		compiled, err := graph.Compile(g, graph.NewSequentialExecutor())
		require.NoError(t, err)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		// Both targets should be executed
		assert.True(t, target1Executed, "target1 should have been executed")
		assert.True(t, target2Executed, "target2 should have been executed")
	})
}

// TestStateManagement tests state updates and reading
func TestStateManagement(t *testing.T) {
	t.Run("node can read and write state", func(t *testing.T) {
		key1Key := state.NewKey("key1", "")
		key2Key := state.NewKey("key2", 0)

		stateManager := newTestManager()
		state.RegisterKey(stateManager, key1Key)
		state.RegisterKey(stateManager, key2Key)

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		// First node writes state
		writer := graph.NewBaseNode("writer", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			return map[string]any{
				"key1": "value1",
				"key2": 42,
			}, nil
		})

		// Second node reads state
		var readValue1 string
		var readValue2 int
		reader := graph.NewBaseNode("reader", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			readValue1 = state.GetFromView(s, key1Key)
			readValue2 = state.GetFromView(s, key2Key)
			return nil, nil
		})

		g.AddNode(writer)
		g.AddNode(reader)
		g.AddEdge(graph.StartNode, "writer")
		g.AddEdge("writer", "reader")
		g.AddEdge("reader", graph.EndNode)

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err)

		ctx := context.Background()
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		assert.Equal(t, "value1", readValue1)
		assert.Equal(t, 42, readValue2)
	})

	t.Run("state persists across execution", func(t *testing.T) {
		counterKey := state.NewKey("counter", 0)

		stateManager := newTestManager()
		state.RegisterKey(stateManager, counterKey)

		// Set initial state using state.Set
		ctx := context.Background()
		if err := state.Set(ctx, stateManager, counterKey, 0); err != nil {
			t.Fatalf("Failed to initialize state: %v", err)
		}

		g, err := graph.NewGraph(stateManager)
		require.NoError(t, err)

		incrementer := graph.NewBaseNode("increment", func(ctx context.Context, s *state.ReadView) (state.Updates, error) {
			counter := state.GetFromView(s, counterKey)
			return map[string]any{"counter": counter + 1}, nil
		})

		g.AddNode(incrementer)
		g.AddEdge(graph.StartNode, "increment")
		g.AddEdge("increment", graph.EndNode)

		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
		require.NoError(t, err)

		// First execution
		for _, err := range compiled.Run(ctx, nil) {
			require.NoError(t, err)
		}

		// Check state was updated
		view, err := stateManager.CreateReadView(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, state.GetFromView(view, counterKey))
	})
}
