package integration_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/exec"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
)

// TestPageRank implements the PageRank algorithm to verify iterative convergence
func TestPageRank(t *testing.T) {
	t.Parallel()

	const (
		dampingFactor = 0.85
		iterations    = 10
		tolerance     = 0.001
	)

	// Create a simple graph: A -> B, A -> C, B -> C, C -> A
	stateManager := newTestState()

	vertices := []string{"A", "B", "C"}
	edges := map[string][]string{
		"A": {"B", "C"},
		"B": {"C"},
		"C": {"A"},
	}

	// Create typed keys for each vertex
	rankKeys := make(map[string]state.Key[float64])
	outgoingKeys := make(map[string]state.Key[[]string])
	contribKeys := make(map[string]map[string]state.Key[float64])

	initialRank := 1.0 / float64(len(vertices))

	for _, v := range vertices {
		rankKeys[v] = state.NewKey(fmt.Sprintf("rank_%s", v), initialRank)
		state.Register(stateManager, rankKeys[v])

		outgoingKeys[v] = state.NewKey(fmt.Sprintf("outgoing_%s", v), edges[v])
		state.Register(stateManager, outgoingKeys[v])
	}

	// Create contribution keys for each edge
	contribKeys = make(map[string]map[string]state.Key[float64])
	for _, source := range vertices {
		contribKeys[source] = make(map[string]state.Key[float64])
		for _, target := range vertices {
			key := state.NewKey(fmt.Sprintf("contrib_%s_%s", source, target), 0.0)
			state.Register(stateManager, key)
			contribKeys[source][target] = key
		}
	}

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Create compute nodes for each vertex
	for _, vertex := range vertices {
		v := vertex // capture loop variable
		err = g.AddNode(&graph.Node{
			Name: v,
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				// Get outgoing edges
				outgoingEdges := state.GetFromView(view, outgoingKeys[v])
				if len(outgoingEdges) == 0 {
					return &graph.NodeResult{}, nil
				}

				// Get current rank
				rank := state.GetFromView(view, rankKeys[v])

				// Distribute rank to outgoing neighbors
				contribution := rank / float64(len(outgoingEdges))

				updates := make(map[string]any)
				for _, target := range outgoingEdges {
					key := fmt.Sprintf("contrib_%s_%s", v, target)
					updates[key] = contribution
				}

				return &graph.NodeResult{Updates: updates}, nil
			},
		})
		require.NoError(t, err)
		g.AddEdge(graph.StartNode, v)
	}

	compiled, err := exec.CompileGraph(g)
	require.NoError(t, err)

	// Run multiple iterations
	for i := 0; i < iterations; i++ {
		// Execute one iteration
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}

		// Get snapshot to read current state
		snap := stateManager.Snapshot()
		view := state.NewReadView(snap)

		// Accumulate contributions for each vertex
		newRanks := make(map[string]float64)
		for _, v := range vertices {
			// Sum all contributions to this vertex
			totalContrib := 0.0
			for _, source := range vertices {
				contrib := state.GetFromView(view, contribKeys[source][v])
				totalContrib += contrib
			}

			// Apply PageRank formula: (1-d)/N + d * sum(contributions)
			newRank := (1-dampingFactor)/float64(len(vertices)) + dampingFactor*totalContrib
			newRanks[v] = newRank
		}

		// Update all ranks for next iteration
		updates := make(map[string]any)
		for v, rank := range newRanks {
			updates[fmt.Sprintf("rank_%s", v)] = rank
		}
		stateManager.ApplyUpdates(context.Background(), updates)
	}

	// Verify ranks sum to approximately 1.0
	snap := stateManager.Snapshot()
	view := state.NewReadView(snap)

	totalRank := 0.0
	for _, v := range vertices {
		rank := state.GetFromView(view, rankKeys[v])
		totalRank += rank
		// Each rank should be positive
		require.Greater(t, rank, 0.0, "rank for %s should be positive", v)
	}

	// Total should be close to 1.0 (allowing for floating point errors)
	require.InDelta(t, 1.0, totalRank, tolerance, "total PageRank should sum to 1.0")
}

