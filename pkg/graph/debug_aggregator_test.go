package graph

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/pregel"
	stateif "github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
)

func TestDebugAggregators(t *testing.T) {
	state, err := NewStateManager(0)
	require.NoError(t, err)
	g, err := NewGraph(state)
	require.NoError(t, err)

	require.NoError(t, g.AddNode(&Node{
		Name: "test",
		RunFunc: func(ctx context.Context, s stateif.Writer) (*NodeResult, error) {
			return nil, s.Aggregate("total", 1)
		},
	}))

	g.AddEdge(StartNode, "test")
	g.AddEdge("test", EndNode)

	executor := NewPregelExecutor(WithPregelAggregators(map[string]pregel.Aggregator{
		"total": &SumAggregator{},
	}))

	fmt.Printf("DEBUG: Executor created, aggregators: %v\n", executor.getAggregators())
	require.NoError(t, g.SetExecutor(executor))

	cg, err := g.Compile()
	require.NoError(t, err)

	fmt.Printf("DEBUG: Compiled executor type: %T\n", cg.executor)
	if pe, ok := cg.executor.(*PregelExecutor); ok {
		fmt.Printf("DEBUG: Is PregelExecutor, aggs: %v\n", pe.getAggregators())
	}

	fmt.Printf("DEBUG: About to run graph\n")
	result, err := Last(cg.Run(context.Background(), nil))
	require.NoError(t, err)
	fmt.Printf("DEBUG: Graph completed, result: %v\n", result)

	aggregates := cg.State().AggregatesSnapshot()
	fmt.Printf("DEBUG: Final aggregates: %v\n", aggregates)
	require.NotNil(t, aggregates["total"])
}
