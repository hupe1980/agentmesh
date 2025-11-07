package graph

import (
	"context"
	"fmt"
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

// Test built-in aggregators

func TestAvgAggregator(t *testing.T) {
	agg := &AvgAggregator{}

	// Start with zero
	state := agg.Zero()
	assert.Equal(t, avgState{Mean: 0, Count: 0}, state)

	// Add values
	state = agg.Aggregate(state, 10)
	s := state.(avgState)
	assert.Equal(t, float64(10), s.Mean)
	assert.Equal(t, int64(1), s.Count)

	state = agg.Aggregate(state, 20)
	s = state.(avgState)
	assert.Equal(t, float64(15), s.Mean) // (10+20)/2
	assert.Equal(t, int64(2), s.Count)

	state = agg.Aggregate(state, 30)
	s = state.(avgState)
	assert.Equal(t, float64(20), s.Mean) // (10+20+30)/3
	assert.Equal(t, int64(3), s.Count)

	// Test with different numeric types
	state = agg.Aggregate(state, int32(40))
	s = state.(avgState)
	assert.Equal(t, float64(25), s.Mean) // (10+20+30+40)/4
	assert.Equal(t, int64(4), s.Count)

	state = agg.Aggregate(state, float32(50.0))
	s = state.(avgState)
	assert.Equal(t, float64(30), s.Mean) // (10+20+30+40+50)/5
	assert.Equal(t, int64(5), s.Count)

	state = agg.Aggregate(state, float64(60.0))
	s = state.(avgState)
	assert.InDelta(t, 35.0, s.Mean, 0.0001) // (10+20+30+40+50+60)/6
	assert.Equal(t, int64(6), s.Count)
}

func TestAvgAggregator_InvalidValues(t *testing.T) {
	agg := &AvgAggregator{}
	state := agg.Zero()

	// Add valid value
	state = agg.Aggregate(state, 10)
	s := state.(avgState)
	assert.Equal(t, float64(10), s.Mean)
	assert.Equal(t, int64(1), s.Count)

	// Invalid values should be ignored
	state = agg.Aggregate(state, "invalid")
	s = state.(avgState)
	assert.Equal(t, float64(10), s.Mean) // Unchanged
	assert.Equal(t, int64(1), s.Count)   // Unchanged

	state = agg.Aggregate(state, nil)
	s = state.(avgState)
	assert.Equal(t, float64(10), s.Mean)
	assert.Equal(t, int64(1), s.Count)
}

func TestVarianceAggregator(t *testing.T) {
	agg := &VarianceAggregator{}

	// Start with zero
	state := agg.Zero()
	assert.Equal(t, varianceState{Mean: 0, M2: 0, Count: 0}, state)

	// Add values: [10, 20, 30]
	// Mean = 20, Variance = ((10-20)^2 + (20-20)^2 + (30-20)^2) / 3 = 66.67
	state = agg.Aggregate(state, 10)
	state = agg.Aggregate(state, 20)
	state = agg.Aggregate(state, 30)

	s := state.(varianceState)
	assert.Equal(t, float64(20), s.Mean)
	assert.Equal(t, int64(3), s.Count)

	// Variance = M2 / count
	variance := s.M2 / float64(s.Count)
	assert.InDelta(t, 66.67, variance, 0.01)
}

func TestVarianceAggregator_UniformValues(t *testing.T) {
	agg := &VarianceAggregator{}
	state := agg.Zero()

	// All same values should have zero variance
	state = agg.Aggregate(state, 5)
	state = agg.Aggregate(state, 5)
	state = agg.Aggregate(state, 5)
	state = agg.Aggregate(state, 5)

	s := state.(varianceState)
	assert.Equal(t, float64(5), s.Mean)
	variance := s.M2 / float64(s.Count)
	assert.Equal(t, float64(0), variance)
}

func TestVarianceAggregator_DifferentTypes(t *testing.T) {
	agg := &VarianceAggregator{}
	state := agg.Zero()

	// Test with different numeric types
	state = agg.Aggregate(state, int(10))
	state = agg.Aggregate(state, int32(20))
	state = agg.Aggregate(state, int64(30))
	state = agg.Aggregate(state, float32(40.0))
	state = agg.Aggregate(state, float64(50.0))

	s := state.(varianceState)
	assert.Equal(t, float64(30), s.Mean)
	assert.Equal(t, int64(5), s.Count)

	// Variance of [10, 20, 30, 40, 50]
	// Mean = 30, Variance = 200
	variance := s.M2 / float64(s.Count)
	assert.InDelta(t, 200.0, variance, 0.01)
}

func TestVarianceAggregator_InvalidValues(t *testing.T) {
	agg := &VarianceAggregator{}
	state := agg.Zero()

	// Add valid value
	state = agg.Aggregate(state, 10)
	s := state.(varianceState)
	assert.Equal(t, float64(10), s.Mean)
	assert.Equal(t, int64(1), s.Count)

	// Invalid values should be ignored
	state = agg.Aggregate(state, "invalid")
	s = state.(varianceState)
	assert.Equal(t, float64(10), s.Mean)
	assert.Equal(t, int64(1), s.Count)
}

func TestAvgAggregator_Integration(t *testing.T) {
	state := NewGraphState(0)
	g := NewGraph(state)

	// Three nodes each reporting a value
	values := []int{10, 20, 30}
	for i, val := range values {
		v := val // Capture for closure
		nodeName := fmt.Sprintf("node%d", i+1)
		require.NoError(t, g.AddNode(&Node{
			Name: nodeName,
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return &NodeResult{}, s.Aggregate("avg_value", v)
			},
		}))
		g.AddEdge(StartNode, nodeName)
		g.AddEdge(nodeName, EndNode)
	}

	cg, err := g.Compile()
	require.NoError(t, err)

	_, err = cg.Invoke(context.Background(), nil, WithAggregators(map[string]Aggregator{
		"avg_value": &AvgAggregator{},
	}))
	require.NoError(t, err)

	// Check aggregates after graph completes
	aggregates := cg.State().AggregatesSnapshot()
	require.NotNil(t, aggregates, "aggregates snapshot should not be nil")
	require.Contains(t, aggregates, "avg_value", "avg_value should be in aggregates")

	avgState, ok := aggregates["avg_value"].(avgState)
	require.True(t, ok, "avg_value should be avgState type, got %T", aggregates["avg_value"])
	assert.Equal(t, float64(20), avgState.Mean) // (10+20+30)/3
	assert.Equal(t, int64(3), avgState.Count)
}

func TestVarianceAggregator_Integration(t *testing.T) {
	state := NewGraphState(0)
	g := NewGraph(state)

	values := []int{10, 20, 30}
	for i, val := range values {
		v := val // Capture for closure
		nodeName := fmt.Sprintf("node%d", i+1)
		require.NoError(t, g.AddNode(&Node{
			Name: nodeName,
			RunFunc: func(ctx context.Context, s StateWriter) (*NodeResult, error) {
				return nil, s.Aggregate("variance", v)
			},
		}))
		g.AddEdge(StartNode, nodeName)
		g.AddEdge(nodeName, EndNode)
	}

	cg, err := g.Compile()
	require.NoError(t, err)

	_, err = cg.Invoke(context.Background(), nil, WithAggregators(map[string]Aggregator{
		"variance": &VarianceAggregator{},
	}))
	require.NoError(t, err)

	aggregates := cg.State().AggregatesSnapshot()
	varState := aggregates["variance"].(varianceState)

	variance := varState.M2 / float64(varState.Count)
	assert.InDelta(t, 66.67, variance, 0.01) // Variance of [10, 20, 30]
}
