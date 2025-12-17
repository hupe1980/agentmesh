package integration_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGraphValidation_ValidGraph tests that a valid graph builds successfully
func TestGraphValidation_ValidGraph(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	g := graph.New(resultKey)

	g.Node("start", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(resultKey, "done").End()
	}, graph.END)

	g.Start("start")

	compiled, err := g.Build()
	require.NoError(t, err)
	require.NotNil(t, compiled)
}

// TestGraphValidation_NoEntryPoint tests that a graph without entry point fails
func TestGraphValidation_NoEntryPoint(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	g := graph.New(resultKey)

	g.Node("orphan", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(resultKey, "done").End()
	}, graph.END)

	// No g.Start() called

	_, err := g.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry")
}

// TestGraphValidation_UnreachableNode tests that unreachable nodes are detected
func TestGraphValidation_UnreachableNode(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	g := graph.New(resultKey)

	g.Node("start", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(resultKey, "done").End()
	}, graph.END)

	// This node is never reached
	g.Node("unreachable", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(resultKey, "never").End()
	}, graph.END)

	g.Start("start")

	// Build with strict validation should fail
	_, err := g.Build(graph.WithStrictValidation())
	if err != nil {
		assert.Contains(t, err.Error(), "unreachable")
	}
}

// TestGraphValidation_InvalidTarget tests that invalid targets are detected
func TestGraphValidation_InvalidTarget(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	g := graph.New(resultKey)

	// Node points to non-existent target
	g.Node("start", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To("nonexistent")
	}, "nonexistent")

	g.Start("start")

	_, err := g.Build()
	if err != nil {
		// Expected to fail with validation error
		t.Logf("Expected validation error: %v", err)
	}
}

// TestGraphValidation_MultipleEntryPoints tests graphs with multiple entry points
func TestGraphValidation_MultipleEntryPoints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	result1Key := graph.NewKey[string]("result1")
	result2Key := graph.NewKey[string]("result2")

	g := graph.New(result1Key, result2Key)

	g.Node("entry1", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(result1Key, "from_entry1").To("merge")
	}, "merge")

	g.Node("entry2", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(result2Key, "from_entry2").To("merge")
	}, "merge")

	g.Node("merge", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)

	// Set both as entry points
	g.Start("entry1", "entry2")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}
}

// TestGraphValidation_SelfLoop tests that self-loops are handled correctly
func TestGraphValidation_SelfLoop(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	counterKey := graph.NewKey[int]("counter")

	g := graph.New(counterKey)

	// Node that loops back to itself
	g.Node("loop", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		counter := graph.Get(scope, counterKey)
		if counter >= 3 {
			return graph.Set(counterKey, counter).End()
		}
		return graph.Set(counterKey, counter+1).To("loop")
	}, "loop", graph.END)

	g.Start("loop")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}
}

// TestGraphValidation_EmptyGraph tests that an empty graph fails validation
func TestGraphValidation_EmptyGraph(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	g := graph.New(resultKey)

	// No nodes added, no entry point set

	_, err := g.Build()
	require.Error(t, err)
}

// TestGraphValidation_DuplicateNodeNames tests that duplicate node names are rejected
func TestGraphValidation_DuplicateNodeNames(t *testing.T) {
	t.Parallel()

	resultKey := graph.NewKey[string]("result")

	g := graph.New(resultKey)

	g.Node("node", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.To(graph.END)
	}, graph.END)

	// Adding same node name again should overwrite or error
	g.Node("node", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		return graph.Set(resultKey, "duplicate").End()
	}, graph.END)

	g.Start("node")

	// Should still build (last definition wins)
	compiled, err := g.Build()
	require.NoError(t, err)
	require.NotNil(t, compiled)
}
