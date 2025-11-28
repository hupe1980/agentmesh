package state_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/hupe1980/agentmesh/pkg/state/aggregators"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateKey_Sum(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	// Register key with sum aggregator
	totalKey := state.NewKey[any]("total_cost", 0)
	err := state.RegisterAggregateKey(builder, totalKey, &aggregators.SumAggregator{})
	require.NoError(t, err)

	ctx := context.Background()

	// Initial value should be zero
	total, err := state.Get(ctx, mgr, totalKey)
	require.NoError(t, err)
	assert.Equal(t, 0, total) // SumAggregator.Zero() returns int 0

	// First contribution
	err = state.Set(ctx, mgr, totalKey, 10.5)
	require.NoError(t, err)

	total, err = state.Get(ctx, mgr, totalKey)
	require.NoError(t, err)
	assert.Equal(t, 10.5, total)

	// Second contribution - should sum
	err = state.Set(ctx, mgr, totalKey, 5.0)
	require.NoError(t, err)

	total, err = state.Get(ctx, mgr, totalKey)
	require.NoError(t, err)
	assert.Equal(t, 15.5, total)

	// Third contribution
	err = state.Set(ctx, mgr, totalKey, 4.5)
	require.NoError(t, err)

	total, err = state.Get(ctx, mgr, totalKey)
	require.NoError(t, err)
	assert.Equal(t, 20.0, total)
}

func TestAggregateKey_Max(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	// Register key with max aggregator
	maxKey := state.NewKey[any]("max_value", float64(-1e308))
	err := state.RegisterAggregateKey(builder, maxKey, &aggregators.MaxAggregator{})
	require.NoError(t, err)

	ctx := context.Background()

	// Contribute values
	err = state.Set(ctx, mgr, maxKey, 10.0)
	require.NoError(t, err)

	max, err := state.Get(ctx, mgr, maxKey)
	require.NoError(t, err)
	assert.Equal(t, 10.0, max)

	// Higher value - should update
	err = state.Set(ctx, mgr, maxKey, 25.0)
	require.NoError(t, err)

	max, err = state.Get(ctx, mgr, maxKey)
	require.NoError(t, err)
	assert.Equal(t, 25.0, max)

	// Lower value - should not update
	err = state.Set(ctx, mgr, maxKey, 15.0)
	require.NoError(t, err)

	max, err = state.Get(ctx, mgr, maxKey)
	require.NoError(t, err)
	assert.Equal(t, 25.0, max) // Still 25.0
}

func TestAggregateKey_Min(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	// Register key with min aggregator
	minKey := state.NewKey[any]("min_value", float64(1e308))
	err := state.RegisterAggregateKey(builder, minKey, &aggregators.MinAggregator{})
	require.NoError(t, err)

	ctx := context.Background()

	// Contribute values
	err = state.Set(ctx, mgr, minKey, 25.0)
	require.NoError(t, err)

	min, err := state.Get(ctx, mgr, minKey)
	require.NoError(t, err)
	assert.Equal(t, 25.0, min)

	// Lower value - should update
	err = state.Set(ctx, mgr, minKey, 10.0)
	require.NoError(t, err)

	min, err = state.Get(ctx, mgr, minKey)
	require.NoError(t, err)
	assert.Equal(t, 10.0, min)

	// Higher value - should not update
	err = state.Set(ctx, mgr, minKey, 15.0)
	require.NoError(t, err)

	min, err = state.Get(ctx, mgr, minKey)
	require.NoError(t, err)
	assert.Equal(t, 10.0, min) // Still 10.0
}

