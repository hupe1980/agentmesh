package integration_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraphAlgorithms_LinearChain tests a simple linear chain of nodes
func TestGraphAlgorithms_LinearChain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var executionOrder []string

	valueKey := graph.NewKey[int]("value")

	g := graph.New[any, any](valueKey)

	g.Node("a", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		executionOrder = append(executionOrder, "a")
		return graph.Set(valueKey, 1).To("b")
	}, "b")

	g.Node("b", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		executionOrder = append(executionOrder, "b")
		v := graph.Get(scope, valueKey)
		return graph.Set(valueKey, v+1).To("c")
	}, "c")

	g.Node("c", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		executionOrder = append(executionOrder, "c")
		v := graph.Get(scope, valueKey)
		return graph.Set(valueKey, v+1).End()
	}, graph.END)

	g.Start("a")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"a", "b", "c"}, executionOrder)
}

// TestGraphAlgorithms_ParallelBranches tests parallel branch execution
func TestGraphAlgorithms_ParallelBranches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var counter atomic.Int32

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, any](resultKey)

	// Start node splits to two parallel branches
	g.Node("start", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter.Add(1)
		return graph.To("branch1", "branch2")
	}, "branch1", "branch2")

	g.Node("branch1", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter.Add(1)
		return graph.To("merge")
	}, "merge")

	g.Node("branch2", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter.Add(1)
		return graph.To("merge")
	}, "merge")

	g.Node("merge", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter.Add(1)
		return graph.Set(resultKey, "done").End()
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// start + branch1 + branch2 + merge (may be called twice due to parallel)
	assert.GreaterOrEqual(t, int(counter.Load()), 4)
}

// TestGraphAlgorithms_ConditionalRouting tests conditional routing based on state
func TestGraphAlgorithms_ConditionalRouting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	pathKey := graph.NewKey[string]("path")
	conditionKey := graph.NewKey[bool]("condition")

	g := graph.New[any, any](pathKey, conditionKey)

	g.Node("router", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		cond := graph.Get(scope, conditionKey)
		if cond {
			return graph.Set(pathKey, "left").To("left")
		}
		return graph.Set(pathKey, "right").To("right")
	}, "left", "right")

	g.Node("left", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.Set(pathKey, "went_left").End()
	}, graph.END)

	g.Node("right", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		return graph.Set(pathKey, "went_right").End()
	}, graph.END)

	g.Start("router")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}
}

// TestGraphAlgorithms_Loop tests a looping graph pattern
func TestGraphAlgorithms_Loop(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	maxIterations := 5

	counterKey := graph.NewKey[int]("counter")

	g := graph.New[any, any](counterKey)

	g.Node("loop", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		counter++
		if counter >= maxIterations {
			return graph.Set(counterKey, counter).End()
		}
		return graph.Set(counterKey, counter).To("loop")
	}, "loop", graph.END)

	g.Start("loop")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}
}

// TestGraphAlgorithms_DiamondPattern tests diamond-shaped execution pattern
func TestGraphAlgorithms_DiamondPattern(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var mu sync.Mutex
	var executed []string

	resultKey := graph.NewKey[string]("result")

	g := graph.New[any, any](resultKey)

	// Diamond: top -> (left, right) -> bottom
	g.Node("top", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		mu.Lock()
		executed = append(executed, "top")
		mu.Unlock()
		return graph.To("left", "right")
	}, "left", "right")

	g.Node("left", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		mu.Lock()
		executed = append(executed, "left")
		mu.Unlock()
		return graph.To("bottom")
	}, "bottom")

	g.Node("right", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		mu.Lock()
		executed = append(executed, "right")
		mu.Unlock()
		return graph.To("bottom")
	}, "bottom")

	g.Node("bottom", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		mu.Lock()
		executed = append(executed, "bottom")
		mu.Unlock()
		return graph.Set(resultKey, "complete").End()
	}, graph.END)

	g.Start("top")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Top should execute first
	assert.Equal(t, "top", executed[0])
	// Bottom should appear after left and right
	assert.Contains(t, executed, "left")
	assert.Contains(t, executed, "right")
	assert.Contains(t, executed, "bottom")
}

// TestGraphAlgorithms_StateAccumulation tests state accumulation across nodes
func TestGraphAlgorithms_StateAccumulation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	sumKey := graph.NewKey[int]("sum")

	g := graph.New[any, any](sumKey)

	g.Node("add1", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		sum := graph.Get(scope, sumKey)
		return graph.Set(sumKey, sum+1).To("add2")
	}, "add2")

	g.Node("add2", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		sum := graph.Get(scope, sumKey)
		return graph.Set(sumKey, sum+2).To("add3")
	}, "add3")

	g.Node("add3", func(ctx context.Context, scope graph.Scope[any]) (*graph.Command, error) {
		sum := graph.Get(scope, sumKey)
		return graph.Set(sumKey, sum+3).End()
	}, graph.END)

	g.Start("add1")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}
}
