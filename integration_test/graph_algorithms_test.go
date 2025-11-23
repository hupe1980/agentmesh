package integration_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
)

// TestPageRank verifies we can run a simple iterative
// PageRank-like computation using a single Command-based entry node.
func TestPageRank(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stateManager := newTestManager()

	const (
		dampingFactor = 0.85
		iterations    = 10
		tolerance     = 0.001
	)

	// Three vertices with a fixed link structure encoded in state.
	vertices := []string{"A", "B", "C"}
	edges := map[string][]string{
		"A": {"B", "C"},
		"B": {"C"},
		"C": {"A"},
	}

	// Per-vertex rank and outgoing-edge keys.
	rankKeys := make(map[string]state.Key[float64])
	outgoingKeys := make(map[string]state.Key[[]string])

	initialRank := 1.0 / float64(len(vertices))
	for _, v := range vertices {
		rankKeys[v] = state.NewKey(fmt.Sprintf("rank_%s", v), initialRank)
		state.RegisterKey(stateManager, rankKeys[v])

		outgoingKeys[v] = state.NewKey(fmt.Sprintf("outgoing_%s", v), edges[v])
		state.RegisterKey(stateManager, outgoingKeys[v])
	}

	// Single node that performs one PageRank iteration per execution.
	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "pagerank",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			// Compute contributions from current ranks.
			contrib := make(map[string]float64)
			for _, v := range vertices {
				outgoing := state.GetFromView(view, outgoingKeys[v])
				if len(outgoing) == 0 {
					continue
				}
				rank := state.GetFromView(view, rankKeys[v])
				share := rank / float64(len(outgoing))
				for _, target := range outgoing {
					contrib[target] += share
				}
			}

			updates := make(map[string]any)
			for _, v := range vertices {
				newRank := (1-dampingFactor)/float64(len(vertices)) + dampingFactor*contrib[v]
				updates[fmt.Sprintf("rank_%s", v)] = newRank
			}

			return graph.End(updates), nil
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("pagerank")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	require.NoError(t, err)

	// Run multiple iterations via repeated graph execution.
	for i := 0; i < iterations; i++ {
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}
	}

	// Verify ranks sum to approximately 1.0.
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView failed: %v", err)
	}

	totalRank := 0.0
	for _, v := range vertices {
		rank := state.GetFromView(view, rankKeys[v])
		totalRank += rank
		require.Greater(t, rank, 0.0, "rank for %s should be positive", v)
	}

	require.InDelta(t, 1.0, totalRank, tolerance, "total PageRank should sum to 1.0")
}

// TestShortestPath implements a simple single-source shortest path
// relaxation using a single Command-based entry node.
func TestShortestPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stateManager := newTestManager()

	// Create graph: A -1-> B -1-> C
	//                A -5-> C
	vertices := []string{"A", "B", "C"}
	edges := map[string]map[string]int{
		"A": {"B": 1, "C": 5},
		"B": {"C": 1},
		"C": {},
	}

	// Create typed keys
	distKeys := make(map[string]state.Key[int])
	edgeKeys := make(map[string]state.Key[map[string]int])

	for _, v := range vertices {
		initialDist := math.MaxInt32
		if v == "A" {
			initialDist = 0 // source
		}
		distKeys[v] = state.NewKey(fmt.Sprintf("dist_%s", v), initialDist)
		state.RegisterKey(stateManager, distKeys[v])

		edgeKeys[v] = state.NewKey(fmt.Sprintf("edges_%s", v), edges[v])
		state.RegisterKey(stateManager, edgeKeys[v])
	}

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Single node that performs relaxation for all vertices per execution.
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "shortest_path",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			updates := make(map[string]any)
			for _, v := range vertices {
				dist := state.GetFromView(view, distKeys[v])
				if dist == math.MaxInt32 {
					continue
				}

				neighbors := state.GetFromView(view, edgeKeys[v])
				for neighbor, weight := range neighbors {
					newDist := dist + weight
					currentNeighborDist := state.GetFromView(view, distKeys[neighbor])
					if newDist < currentNeighborDist {
						updates[fmt.Sprintf("dist_%s", neighbor)] = newDist
					}
				}
			}

			return graph.End(updates), nil
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("shortest_path")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	require.NoError(t, err)

	// Run multiple supersteps for convergence
	maxSupersteps := len(vertices)
	for i := 0; i < maxSupersteps; i++ {
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}
	}

	// Verify shortest paths
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView failed: %v", err)
	}

	distA := state.GetFromView(view, distKeys["A"])
	require.Equal(t, 0, distA, "source distance should be 0")

	distB := state.GetFromView(view, distKeys["B"])
	require.Equal(t, 1, distB, "A -> B shortest path should be 1")

	distC := state.GetFromView(view, distKeys["C"])
	require.Equal(t, 2, distC, "A -> B -> C shortest path should be 2 (not A -> C = 5)")
}

// TestGraphConvergence verifies that iterative algorithms eventually stabilize
func TestGraphConvergence(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	counterKey := state.NewKey("counter", 0)
	targetKey := state.NewKey("target", 10)

	stateManager := newTestManager()
	state.RegisterKey(stateManager, counterKey)
	state.RegisterKey(stateManager, targetKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "incrementer",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			count := state.GetFromView(view, counterKey)
			target := state.GetFromView(view, targetKey)

			if count >= target {
				// Converged - return empty result
				return graph.End(nil), nil
			}

			updates := map[string]any{
				"counter": count + 1,
			}
			return graph.End(updates), nil
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("incrementer")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	require.NoError(t, err)

	// Run the graph once - single pass execution
	for _, err := range compiled.Run(context.Background(), nil) {
		require.NoError(t, err)
	}

	// Verify counter was incremented once (single execution)
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView failed: %v", err)
	}

	count := state.GetFromView(view, counterKey)
	require.Equal(t, 1, count, "counter should increment once per invocation")
}

// TestIterativeComputation verifies multiple graph executions for convergence
func TestIterativeComputation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	valueKey := state.NewKey("value", 1.0)
	iterationKey := state.NewKey("iteration", 0)

	stateManager := newTestManager()
	state.RegisterKey(stateManager, valueKey)
	state.RegisterKey(stateManager, iterationKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node that halves the value each iteration
	err = g.AddNode(&graph.BaseCommandNode{
		NodeName:        "halvinator",
		DeclaredTargets: graph.NewTargetSet(graph.EndNode),
		Fn: func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
			value := state.GetFromView(view, valueKey)
			iteration := state.GetFromView(view, iterationKey)

			updates := map[string]any{
				"value":     value / 2.0,
				"iteration": iteration + 1,
			}
			return graph.End(updates), nil
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("halvinator")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	require.NoError(t, err)

	// Run multiple iterations
	maxIterations := 5
	for i := 0; i < maxIterations; i++ {
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}
	}

	// Verify convergence
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView failed: %v", err)
	}

	finalValue := state.GetFromView(view, valueKey)
	expectedValue := 1.0 / math.Pow(2, float64(maxIterations))
	require.InDelta(t, expectedValue, finalValue, 0.0001, "value should converge to 1/(2^iterations)")

	finalIteration := state.GetFromView(view, iterationKey)
	require.Equal(t, maxIterations, finalIteration, "should track iteration count")
}
