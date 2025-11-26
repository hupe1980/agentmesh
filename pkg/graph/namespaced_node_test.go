package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamespacedCommandNode(t *testing.T) {
	t.Run("Creates node with namespace", func(t *testing.T) {
		agentNS := state.MustNamespace("agent1")
		targets := []string{graph.EndNode}

		node := graph.NewNamespacedNode(
			"test_node",
			agentNS,
			func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
			targets,
			false, // includeGlobal
		)

		assert.Equal(t, "test_node", node.Name())
		assert.Equal(t, agentNS, node.Namespace())
		assert.Equal(t, []string{graph.EndNode}, node.Targets())
	})

	t.Run("Implements NamespacedNode interface", func(t *testing.T) {
		agentNS := state.MustNamespace("agent1")
		targets := []string{graph.EndNode}

		node := graph.NewNamespacedNode(
			"test_node",
			agentNS,
			nil,
			targets,
			false, // includeGlobal
		)

		// Verify it implements NamespacedNode
		var _ *graph.NamespacedNode = node

		// Verify it also implements Node
		var _ graph.Node = node
	})

	t.Run("WithRetry creates node with retry policy", func(t *testing.T) {
		agentNS := state.MustNamespace("agent1")
		targets := []string{graph.EndNode}
		retry := graph.NewRetryPolicy().WithMaxAttempts(3).Build()

		node := graph.NewNamespacedNodeWithRetry(
			"test_node",
			agentNS,
			func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				return []string{graph.EndNode}, nil, nil
			},
			targets,
			retry,
			false, // includeGlobal
		)

		assert.Equal(t, retry, node.RetryPolicy())
	})
}

func TestNamespacedNodeIsolation(t *testing.T) {
	ctx := context.Background()
	manager := state.NewManager()

	// Create two namespaces
	agent1NS := state.MustNamespace("agent1")
	agent2NS := state.MustNamespace("agent2")

	// Create namespaced keys
	agent1StatusKey := state.TypedKey[string](agent1NS, "status", "idle")
	agent2StatusKey := state.TypedKey[string](agent2NS, "status", "idle")

	// Register keys
	require.NoError(t, state.RegisterKey(manager, agent1StatusKey))
	require.NoError(t, state.RegisterKey(manager, agent2StatusKey))

	// Set values
	require.NoError(t, state.Set(ctx, manager, agent1StatusKey, "agent1_active"))
	require.NoError(t, state.Set(ctx, manager, agent2StatusKey, "agent2_active"))

	// Verify key isolation through key names
	t.Run("Each namespace creates isolated keys", func(t *testing.T) {
		// Keys have namespace prefixes
		assert.Equal(t, "agent1.status", agent1StatusKey.Name())
		assert.Equal(t, "agent2.status", agent2StatusKey.Name())

		// Verify values can be retrieved independently
		agent1Value, err := state.Get(ctx, manager, agent1StatusKey)
		require.NoError(t, err)
		agent2Value, err := state.Get(ctx, manager, agent2StatusKey)
		require.NoError(t, err)

		assert.Equal(t, "agent1_active", agent1Value)
		assert.Equal(t, "agent2_active", agent2Value)
	})

	t.Run("Namespaced nodes in graph execution", func(t *testing.T) {
		// Create nodes for different agents
		targets := []string{graph.EndNode}

		agent1Node := graph.NewNamespacedNode(
			"agent1_process",
			agent1NS,
			func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				status := state.GetFromView(view, agent1StatusKey)
				updates := state.NoUpdate()
				updates[agent1StatusKey.Name()] = status + "_processed"
				return []string{graph.EndNode}, nil, nil
			},
			targets,
			false, // includeGlobal
		)

		agent2Node := graph.NewNamespacedNode(
			"agent2_process",
			agent2NS,
			func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
				status := state.GetFromView(view, agent2StatusKey)
				updates := state.NoUpdate()
				updates[agent2StatusKey.Name()] = status + "_processed"
				return []string{graph.EndNode}, nil, nil
			},
			targets,
			false, // includeGlobal
		)

		// Verify namespace isolation
		assert.Equal(t, agent1NS, agent1Node.Namespace())
		assert.Equal(t, agent2NS, agent2Node.Namespace())
		assert.NotEqual(t, agent1Node.Namespace(), agent2Node.Namespace())
	})
}

func TestNamespacedReadView(t *testing.T) {
	// This test validates the NamespacedReadView API structure
	// Note: Full integration testing happens in executor tests

	t.Run("NamespacedReadView validates namespace access", func(t *testing.T) {
		agentNS := state.MustNamespace("agent1")
		correctKey := state.TypedKey[string](agentNS, "status", "idle")
		wrongNS := state.MustNamespace("agent2")
		wrongKey := state.TypedKey[string](wrongNS, "status", "idle")

		// Verify key names
		assert.Equal(t, "agent1.status", correctKey.Name())
		assert.Equal(t, "agent2.status", wrongKey.Name())

		// NamespacedReadView API validates namespace membership
		// (Testing the panic behavior requires actual view - tested in integration tests)
	})

	t.Run("Namespace filtering logic", func(t *testing.T) {
		agentNS := state.MustNamespace("agent1")

		// Test namespace prefix logic
		assert.Equal(t, "agent1", agentNS.Name())
		assert.False(t, agentNS.IsGlobal())

		// Global namespace
		assert.True(t, state.Global.IsGlobal())
		assert.Equal(t, "", state.Global.Name())
	})
}

func TestGlobalNamespaceKeys(t *testing.T) {
	t.Run("Global vs namespaced key naming", func(t *testing.T) {
		// Global keys have no prefix
		globalKey1 := state.NewKey[string]("key1", "")
		globalKey2 := state.NewKey[string]("key2", "")

		assert.Equal(t, "key1", globalKey1.Name())
		assert.Equal(t, "key2", globalKey2.Name())

		// Namespaced keys have prefix
		agentNS := state.MustNamespace("agent1")
		namespacedKey := state.TypedKey[string](agentNS, "status", "")

		assert.Equal(t, "agent1.status", namespacedKey.Name())

		// Keys are distinguishable by name
		assert.NotEqual(t, globalKey1.Name(), namespacedKey.Name())
	})
}
