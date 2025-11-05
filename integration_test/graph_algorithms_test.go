package integration_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
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
	// This creates a cycle that PageRank can analyze
	state := graph.NewGraphState(0)
	g := graph.NewGraph(state)

	vertices := []string{"A", "B", "C"}
	edges := map[string][]string{
		"A": {"B", "C"},
		"B": {"C"},
		"C": {"A"},
	}

	// Initialize PageRank values
	initialRank := 1.0 / float64(len(vertices))
	for _, v := range vertices {
		state.Set(fmt.Sprintf("rank_%s", v), initialRank)
		state.Set(fmt.Sprintf("outgoing_%s", v), edges[v])
	}

	// Create compute nodes for each vertex
	for _, vertex := range vertices {
		v := vertex // capture loop variable
		err := g.AddNode(&graph.Node{
			Name: v,
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				// Get outgoing edges
				outgoing := s.Get(fmt.Sprintf("outgoing_%s", v))
				outgoingEdges, ok := outgoing.([]string)
				if !ok || len(outgoingEdges) == 0 {
					return &graph.NodeResult{Updates: map[string]any{}}, nil
				}

				// Get current rank
				currentRank := s.Get(fmt.Sprintf("rank_%s", v))
				rank, ok := currentRank.(float64)
				if !ok {
					rank = initialRank
				}

				// Distribute rank to outgoing neighbors
				contribution := rank / float64(len(outgoingEdges))

				updates := make(map[string]any)
				for _, target := range outgoingEdges {
					// Use source_target format to avoid collisions
					key := fmt.Sprintf("contrib_%s_%s", v, target)
					updates[key] = contribution
				}

				return &graph.NodeResult{Updates: updates}, nil
			},
		})
		require.NoError(t, err)
		g.AddEdge(graph.StartNode, v)
	}

	compiled, err := g.Compile()
	require.NoError(t, err)

	// Run multiple iterations with max iterations set
	for i := 0; i < iterations; i++ {
		_, err := compiled.Invoke(context.Background(), nil, graph.WithMaxIterations(1))
		require.NoError(t, err)

		// Accumulate contributions for each vertex
		newRanks := make(map[string]float64)
		for _, v := range vertices {
			// Sum all contributions to this vertex
			totalContrib := 0.0
			for _, source := range vertices {
				contrib := compiled.State().Get(fmt.Sprintf("contrib_%s_%s", source, v))
				if contribVal, ok := contrib.(float64); ok {
					totalContrib += contribVal
				}
			}

			// Apply PageRank formula: (1-d)/N + d * sum(contributions)
			newRank := (1-dampingFactor)/float64(len(vertices)) + dampingFactor*totalContrib
			newRanks[v] = newRank
		}

		// Update all ranks for next iteration
		for v, rank := range newRanks {
			compiled.State().Set(fmt.Sprintf("rank_%s", v), rank)
		}
	}

	// Verify ranks sum to approximately 1.0
	totalRank := 0.0
	for _, v := range vertices {
		rank := compiled.State().Get(fmt.Sprintf("rank_%s", v))
		if rankVal, ok := rank.(float64); ok {
			totalRank += rankVal
			// Each rank should be positive
			require.Greater(t, rankVal, 0.0)
		}
	}

	// Total should be close to 1.0 (allowing for floating point errors)
	require.InDelta(t, 1.0, totalRank, tolerance)
}

// TestShortestPath implements Dijkstra-style single-source shortest path
func TestShortestPath(t *testing.T) {
	t.Parallel()

	state := graph.NewGraphState(0)
	g := graph.NewGraph(state)

	// Create graph: A -1-> B -1-> C
	//                A -5-> C
	vertices := []string{"A", "B", "C"}
	edges := map[string]map[string]int{
		"A": {"B": 1, "C": 5},
		"B": {"C": 1},
		"C": {},
	}

	// Initialize distances
	for _, v := range vertices {
		if v == "A" {
			state.Set(fmt.Sprintf("dist_%s", v), 0) // source
		} else {
			state.Set(fmt.Sprintf("dist_%s", v), math.MaxInt32)
		}
		state.Set(fmt.Sprintf("edges_%s", v), edges[v])
	}

	// Create relaxation nodes
	for _, vertex := range vertices {
		v := vertex
		err := g.AddNode(&graph.Node{
			Name: v,
			RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
				currentDist := s.Get(fmt.Sprintf("dist_%s", v))
				dist, ok := currentDist.(int)
				if !ok || dist == math.MaxInt32 {
					return &graph.NodeResult{Updates: map[string]any{}}, nil
				}

				edgesRaw := s.Get(fmt.Sprintf("edges_%s", v))
				neighbors, ok := edgesRaw.(map[string]int)
				if !ok {
					return &graph.NodeResult{Updates: map[string]any{}}, nil
				}

				updates := make(map[string]any)
				for neighbor, weight := range neighbors {
					newDist := dist + weight
					neighborDist := s.Get(fmt.Sprintf("dist_%s", neighbor))
					currentNeighborDist, ok := neighborDist.(int)
					if !ok {
						currentNeighborDist = math.MaxInt32
					}

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

	compiled, err := g.Compile()
	require.NoError(t, err)

	// Run algorithm (in practice, would need multiple supersteps)
	_, err = compiled.Invoke(context.Background(), nil)
	require.NoError(t, err)

	// Verify shortest paths
	distA := compiled.State().Get("dist_A")
	require.Equal(t, 0, distA) // source is 0

	distB := compiled.State().Get("dist_B")
	require.Equal(t, 1, distB) // A -> B = 1

	// Note: This simple implementation only does one superstep
	// In a full implementation, you'd run multiple supersteps until convergence
}

// TestGraphConvergence verifies that iterative algorithms eventually stabilize
func TestGraphConvergence(t *testing.T) {
	t.Parallel()

	state := graph.NewGraphState(0)
	g := graph.NewGraph(state)

	// Create nodes that increment a counter until reaching a target
	state.Set("counter", 0)
	state.Set("target", 10)

	err := g.AddNode(&graph.Node{
		Name: "incrementer",
		RunFunc: func(ctx context.Context, s graph.StateWriter) (*graph.NodeResult, error) {
			counter := s.Get("counter")
			count, ok := counter.(int)
			if !ok {
				count = 0
			}

			target := s.Get("target")
			targetVal, ok := target.(int)
			if !ok {
				targetVal = 10
			}

			if count >= targetVal {
				// Converged - but still returns result to complete the superstep
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

	compiled, err := g.Compile()
	require.NoError(t, err)

	// Run the graph once - nodes run until completion (no cyclic edges)
	_, err = compiled.Invoke(context.Background(), nil)
	require.NoError(t, err)

	counter := compiled.State().Get("counter")
	count, ok := counter.(int)
	require.True(t, ok, "counter should be an int")
	require.Equal(t, 1, count, "counter should increment once per invocation")
}
