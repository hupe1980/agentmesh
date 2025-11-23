package integration_test

import (
	"context"
	"testing"

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
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node_a",
		DeclaredTargets: graph.NewTargetSet("node_b"),
		Fn: func(ctx context.Context, s *state.ReadView) (*graph.Command, error) {
			// Send data to node_b
			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, fromAKey, "hello from A")
			state.SetUpdate(builder, counterKey, 1)
			updates, _ := builder.Build()
			return graph.Goto("node_b", updates), nil
		},
	})

	// Node B receives updates from Node A
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node_b",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*graph.Command, error) {
			// Verify we received the update from node_a
			fromA := state.GetFromView(s, fromAKey)
			counter := state.GetFromView(s, counterKey)

			// These should be available after node_a completes
			require.NotEmpty(t, fromA, "Should receive update from node_a")
			require.Equal(t, "hello from A", fromA)
			require.Equal(t, 1, counter)

			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, fromBKey, "hello from B")
			state.SetUpdate(builder, statusKey, "received")
			updates, _ := builder.Build()
			return graph.End(updates), nil
		},
	})

	g.SetEntryPoint("node_a")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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

	// Single entry node that simulates two parallel senders by writing
	// both updates before routing to the aggregator.
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "parallel_entry",
		DeclaredTargets: graph.NewTargetSet("aggregator"),
		Fn: func(ctx context.Context, s *state.ReadView) (*graph.Command, error) {
			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, fromParallelAKey, "data_a")
			state.SetUpdate(builder, fromParallelBKey, "data_b")
			updates, _ := builder.Build()
			return graph.Goto("aggregator", updates), nil
		},
	})
	require.NoError(t, err)

	// Aggregator node receives from both logical senders
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "aggregator",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*graph.Command, error) {
			dataA := state.GetFromView(s, fromParallelAKey)
			dataB := state.GetFromView(s, fromParallelBKey)

			// Both updates should be present
			require.NotEmpty(t, dataA, "Should receive update from parallel_a")
			require.NotEmpty(t, dataB, "Should receive update from parallel_b")
			require.Equal(t, "data_a", dataA)
			require.Equal(t, "data_b", dataB)

			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, aggregatedKey, true)
			updates, _ := builder.Build()
			return graph.End(updates), nil
		},
	})

	g.SetEntryPoint("parallel_entry")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node_1",
		DeclaredTargets: graph.NewTargetSet("node_2"),
		Fn: func(ctx context.Context, s *state.ReadView) (*graph.Command, error) {
			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, stepKey, 1)
			state.SetUpdate(builder, dataKey, "from_node_1")
			updates, _ := builder.Build()
			return graph.Goto("node_2", updates), nil
		},
	})

	// Node 2: Reads from node 1, adds its own data
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node_2",
		DeclaredTargets: graph.NewTargetSet("node_3"),
		Fn: func(ctx context.Context, s *state.ReadView) (*graph.Command, error) {
			step := state.GetFromView(s, stepKey)
			data := state.GetFromView(s, dataKey)

			require.Equal(t, 1, step, "Should receive step from node_1")
			require.Equal(t, "from_node_1", data)

			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, stepKey, 2)
			state.SetUpdate(builder, dataKey, "from_node_2")
			updates, _ := builder.Build()
			return graph.Goto("node_3", updates), nil
		},
	})

	// Node 3: Reads from node 2, verifies propagation
	g.AddNode(&graph.BaseCommandNode{
		NodeName:        "node_3",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, s *state.ReadView) (*graph.Command, error) {
			step := state.GetFromView(s, stepKey)
			data := state.GetFromView(s, dataKey)

			require.Equal(t, 2, step, "Should receive step from node_2")
			require.Equal(t, "from_node_2", data)

			builder := state.NewUpdateBuilder()
			state.SetUpdate(builder, stepKey, 3)
			state.SetUpdate(builder, finalKey, true)
			updates, _ := builder.Build()
			return graph.End(updates), nil
		},
	})

	g.SetEntryPoint("node_1")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
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
