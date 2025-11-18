package integration_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
)

// TestMessagePropagationAcrossSupersteps verifies that updates sent in superstep N
// are received and applied in superstep N+1, testing the BSP message delivery model.
func TestMessagePropagationAcrossSupersteps(t *testing.T) {
	t.Parallel()

	fromAKey := state.NewKey("from_a", "")
	counterKey := state.NewKey("counter", 0)
	fromBKey := state.NewKey("from_b", "")
	statusKey := state.NewKey("status", "")

	stateManager := newTestManager()
	state.RegisterKey(stateManager, fromAKey)
	state.RegisterKey(stateManager, counterKey)
	state.RegisterKey(stateManager, fromBKey)
	state.RegisterKey(stateManager, statusKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node A sends updates to Node B
	g.AddNode(&graph.Node{
		Name: "node_a",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			// Send data to node_b
			return &graph.NodeResult{
				Updates: map[string]any{
					"from_a":  "hello from A",
					"counter": 1,
				},
			}, nil
		},
	})

	// Node B receives updates from Node A
	g.AddNode(&graph.Node{
		Name: "node_b",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			// Verify we received the update from node_a
			fromA := state.GetFromView(s, fromAKey)
			counter := state.GetFromView(s, counterKey)

			// These should be available after node_a completes
			require.NotEmpty(t, fromA, "Should receive update from node_a")
			require.Equal(t, "hello from A", fromA)
			require.Equal(t, 1, counter)

			return &graph.NodeResult{
				Updates: map[string]any{
					"from_b": "hello from B",
					"status": "received",
				},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "node_a")
	g.AddEdge("node_a", "node_b")
	g.AddEdge("node_b", graph.EndNode)

	compiled, err := exec.CompileGraph(g, exec.NewPregelExecutor())
	require.NoError(t, err)

	// Execute the graph
	ctx := context.Background()
	events, err := graph.Collect(compiled.Run(ctx, nil))
	require.NoError(t, err)
	require.NotNil(t, events)

	// Verify final state
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView error: %v", err)
	}

	require.Equal(t, "hello from A", state.GetFromView(view, fromAKey))
	require.Equal(t, "hello from B", state.GetFromView(view, fromBKey))
	require.Equal(t, "received", state.GetFromView(view, statusKey))
}

// TestParallelMessagePropagation tests that parallel nodes can send messages
// to the same downstream node and all messages are received.
func TestParallelMessagePropagation(t *testing.T) {
	t.Parallel()

	fromParallelAKey := state.NewKey("from_parallel_a", "")
	fromParallelBKey := state.NewKey("from_parallel_b", "")
	aggregatedKey := state.NewKey("aggregated", false)

	stateManager := newTestManager()
	state.RegisterKey(stateManager, fromParallelAKey)
	state.RegisterKey(stateManager, fromParallelBKey)
	state.RegisterKey(stateManager, aggregatedKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Two parallel nodes sending to the same target
	g.AddNode(&graph.Node{
		Name: "parallel_a",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"from_parallel_a": "data_a"},
			}, nil
		},
	})

	g.AddNode(&graph.Node{
		Name: "parallel_b",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{"from_parallel_b": "data_b"},
			}, nil
		},
	})

	// Aggregator node receives from both parallel nodes
	g.AddNode(&graph.Node{
		Name: "aggregator",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			dataA := state.GetFromView(s, fromParallelAKey)
			dataB := state.GetFromView(s, fromParallelBKey)

			// Both updates should be present
			require.NotEmpty(t, dataA, "Should receive update from parallel_a")
			require.NotEmpty(t, dataB, "Should receive update from parallel_b")
			require.Equal(t, "data_a", dataA)
			require.Equal(t, "data_b", dataB)

			return &graph.NodeResult{
				Updates: map[string]any{"aggregated": true},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "parallel_a")
	g.AddEdge(graph.StartNode, "parallel_b")
	g.AddEdge("parallel_a", "aggregator")
	g.AddEdge("parallel_b", "aggregator")
	g.AddEdge("aggregator", graph.EndNode)

	compiled, err := exec.CompileGraph(g, exec.NewPregelExecutor())
	require.NoError(t, err)

	ctx := context.Background()
	// Consume all results - we only care that execution completes without error
	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Verify all updates were applied
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView error: %v", err)
	}

	require.Equal(t, "data_a", state.GetFromView(view, fromParallelAKey))
	require.Equal(t, "data_b", state.GetFromView(view, fromParallelBKey))
	require.Equal(t, true, state.GetFromView(view, aggregatedKey))
}

// TestMessagePropagationSequential verifies that updates from one node
// are visible to subsequent nodes in a linear chain.
func TestMessagePropagationSequential(t *testing.T) {
	t.Parallel()

	stepKey := state.NewKey("step", 0)
	dataKey := state.NewKey("data", "")
	finalKey := state.NewKey("final", false)

	stateManager := newTestManager()
	state.RegisterKey(stateManager, stepKey)
	state.RegisterKey(stateManager, dataKey)
	state.RegisterKey(stateManager, finalKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node 1: Sets initial values
	g.AddNode(&graph.Node{
		Name: "node_1",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			return &graph.NodeResult{
				Updates: map[string]any{
					"step": 1,
					"data": "from_node_1",
				},
			}, nil
		},
	})

	// Node 2: Reads from node 1, adds its own data
	g.AddNode(&graph.Node{
		Name: "node_2",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			step := state.GetFromView(s, stepKey)
			data := state.GetFromView(s, dataKey)

			require.Equal(t, 1, step, "Should receive step from node_1")
			require.Equal(t, "from_node_1", data)

			return &graph.NodeResult{
				Updates: map[string]any{
					"step": 2,
					"data": "from_node_2",
				},
			}, nil
		},
	})

	// Node 3: Reads from node 2, verifies propagation
	g.AddNode(&graph.Node{
		Name: "node_3",
		RunFunc: func(ctx context.Context, s *state.ReadView) (*graph.NodeResult, error) {
			step := state.GetFromView(s, stepKey)
			data := state.GetFromView(s, dataKey)

			require.Equal(t, 2, step, "Should receive step from node_2")
			require.Equal(t, "from_node_2", data)

			return &graph.NodeResult{
				Updates: map[string]any{
					"step":  3,
					"final": true,
				},
			}, nil
		},
	})

	g.AddEdge(graph.StartNode, "node_1")
	g.AddEdge("node_1", "node_2")
	g.AddEdge("node_2", "node_3")
	g.AddEdge("node_3", graph.EndNode)

	compiled, err := exec.CompileGraph(g, exec.NewPregelExecutor())
	require.NoError(t, err)

	ctx := context.Background()
	// Consume all results - we only care that execution completes without error
	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Verify final state has all updates
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView error: %v", err)
	}

	require.Equal(t, 3, state.GetFromView(view, stepKey))
	require.Equal(t, true, state.GetFromView(view, finalKey))
}
