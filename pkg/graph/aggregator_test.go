package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sumAggregator struct{}

func (sumAggregator) Zero() any { return 0 }

func (sumAggregator) Aggregate(current, value any) any {
	cur, _ := current.(int)
	inc, _ := value.(int)
	return cur + inc
}

func TestGraphAggregatorsAccessible(t *testing.T) {
	state := NewGraphState(0)
	g := NewGraph(state)

	require.NoError(t, g.AddNode(&Node{
		Name: "count",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			snap := s.AggregatesSnapshot()
			if snap != nil {
				assert.Equal(t, 0, snap["total"])
			}
			require.NoError(t, s.Aggregate("total", 1))
			return nil, nil
		},
	}))

	require.NoError(t, g.AddNode(&Node{
		Name: "report",
		RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
			snap := s.AggregatesSnapshot()
			total, _ := snap["total"].(int)
			return &NodeResult{Updates: map[string]any{"observed": total}}, nil
		},
	}))

	g.AddEdge(StartNode, "count")
	g.AddEdge("count", "report")

	cg, err := g.Compile()
	require.NoError(t, err)

	_, err = cg.Invoke(context.Background(), nil, WithAggregators(map[string]Aggregator{"total": sumAggregator{}}))
	require.NoError(t, err)

	// v2.0: Use cg.State() instead of original state variable
	value := cg.State().Get("observed")
	require.Equal(t, 1, value)

	aggregates := cg.State().AggregatesSnapshot()
	require.Equal(t, 1, aggregates["total"])
}
