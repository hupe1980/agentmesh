// Package integration_test contains integration tests for namespace isolation.
package integration_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNamespaceBasicIsolation tests that namespace prefixes provide proper key isolation.
func TestNamespaceBasicIsolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create keys with namespace prefixes (convention: ns.keyname)
	agent1Counter := graph.NewKey("agent1.counter", 0)
	agent2Counter := graph.NewKey("agent2.counter", 0)
	resultKey := graph.NewKey("result", "")

	var finalResult string

	g := graph.New[any, any](agent1Counter, agent2Counter, resultKey)

	// Agent1 updates its namespaced counter
	g.Node("agent1", func(_ context.Context, view graph.View) (*graph.Command, error) {
		return graph.Set(agent1Counter, 10).To("agent2")
	}, "agent2")

	// Agent2 updates its namespaced counter
	g.Node("agent2", func(_ context.Context, view graph.View) (*graph.Command, error) {
		return graph.Set(agent2Counter, 20).To("combine")
	}, "combine")

	// Combine reads both namespaces
	g.Node("combine", func(_ context.Context, view graph.View) (*graph.Command, error) {
		a1 := graph.Get(view, agent1Counter)
		a2 := graph.Get(view, agent2Counter)
		finalResult = fmt.Sprintf("agent1=%d, agent2=%d", a1, a2)
		return graph.Set(resultKey, finalResult).To(graph.END)
	}, graph.END)

	g.Start("agent1")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	assert.Equal(t, "agent1=10, agent2=20", finalResult)
}

// TestNamespaceViolation tests that namespace violations are detected with WithNamespace.
func TestNamespaceViolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	agent1Key := graph.NewKey("agent1.data", "")
	agent2Key := graph.NewKey("agent2.data", "")

	g := graph.New[any, any](agent1Key, agent2Key)

	// Create namespace for agent1
	ns := graph.NewNamespace("agent1")

	// Agent1 tries to update agent2's namespace (should fail)
	g.Node("bad_agent", graph.WithNamespace(func(_ context.Context, _ graph.View) (*graph.Command, error) {
		// This should cause a namespace violation
		return graph.Set(agent2Key, "hacked!").To(graph.END)
	}, ns, false), graph.END)

	g.Start("bad_agent")

	compiled, err := g.Build()
	require.NoError(t, err)

	var foundError error
	for _, err := range compiled.Run(ctx, nil) {
		if err != nil {
			foundError = err
		}
	}

	require.Error(t, foundError)
	assert.ErrorIs(t, foundError, graph.ErrNamespaceViolation)
}

// TestNamespaceWithGlobalAccess tests that global keys can be accessed when allowed.
func TestNamespaceWithGlobalAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Global key (no dots = not namespaced)
	globalConfig := graph.NewKey("config", "default")
	// Namespaced key
	agent1Data := graph.NewKey("agent1.data", "")

	var finalData string

	g := graph.New[any, any](globalConfig, agent1Data)

	ns := graph.NewNamespace("agent1")

	// Agent1 can read global and update its namespace
	g.Node("agent1", graph.WithNamespace(func(_ context.Context, view graph.View) (*graph.Command, error) {
		// Should be able to read global key
		cfg := graph.Get(view, globalConfig)
		// Can update its own namespace
		finalData = "processed-" + cfg
		return graph.Set(agent1Data, finalData).To(graph.END)
	}, ns, true), graph.END) // includeGlobal = true

	g.Start("agent1")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	assert.Equal(t, "processed-default", finalData)
}

// TestNamespaceCannotReadOtherNamespace tests that a namespaced view cannot read other namespaces.
func TestNamespaceCannotReadOtherNamespace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Use empty string as default so we can detect when the value is NOT available
	agent1Key := graph.NewKey("agent1.secret", "")
	agent2Key := graph.NewKey("agent2.secret", "")
	// Use a namespaced result key for agent1
	resultKey := graph.NewKey("agent1.result", "")

	var finalResult string

	g := graph.New[any, any](agent1Key, agent2Key, resultKey)

	// First node sets initial values
	g.Node("setup", func(_ context.Context, view graph.View) (*graph.Command, error) {
		return graph.Set(agent1Key, "agent1-secret").
			With(graph.SetValue(agent2Key, "agent2-secret")).
			To("reader")
	}, "reader")

	// Create namespace for agent1
	ns := graph.NewNamespace("agent1")

	// Agent1 tries to read agent2's data (should fail silently - returns zero value)
	g.Node("reader", graph.WithNamespace(func(_ context.Context, view graph.View) (*graph.Command, error) {
		// Can read own namespace
		own := graph.Get(view, agent1Key)
		// Cannot read other namespace (returns empty string = zero value)
		other := graph.Get(view, agent2Key)

		finalResult = fmt.Sprintf("own=%s,other=%s", own, other)
		return graph.Set(resultKey, finalResult).To(graph.END)
	}, ns, false), graph.END)

	g.Start("setup")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	// Agent1 can read its own secret, but agent2's appears empty (default value)
	assert.Equal(t, "own=agent1-secret,other=", finalResult)
}

// TestNamespacePrefix tests the Namespace.Prefix() utility function.
func TestNamespacePrefix(t *testing.T) {
	t.Parallel()

	ns := graph.NewNamespace("mymodule")

	assert.Equal(t, "mymodule", ns.Name())
	assert.Equal(t, "mymodule.counter", ns.Prefix("counter"))
	assert.Equal(t, "mymodule.sub.key", ns.Prefix("sub.key"))
}

// TestNamespaceGlobalUpdateAllowed tests that global keys can be updated when includeGlobal is true.
func TestNamespaceGlobalUpdateAllowed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	globalKey := graph.NewKey("shared", "initial")
	agent1Key := graph.NewKey("agent1.private", "")

	var globalVal, localVal string

	g := graph.New[any, any](globalKey, agent1Key)

	ns := graph.NewNamespace("agent1")

	// Agent1 updates both global and its own namespace
	g.Node("agent1", graph.WithNamespace(func(_ context.Context, view graph.View) (*graph.Command, error) {
		globalVal = "updated-global"
		localVal = "local-data"
		return graph.Set(globalKey, globalVal).
			With(graph.SetValue(agent1Key, localVal)).
			To(graph.END)
	}, ns, true), graph.END) // includeGlobal = true

	g.Start("agent1")

	compiled, err := g.Build()
	require.NoError(t, err)

	for _, err := range compiled.Run(ctx, nil) {
		require.NoError(t, err)
	}

	assert.Equal(t, "updated-global", globalVal)
	assert.Equal(t, "local-data", localVal)
}