// TestShortestPath implements Dijkstra-style single-source shortest path
func TestShortestPath(t *testing.T) {
	t.Parallel()

	stateManager := newTestState()

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
		state.Register(stateManager, distKeys[v])

		edgeKeys[v] = state.NewKey(fmt.Sprintf("edges_%s", v), edges[v])
		state.Register(stateManager, edgeKeys[v])
	}

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Create relaxation nodes
	for _, vertex := range vertices {
		v := vertex
		err = g.AddNode(&graph.Node{
			Name: v,
			RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
				dist := state.GetFromView(view, distKeys[v])
				if dist == math.MaxInt32 {
					return &graph.NodeResult{}, nil
				}

				neighbors := state.GetFromView(view, edgeKeys[v])
				if len(neighbors) == 0 {
					return &graph.NodeResult{}, nil
				}

				updates := make(map[string]any)
				for neighbor, weight := range neighbors {
					newDist := dist + weight
					currentNeighborDist := state.GetFromView(view, distKeys[neighbor])

					if newDist < currentNeighborDist {
						updates[fmt.Sprintf("dist_%s", neighbor)] = newDist
					}
				}

				return &graph.NodeResult{Updates: updates}, nil
			},
		})
		require.NoError(t, err)
		g.AddEdge(graph.StartNode, v)
	}

	compiled, err := exec.CompileGraph(g)
	require.NoError(t, err)

	// Run multiple supersteps for convergence
	maxSupersteps := len(vertices)
	for i := 0; i < maxSupersteps; i++ {
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}
	}

	// Verify shortest paths
	snap := stateManager.Snapshot()
	view := state.NewReadView(snap)

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

	counterKey := state.NewKey("counter", 0)
	targetKey := state.NewKey("target", 10)

	stateManager := newTestState()
	state.Register(stateManager, counterKey)
	state.Register(stateManager, targetKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	err = g.AddNode(&graph.Node{
		Name: "incrementer",
		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			count := state.GetFromView(view, counterKey)
			target := state.GetFromView(view, targetKey)

			if count >= target {
				// Converged - return empty result
				return &graph.NodeResult{}, nil
			}

			return &graph.NodeResult{
				Updates: map[string]any{
					"counter": count + 1,
				},
			}, nil
		},
	})
	require.NoError(t, err)

	g.AddEdge(graph.StartNode, "incrementer")
	g.AddEdge("incrementer", graph.EndNode)

	compiled, err := exec.CompileGraph(g)
	require.NoError(t, err)

	// Run the graph once - single pass execution
	for _, err := range compiled.Run(context.Background(), nil) {
		require.NoError(t, err)
	}

	// Verify counter was incremented once (single execution)
	snap := stateManager.Snapshot()
	view := state.NewReadView(snap)
	count := state.GetFromView(view, counterKey)
	require.Equal(t, 1, count, "counter should increment once per invocation")
}

// TestIterativeComputation verifies multiple graph executions for convergence
func TestIterativeComputation(t *testing.T) {
	t.Parallel()

	valueKey := state.NewKey("value", 1.0)
	iterationKey := state.NewKey("iteration", 0)

	stateManager := newTestState()
	state.Register(stateManager, valueKey)
	state.Register(stateManager, iterationKey)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node that halves the value each iteration
	err = g.AddNode(&graph.Node{
		Name: "halvinator",
		RunFunc: func(ctx context.Context, view *state.ReadView) (*graph.NodeResult, error) {
			value := state.GetFromView(view, valueKey)
			iteration := state.GetFromView(view, iterationKey)

			return &graph.NodeResult{
				Updates: map[string]any{
					"value":     value / 2.0,
					"iteration": iteration + 1,
				},
			}, nil
		},
	})
	require.NoError(t, err)

	g.AddEdge(graph.StartNode, "halvinator")
	g.AddEdge("halvinator", graph.EndNode)

	compiled, err := exec.CompileGraph(g)
	require.NoError(t, err)

	// Run multiple iterations
	maxIterations := 5
	for i := 0; i < maxIterations; i++ {
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}
	}

	// Verify convergence
	snap := stateManager.Snapshot()
	view := state.NewReadView(snap)

	finalValue := state.GetFromView(view, valueKey)
	expectedValue := 1.0 / math.Pow(2, float64(maxIterations))
	require.InDelta(t, expectedValue, finalValue, 0.0001, "value should converge to 1/(2^iterations)")

	finalIteration := state.GetFromView(view, iterationKey)
	require.Equal(t, maxIterations, finalIteration, "should track iteration count")
}
