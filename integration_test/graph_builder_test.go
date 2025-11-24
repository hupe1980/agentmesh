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

		node := &graph.BaseCommandNode{
			NodeName:        "test_node",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				updates := map[string]any{"executed": true}
				return graph.End(updates), nil
			},
		}

		err := g.AddNode(node)
		require.NoError(t, err)
		assert.Contains(t, g.Nodes, "test_node")
	})

	t.Run("adds edges successfully", func(t *testing.T) {
		stateManager := newTestManager()
		g, _ := graph.NewGraph(stateManager)

		node := &graph.BaseCommandNode{
			NodeName:        "test",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				return graph.End(nil), nil
			},
		}
		g.AddNode(node)

		g.SetEntryPoint("test")

		// EntryPoint should be set correctly; routing defined by DeclaredTargets
		assert.Equal(t, "test", g.EntryPoint)
	})

	t.Run("adds command nodes with multiple targets", func(t *testing.T) {
		stateManager := newTestManager()
		g, _ := graph.NewGraph(stateManager)

		g.AddNode(&graph.BaseCommandNode{
			NodeName:        "source",
			DeclaredTargets: graph.NewTargetSet("target1", "target2"),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				return graph.GotoOne("target1"), nil
			},
		})
		g.AddNode(&graph.BaseCommandNode{
			NodeName:        "target1",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				return graph.End(nil), nil
			},
		})
		g.AddNode(&graph.BaseCommandNode{
			NodeName:        "target2",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				return graph.End(nil), nil
			},
		})

		// Command pattern: routing via DeclaredTargets
		// Verify source node has multiple declared targets
		sourceNode := g.Nodes["source"]
		assert.Len(t, sourceNode.Targets(), 2)
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
		node := &graph.BaseCommandNode{
			NodeName:        "test",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				executed = true
				updates := map[string]any{"result": "success"}
				return graph.End(updates), nil
			},
		}

		g.AddNode(node)
		g.SetEntryPoint("test")

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

		node1 := &graph.BaseCommandNode{
			NodeName:        "node1",
			DeclaredTargets: graph.NewTargetSet("node2"),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				executionOrder = append(executionOrder, "node1")
				updates := map[string]any{"count": 1}
				return graph.Goto("node2", updates), nil
			},
		}

		node2 := &graph.BaseCommandNode{
			NodeName:        "node2",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				executionOrder = append(executionOrder, "node2")
				count := state.GetFromView(s, countKey)
				updates := map[string]any{"count": count + 1}
				return graph.End(updates), nil
			},
		}

		g.AddNode(node1)
		g.AddNode(node2)
		g.SetEntryPoint("node1")

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
		decider := &graph.BaseCommandNode{
			NodeName:        "decider",
			DeclaredTargets: graph.NewTargetSet("left", "right"),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				updates := map[string]any{"choice": "left"}
				return graph.Goto("left", updates), nil
			},
		}

		leftExecuted := false
		leftNode := &graph.BaseCommandNode{
			NodeName:        "left",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				leftExecuted = true
				updates := map[string]any{"result": "went_left"}
				return graph.End(updates), nil
			},
		}

		rightExecuted := false
		rightNode := &graph.BaseCommandNode{
			NodeName:        "right",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				rightExecuted = true
				updates := map[string]any{"result": "went_right"}
				return graph.End(updates), nil
			},
		}

		g.AddNode(decider)
		g.AddNode(leftNode)
		g.AddNode(rightNode)

		g.SetEntryPoint("decider")

		// Conditional routing based on "choice" value		// Use Sequential executor for deterministic routing
		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

		source := &graph.BaseCommandNode{
			NodeName:        "source",
			DeclaredTargets: graph.NewTargetSet("target1", "target2"),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				updates := map[string]any{"broadcast": true}
				return graph.GotoAll([]string{"target1", "target2"}, updates), nil
			},
		}

		target1Executed := false
		target1 := &graph.BaseCommandNode{
			NodeName:        "target1",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				target1Executed = true
				return graph.End(nil), nil
			},
		}

		target2Executed := false
		target2 := &graph.BaseCommandNode{
			NodeName:        "target2",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				target2Executed = true
				return graph.End(nil), nil
			},
		}

		g.AddNode(source)
		g.AddNode(target1)
		g.AddNode(target2)

		g.SetEntryPoint("source")

		// Route to both targets		// Use Sequential to avoid repeated executions
		compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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
		writer := &graph.BaseCommandNode{
			NodeName:        "writer",
			DeclaredTargets: graph.NewTargetSet("reader"),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				builder := graph.NewUpdate()
				graph.UpdateSet(builder, key1Key, "value1")
				graph.UpdateSet(builder, key2Key, 42)
				updates, _ := builder.Build()
				return graph.Goto("reader", updates), nil
			},
		}

		// Second node reads state
		var readValue1 string
		var readValue2 int
		reader := &graph.BaseCommandNode{
			NodeName:        "reader",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				readValue1 = state.GetFromView(s, key1Key)
				readValue2 = state.GetFromView(s, key2Key)
				return graph.End(nil), nil
			},
		}

		g.AddNode(writer)
		g.AddNode(reader)
		g.SetEntryPoint("writer")

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

		incrementer := &graph.BaseCommandNode{
			NodeName:        "increment",
			DeclaredTargets: graph.NewTargetSet(graph.EndNode),
			Fn: func(ctx context.Context, s state.ReadView) (*graph.Command, error) {
				counter := state.GetFromView(s, counterKey)
				updates := map[string]any{"counter": counter + 1}
				return graph.End(updates), nil
			},
		}

		g.AddNode(incrementer)
		g.SetEntryPoint("increment")

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
