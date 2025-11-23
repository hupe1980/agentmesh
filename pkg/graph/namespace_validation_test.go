package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNamespacedNodeValidatesUpdates verifies that NamespacedCommandNode
// validates that returned updates only contain keys from its namespace.
func TestNamespacedNodeValidatesUpdates(t *testing.T) {
	ctx := context.Background()

	agent1NS := state.MustNamespace("agent1")
	agent2NS := state.MustNamespace("agent2")

	agent1Key := state.TypedKey[string](agent1NS, "data", "")
	agent2Key := state.TypedKey[string](agent2NS, "data", "")
	globalKey := state.NewKey[string]("global", "")

	mgr := state.NewManager()
	state.RegisterKey(mgr, agent1Key)
	state.RegisterKey(mgr, agent2Key)
	state.RegisterKey(mgr, globalKey)

	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	t.Run("allows updates to own namespace", func(t *testing.T) {
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agent1NS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				// Update own namespace - should be allowed
				updates := state.Updates{
					"agent1.data": "value",
				}
				return graph.End(updates), nil
			},
			graph.NewTargetSet(graph.EndNode),
			false, // Don't include global
		)

		cmd, err := node.Execute(ctx, view)
		require.NoError(t, err)
		assert.NotNil(t, cmd)
	})

	t.Run("rejects updates to other namespace", func(t *testing.T) {
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agent1NS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				// Try to update different namespace - should fail
				updates := state.Updates{
					"agent2.data": "value",
				}
				return graph.End(updates), nil
			},
			graph.NewTargetSet(graph.EndNode),
			false, // Don't include global
		)

		_, err := node.Execute(ctx, view)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "attempted to update key")
		assert.Contains(t, err.Error(), "agent2.data")
		assert.Contains(t, err.Error(), "different namespace")
	})

	t.Run("rejects global updates when includeGlobal=false", func(t *testing.T) {
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agent1NS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				// Try to update global key - should fail
				updates := state.Updates{
					"global": "value",
				}
				return graph.End(updates), nil
			},
			graph.NewTargetSet(graph.EndNode),
			false, // Don't include global
		)

		_, err := node.Execute(ctx, view)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "attempted to update key")
		assert.Contains(t, err.Error(), "global")
	})

	t.Run("allows global updates when includeGlobal=true", func(t *testing.T) {
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agent1NS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				// Update both namespace and global - should be allowed
				updates := state.Updates{
					"agent1.data": "value",
					"global":      "global_value",
				}
				return graph.End(updates), nil
			},
			graph.NewTargetSet(graph.EndNode),
			true, // Include global
		)

		cmd, err := node.Execute(ctx, view)
		require.NoError(t, err)
		assert.NotNil(t, cmd)
	})

	t.Run("rejects other namespace even with includeGlobal=true", func(t *testing.T) {
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agent1NS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				// Try to update different namespace - should still fail
				updates := state.Updates{
					"agent2.data": "value",
				}
				return graph.End(updates), nil
			},
			graph.NewTargetSet(graph.EndNode),
			true, // Include global
		)

		_, err := node.Execute(ctx, view)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent2.data")
	})

	t.Run("allows mixed updates with includeGlobal=true", func(t *testing.T) {
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agent1NS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				updates := state.Updates{
					"agent1.data":   "namespace_value",
					"agent1.status": "active",
					"global":        "global_value",
					"config":        "config_value",
				}
				return graph.End(updates), nil
			},
			graph.NewTargetSet(graph.EndNode),
			true, // Include global
		)

		cmd, err := node.Execute(ctx, view)
		require.NoError(t, err)
		assert.NotNil(t, cmd)
		assert.Len(t, cmd.Updates, 4)
	})
}

// TestNamespacedNodeIncludeGlobalView verifies that includeGlobal
// controls whether global keys are visible in the view.
func TestNamespacedNodeIncludeGlobalView(t *testing.T) {
	ctx := context.Background()

	agentNS := state.MustNamespace("agent1")

	agentKey := state.TypedKey[string](agentNS, "data", "")
	globalKey := state.NewKey[string]("global", "")

	mgr := state.NewManager()
	state.RegisterKey(mgr, agentKey)
	state.RegisterKey(mgr, globalKey)

	state.Set(ctx, mgr, agentKey, "agent_value")
	state.Set(ctx, mgr, globalKey, "global_value")

	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	t.Run("includeGlobal=false hides global keys", func(t *testing.T) {
		var seenKeys []string
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agentNS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				seenKeys = view.Keys()

				// Check global key is not visible
				hasGlobal := view.Has("global")
				assert.False(t, hasGlobal, "Global key should not be visible")

				return graph.End(), nil
			},
			graph.NewTargetSet(graph.EndNode),
			false, // Don't include global
		)

		_, err := node.Execute(ctx, view)
		require.NoError(t, err)

		assert.Len(t, seenKeys, 1)
		assert.Contains(t, seenKeys, "agent1.data")
		assert.NotContains(t, seenKeys, "global")
	})

	t.Run("includeGlobal=true exposes global keys", func(t *testing.T) {
		var seenKeys []string
		node := graph.NewNamespacedCommandNode(
			"agent1",
			agentNS,
			func(ctx context.Context, view state.ReadView) (*graph.Command, error) {
				seenKeys = view.Keys()

				// Check global key is visible
				hasGlobal := view.Has("global")
				assert.True(t, hasGlobal, "Global key should be visible")

				// Can read global key
				globalValue := state.GetFromView(view, globalKey)
				assert.Equal(t, "global_value", globalValue)

				return graph.End(), nil
			},
			graph.NewTargetSet(graph.EndNode),
			true, // Include global
		)

		_, err := node.Execute(ctx, view)
		require.NoError(t, err)

		assert.Len(t, seenKeys, 2)
		assert.Contains(t, seenKeys, "agent1.data")
		assert.Contains(t, seenKeys, "global")
	})
}
