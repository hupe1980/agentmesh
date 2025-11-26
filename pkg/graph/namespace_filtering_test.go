package graph_test

import (
	"context"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNamespacedNodeReceivesFilteredView verifies that NamespacedCommandNode
// actually receives a filtered view that only contains keys from its namespace.
func TestNamespacedNodeReceivesFilteredView(t *testing.T) {
	ctx := context.Background()

	// Create namespaces
	agent1NS := state.MustNamespace("agent1")
	agent2NS := state.MustNamespace("agent2")

	// Create keys for different namespaces
	agent1Status := state.TypedKey[string](agent1NS, "status", "")
	agent2Status := state.TypedKey[string](agent2NS, "status", "")
	globalKey := state.NewKey[string]("global", "")

	// Setup manager with data in multiple namespaces
	mgr := state.NewManager()
	state.RegisterKey(mgr, agent1Status)
	state.RegisterKey(mgr, agent2Status)
	state.RegisterKey(mgr, globalKey)

	// Set state in different namespaces
	state.Set(ctx, mgr, agent1Status, "agent1_value")
	state.Set(ctx, mgr, agent2Status, "agent2_value")
	state.Set(ctx, mgr, globalKey, "global_value")

	// Create view
	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	// Verify full view sees all keys
	allKeys := view.Keys()
	assert.Len(t, allKeys, 3, "Full view should see all 3 keys")
	assert.Contains(t, allKeys, "agent1.status")
	assert.Contains(t, allKeys, "agent2.status")
	assert.Contains(t, allKeys, "global")

	// Track what keys the agent1 node saw
	var agent1ViewKeys []string

	// Create namespaced node for agent1
	agent1Node := graph.NewNamespacedNode(
		"agent1_node",
		agent1NS,
		func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// This view should only see agent1.* keys
			agent1ViewKeys = view.Keys()
			return []string{graph.EndNode}, nil, nil
		},
		[]string{graph.EndNode},
		false, // includeGlobal
	)

	// Execute the node
	_, _, err = agent1Node.Execute(ctx, view)
	require.NoError(t, err)

	// Verify agent1 node only saw its own namespace keys
	assert.Len(t, agent1ViewKeys, 1, "Agent1 node should only see 1 key from its namespace")
	assert.Contains(t, agent1ViewKeys, "agent1.status", "Should see agent1.status")
	assert.NotContains(t, agent1ViewKeys, "agent2.status", "Should NOT see agent2.status")
	assert.NotContains(t, agent1ViewKeys, "global", "Should NOT see global key")

	// Now test agent2 node
	var agent2ViewKeys []string

	agent2Node := graph.NewNamespacedNode(
		"agent2_node",
		agent2NS,
		func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			agent2ViewKeys = view.Keys()
			return []string{graph.EndNode}, nil, nil
		},
		[]string{graph.EndNode},
		false, // includeGlobal
	)

	_, _, err = agent2Node.Execute(ctx, view)
	require.NoError(t, err)

	// Verify agent2 node only saw its own namespace keys
	assert.Len(t, agent2ViewKeys, 1, "Agent2 node should only see 1 key from its namespace")
	assert.Contains(t, agent2ViewKeys, "agent2.status", "Should see agent2.status")
	assert.NotContains(t, agent2ViewKeys, "agent1.status", "Should NOT see agent1.status")
	assert.NotContains(t, agent2ViewKeys, "global", "Should NOT see global key")
}

// TestNamespacedNodeCannotAccessOtherNamespace verifies that namespaced nodes
// cannot read values from other namespaces.
func TestNamespacedNodeCannotAccessOtherNamespace(t *testing.T) {
	ctx := context.Background()

	agent1NS := state.MustNamespace("agent1")
	agent2NS := state.MustNamespace("agent2")

	agent1Key := state.TypedKey[string](agent1NS, "data", "")
	agent2Key := state.TypedKey[string](agent2NS, "data", "")

	mgr := state.NewManager()
	state.RegisterKey(mgr, agent1Key)
	state.RegisterKey(mgr, agent2Key)

	state.Set(ctx, mgr, agent1Key, "agent1_data")
	state.Set(ctx, mgr, agent2Key, "agent2_data")

	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	// Agent1 node tries to read its own key - should work
	agent1Node := graph.NewNamespacedNode(
		"agent1",
		agent1NS,
		func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			// Can read own namespace
			value := state.GetFromView(view, agent1Key)
			assert.Equal(t, "agent1_data", value)

			// Check if agent2 key exists - should return false
			exists := view.Has("agent2.data")
			assert.False(t, exists, "Agent1 node should not see agent2 keys")

			return []string{graph.EndNode}, nil, nil
		},
		[]string{graph.EndNode},
		false, // includeGlobal
	)

	_, _, err = agent1Node.Execute(ctx, view)
	require.NoError(t, err)
}

// TestRegularNodeSeesAllNamespaces verifies that regular (non-namespaced) nodes
// still see all keys from all namespaces.
func TestRegularNodeSeesAllNamespaces(t *testing.T) {
	ctx := context.Background()

	agent1NS := state.MustNamespace("agent1")
	agent2NS := state.MustNamespace("agent2")

	agent1Key := state.TypedKey[string](agent1NS, "data", "")
	agent2Key := state.TypedKey[string](agent2NS, "data", "")

	mgr := state.NewManager()
	state.RegisterKey(mgr, agent1Key)
	state.RegisterKey(mgr, agent2Key)

	state.Set(ctx, mgr, agent1Key, "agent1_data")
	state.Set(ctx, mgr, agent2Key, "agent2_data")

	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	// Regular node (BaseNode) should see all keys
	var seenKeys []string
	regularNode := &graph.BaseNode{
		NodeName: "regular",
		Fn: func(ctx context.Context, view state.ReadView) ([]string, state.Updates, error) {
			seenKeys = view.Keys()
			return []string{graph.EndNode}, nil, nil
		},
		DeclaredTargets: []string{graph.EndNode},
	}

	_, _, err = regularNode.Execute(ctx, view)
	require.NoError(t, err)

	// Regular node should see all namespaced keys
	assert.Len(t, seenKeys, 2)
	assert.Contains(t, seenKeys, "agent1.data")
	assert.Contains(t, seenKeys, "agent2.data")
}
