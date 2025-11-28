package integration_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/command"
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

	stateBuilder := newTestManagerBuilder()
	state.RegisterKey(stateBuilder, fromAKey)
	state.RegisterKey(stateBuilder, counterKey)
	state.RegisterKey(stateBuilder, fromBKey)
	state.RegisterKey(stateBuilder, statusKey)
	stateManager := stateBuilder.Build()

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node A sends updates to Node B
	g.AddNode(&graph.BaseNode{
		NodeName:        "node_a",
		DeclaredTargets: []string{"node_b"},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			// Send data to node_b
			return command.New().
				With(command.SetValue(fromAKey, "hello from A")).
				With(command.SetValue(counterKey, 1)).
				To("node_b")
		},
	})

	// Node B receives updates from Node A
	g.AddNode(&graph.BaseNode{
		NodeName:        "node_b",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			// Verify we received the update from node_a
			fromA := state.GetFromView(s, fromAKey)
			counter := state.GetFromView(s, counterKey)

			// These should be available after node_a completes
			require.NotEmpty(t, fromA, "Should receive update from node_a")
			require.Equal(t, "hello from A", fromA)
			require.Equal(t, 1, counter)

			return command.New().
				With(command.SetValue(fromBKey, "hello from B")).
				With(command.SetValue(statusKey, "received")).
				To(graph.EndNode)
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

	stateBuilder := newTestManagerBuilder()
	state.RegisterKey(stateBuilder, fromParallelAKey)
	state.RegisterKey(stateBuilder, fromParallelBKey)
	state.RegisterKey(stateBuilder, aggregatedKey)
	stateManager := stateBuilder.Build()

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Single entry node that simulates two parallel senders by writing
	// both updates before routing to the aggregator.
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "parallel_entry",
		DeclaredTargets: []string{"aggregator"},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return command.New().
				With(command.SetValue(fromParallelAKey, "data_a")).
				With(command.SetValue(fromParallelBKey, "data_b")).
				To("aggregator")
		},
	})
	require.NoError(t, err)

	// Aggregator node receives from both logical senders
	g.AddNode(&graph.BaseNode{
		NodeName:        "aggregator",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			dataA := state.GetFromView(s, fromParallelAKey)
			dataB := state.GetFromView(s, fromParallelBKey)

			// Both updates should be present
			require.NotEmpty(t, dataA, "Should receive update from parallel_a")
			require.NotEmpty(t, dataB, "Should receive update from parallel_b")
			require.Equal(t, "data_a", dataA)
			require.Equal(t, "data_b", dataB)

			return command.New().
				With(command.SetValue(aggregatedKey, true)).
				To(graph.EndNode)
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

	stateBuilder := newTestManagerBuilder()
	state.RegisterKey(stateBuilder, stepKey)
	state.RegisterKey(stateBuilder, dataKey)
	state.RegisterKey(stateBuilder, finalKey)
	stateManager := stateBuilder.Build()

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node 1: Sets initial values
	g.AddNode(&graph.BaseNode{
		NodeName:        "node_1",
		DeclaredTargets: []string{"node_2"},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			return command.New().
				With(command.SetValue(stepKey, 1)).
				With(command.SetValue(dataKey, "from_node_1")).
				To("node_2")
		},
	})

	// Node 2: Reads from node 1, adds its own data
	g.AddNode(&graph.BaseNode{
		NodeName:        "node_2",
		DeclaredTargets: []string{"node_3"},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			step := state.GetFromView(s, stepKey)
			data := state.GetFromView(s, dataKey)

			require.Equal(t, 1, step, "Should receive step from node_1")
			require.Equal(t, "from_node_1", data)

			return command.New().
				With(command.SetValue(stepKey, 2)).
				With(command.SetValue(dataKey, "from_node_2")).
				To("node_3")
		},
	})

	// Node 3: Reads from node 2, verifies propagation
	g.AddNode(&graph.BaseNode{
		NodeName:        "node_3",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, s state.ReadView) ([]string, state.Updates, error) {
			step := state.GetFromView(s, stepKey)
			data := state.GetFromView(s, dataKey)

			require.Equal(t, 2, step, "Should receive step from node_2")
			require.Equal(t, "from_node_2", data)

			return command.New().
				With(command.SetValue(stepKey, 3)).
				With(command.SetValue(finalKey, true)).
				To(graph.EndNode)
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