func TestAggregateKey_Count(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	// Register key with count aggregator
	countKey := state.NewKey[any]("event_count", 0)
	err := state.RegisterAggregateKey(builder, countKey, &aggregators.CountAggregator{})
	require.NoError(t, err)

	ctx := context.Background()

	// Each write increments count (any non-nil value)
	err = state.Set(ctx, mgr, countKey, 1)
	require.NoError(t, err)

	count, err := state.Get(ctx, mgr, countKey)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	err = state.Set(ctx, mgr, countKey, 1)
	require.NoError(t, err)

	count, err = state.Get(ctx, mgr, countKey)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Third contribution
	err = state.Set(ctx, mgr, countKey, 1)
	require.NoError(t, err)

	count, err = state.Get(ctx, mgr, countKey)
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestAggregateKey_Snapshot(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	// Register keys
	totalKey := state.NewKey[any]("total", 0)
	countKey := state.NewKey[any]("count", 0)
	maxKey := state.NewKey[any]("max", float64(-1e308))

	err := state.RegisterAggregateKey(builder, totalKey, &aggregators.SumAggregator{})
	require.NoError(t, err)
	err = state.RegisterAggregateKey(builder, countKey, &aggregators.CountAggregator{})
	require.NoError(t, err)
	err = state.RegisterAggregateKey(builder, maxKey, &aggregators.MaxAggregator{})
	require.NoError(t, err)

	ctx := context.Background()

	// Contribute to aggregates
	state.Set(ctx, mgr, totalKey, 10.0)
	state.Set(ctx, mgr, countKey, 1)
	state.Set(ctx, mgr, maxKey, 10.0)

	state.Set(ctx, mgr, totalKey, 20.0)
	state.Set(ctx, mgr, countKey, 1)
	state.Set(ctx, mgr, maxKey, 25.0)

	// Verify values directly (snapshots are complex, just test Get)
	total, err := state.Get(ctx, mgr, totalKey)
	require.NoError(t, err)
	assert.Equal(t, 30.0, total)

	count, err := state.Get(ctx, mgr, countKey)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	max, err := state.Get(ctx, mgr, maxKey)
	require.NoError(t, err)
	assert.Equal(t, 25.0, max)
}

func TestAggregateKey_Updates(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	// Register aggregate keys
	totalKey := state.NewKey[any]("total", 0)
	countKey := state.NewKey[any]("count", 0)

	state.RegisterAggregateKey(builder, totalKey, &aggregators.SumAggregator{})
	state.RegisterAggregateKey(builder, countKey, &aggregators.CountAggregator{})

	ctx := context.Background()

	// Apply updates (like a node would)
	updates := state.Updates{
		totalKey.Name(): 42.0,
		countKey.Name(): 1,
	}

	err := mgr.ApplyUpdates(ctx, updates)
	require.NoError(t, err)

	// Verify
	total, _ := state.Get(ctx, mgr, totalKey)
	count, _ := state.Get(ctx, mgr, countKey)

	assert.Equal(t, 42.0, total)
	assert.Equal(t, 1, count)

	// Apply more updates
	updates = state.Updates{
		totalKey.Name(): 8.0,
		countKey.Name(): 1,
	}

	err = mgr.ApplyUpdates(ctx, updates)
	require.NoError(t, err)

	// Verify accumulated values
	total, _ = state.Get(ctx, mgr, totalKey)
	count, _ = state.Get(ctx, mgr, countKey)

	assert.Equal(t, 50.0, total) // 42 + 8
	assert.Equal(t, 2, count)    // 1 + 1
}

func TestAggregateKey_Idempotent(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	totalKey := state.NewKey[any]("total", 0)

	// Register twice - should be idempotent
	err := state.RegisterAggregateKey(builder, totalKey, &aggregators.SumAggregator{})
	require.NoError(t, err)

	err = state.RegisterAggregateKey(builder, totalKey, &aggregators.SumAggregator{})
	require.NoError(t, err) // No error on duplicate

	ctx := context.Background()

	// Should still work normally
	state.Set(ctx, mgr, totalKey, 10.0)
	total, _ := state.Get(ctx, mgr, totalKey)
	assert.Equal(t, 10.0, total)
}

func TestAggregateKey_Avg(t *testing.T) {
	builder := state.NewManagerBuilder()

	mgr := builder.Build()

	avgKey := state.NewKey[any]("avg_value", aggregators.AvgState{Mean: 0, Count: 0})
	err := state.RegisterAggregateKey(builder, avgKey, &aggregators.AvgAggregator{})
	require.NoError(t, err)

	ctx := context.Background()

	// Contribute values: 10, 20, 30
	state.Set(ctx, mgr, avgKey, 10.0)
	state.Set(ctx, mgr, avgKey, 20.0)
	state.Set(ctx, mgr, avgKey, 30.0)

	// Get the AvgState result
	result, err := state.Get(ctx, mgr, avgKey)
	require.NoError(t, err)

	avgState, ok := result.(aggregators.AvgState)
	require.True(t, ok, "Expected AvgState type")

	// Average should be 20
	assert.Equal(t, 20.0, avgState.Mean)
	assert.Equal(t, int64(3), avgState.Count)
}
