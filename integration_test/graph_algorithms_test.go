package integration_test

import (
	"context"
	"math"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/command"
	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
)

// TestPageRank verifies PageRank computation using actual graph structure.
// Graph topology: A → B, B → C, C → A (simple cycle)
// Each node receives contributions from predecessors in the graph.
func TestPageRank(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stateManager := newTestManager()

	const (
		dampingFactor = 0.85
		iterations    = 10
		tolerance     = 0.001
		numVertices   = 3
	)

	initialRank := 1.0 / float64(numVertices)

	// Rank keys for each node
	rankA := state.NewKey("rank_A", initialRank)
	rankB := state.NewKey("rank_B", initialRank)
	rankC := state.NewKey("rank_C", initialRank)

	state.RegisterKey(stateManager, rankA)
	state.RegisterKey(stateManager, rankB)
	state.RegisterKey(stateManager, rankC)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node A: receives from C, sends to B
	// PageRank formula: (1-d)/N + d * (sum of contributions from incoming links)
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "A",
		DeclaredTargets: []string{"B"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// A receives contribution from C (C has only 1 outgoing edge to A)
			rankCValue := state.GetFromView(view, rankC)
			contribFromC := rankCValue / 1.0 // C sends all its rank to A

			newRank := (1-dampingFactor)/float64(numVertices) + dampingFactor*contribFromC

			return command.New().
				With(command.SetValue(rankA, newRank)).
				To("B")
		},
	})
	require.NoError(t, err)

	// Node B: receives from A, sends to C
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "B",
		DeclaredTargets: []string{"C"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// B receives contribution from A (A has only 1 outgoing edge to B)
			rankAValue := state.GetFromView(view, rankA)
			contribFromA := rankAValue / 1.0

			newRank := (1-dampingFactor)/float64(numVertices) + dampingFactor*contribFromA

			return command.New().
				With(command.SetValue(rankB, newRank)).
				To("C")
		},
	})
	require.NoError(t, err)

	// Node C: receives from B, sends to A
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "C",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// C receives contribution from B (B has only 1 outgoing edge to C)
			rankBValue := state.GetFromView(view, rankB)
			contribFromB := rankBValue / 1.0

			newRank := (1-dampingFactor)/float64(numVertices) + dampingFactor*contribFromB

			return command.New().
				With(command.SetValue(rankC, newRank)).
				To(graph.EndNode)
		},
	})
	require.NoError(t, err)

	// Start from all nodes in parallel to update all ranks simultaneously
	g.SetEntryPoint("A")
	g.SetEntryPoint("B")
	g.SetEntryPoint("C")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	require.NoError(t, err)

	// Run multiple iterations
	for range iterations {
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}
	}

	// Verify ranks sum to approximately 1.0
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView failed: %v", err)
	}

	rankAVal := state.GetFromView(view, rankA)
	rankBVal := state.GetFromView(view, rankB)
	rankCVal := state.GetFromView(view, rankC)

	totalRank := rankAVal + rankBVal + rankCVal

	require.Greater(t, rankAVal, 0.0, "rank for A should be positive")
	require.Greater(t, rankBVal, 0.0, "rank for B should be positive")
	require.Greater(t, rankCVal, 0.0, "rank for C should be positive")
	require.InDelta(t, 1.0, totalRank, tolerance, "total PageRank should sum to 1.0")

	// In a cycle with equal edge weights, all ranks should converge to 1/N
	require.InDelta(t, 1.0/3.0, rankAVal, 0.01, "rank A should converge to ~0.333")
	require.InDelta(t, 1.0/3.0, rankBVal, 0.01, "rank B should converge to ~0.333")
	require.InDelta(t, 1.0/3.0, rankCVal, 0.01, "rank C should converge to ~0.333")
}

// TestShortestPath implements shortest path using actual graph nodes as vertices.
// Graph topology: A --(1)--> B --(1)--> C
//
//	A --(5)--> C (direct but longer path)
//
// Each node relaxes its neighbors' distances.
func TestShortestPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stateManager := newTestManager()

	// Distance keys for each vertex
	distA := state.NewKey("dist_A", 0)
	distB := state.NewKey("dist_B", math.MaxInt32)
	distC := state.NewKey("dist_C", math.MaxInt32)

	state.RegisterKey(stateManager, distA)
	state.RegisterKey(stateManager, distB)
	state.RegisterKey(stateManager, distC)

	g, err := graph.NewGraph(stateManager)
	require.NoError(t, err)

	// Node A: source node, relaxes B (distance 1) and C (distance 5)
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "A",
		DeclaredTargets: []string{"B", "C"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			myDist := state.GetFromView(view, distA)
			if myDist == math.MaxInt32 {
				return []string{graph.EndNode}, nil, nil
			}

			cmd := command.New()
			// Relax B: edge weight 1
			newDistB := myDist + 1
			if newDistB < state.GetFromView(view, distB) {
				cmd = cmd.With(command.SetValue(distB, newDistB))
			}

			// Relax C: edge weight 5
			newDistC := myDist + 5
			if newDistC < state.GetFromView(view, distC) {
				cmd = cmd.With(command.SetValue(distC, newDistC))
			}

			return cmd.To("B", "C")
		},
	})
	require.NoError(t, err)

	// Node B: relaxes C (distance 1)
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "B",
		DeclaredTargets: []string{"C"},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			myDist := state.GetFromView(view, distB)
			if myDist == math.MaxInt32 {
				return []string{graph.EndNode}, nil, nil
			}

			cmd := command.New()
			// Relax C: edge weight 1
			newDistC := myDist + 1
			if newDistC < state.GetFromView(view, distC) {
				cmd = cmd.With(command.SetValue(distC, newDistC))
			}

			return cmd.To("C")
		},
	})
	require.NoError(t, err)

	// Node C: sink node, no outgoing edges
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "C",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			return []string{graph.EndNode}, nil, nil
		},
	})
	require.NoError(t, err)

	g.SetEntryPoint("A")

	compiled, err := graph.Compile(g, graph.NewMessagePregelExecutor())
	require.NoError(t, err)

	// Run multiple iterations for convergence
	maxIterations := 3
	for i := 0; i < maxIterations; i++ {
		for _, err := range compiled.Run(context.Background(), nil) {
			require.NoError(t, err)
		}
	}

	// Verify shortest paths
	view, err := stateManager.CreateReadView(ctx)
	if err != nil {
		t.Fatalf("CreateReadView failed: %v", err)
	}

	require.Equal(t, 0, state.GetFromView(view, distA), "source distance should be 0")
	require.Equal(t, 1, state.GetFromView(view, distB), "A -> B shortest path should be 1")
	require.Equal(t, 2, state.GetFromView(view, distC), "A -> B -> C shortest path should be 2 (not A -> C = 5)")
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

	err = g.AddNode(&graph.BaseNode{
		NodeName:        "incrementer",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			count := state.GetFromView(view, counterKey)
			target := state.GetFromView(view, targetKey)

			if count >= target {
				// Converged - return empty result
				return []string{graph.EndNode}, nil, nil
			}

			return command.New().
				With(command.SetValue(counterKey, count+1)).
				To(graph.EndNode)
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
	err = g.AddNode(&graph.BaseNode{
		NodeName:        "halvinator",
		DeclaredTargets: []string{graph.EndNode},
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			value := state.GetFromView(view, valueKey)
			iteration := state.GetFromView(view, iterationKey)

			return command.New().
				With(command.SetValue(valueKey, value/2.0)).
				With(command.SetValue(iterationKey, iteration+1)).
				To(graph.EndNode)
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
