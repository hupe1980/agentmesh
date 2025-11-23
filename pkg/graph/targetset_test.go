package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/message"
	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = errors.New // prevent unused import error

func TestTargetSet(t *testing.T) {
	t.Run("NewTargetSet creates valid set", func(t *testing.T) {
		targets := NewTargetSet("node_a", "node_b", EndNode)

		assert.NotNil(t, targets)
		assert.Len(t, targets.All(), 3)
		assert.Contains(t, targets.All(), "node_a")
		assert.Contains(t, targets.All(), "node_b")
		assert.Contains(t, targets.All(), EndNode)
	})

	t.Run("Get returns target if exists", func(t *testing.T) {
		targets := NewTargetSet("node_a", "node_b")

		assert.Equal(t, "node_a", targets.Get("node_a"))
		assert.Equal(t, "node_b", targets.Get("node_b"))
		assert.Equal(t, "", targets.Get("nonexistent"))
	})

	t.Run("Has checks target existence", func(t *testing.T) {
		targets := NewTargetSet("node_a", "node_b")

		assert.True(t, targets.Has("node_a"))
		assert.True(t, targets.Has("node_b"))
		assert.False(t, targets.Has("nonexistent"))
	})

	t.Run("Goto creates command with targets", func(t *testing.T) {
		targets := NewTargetSet("node_a", "node_b")
		updates := state.Updates{"key": "value"}

		cmd := targets.Goto(targets.Get("node_a"), updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{"node_a"}, cmd.Goto)
	})

	t.Run("GotoOne creates single-target command", func(t *testing.T) {
		targets := NewTargetSet("node_a")
		updates := state.Updates{"key": "value"}

		cmd := targets.GotoOne(targets.Get("node_a"), updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{"node_a"}, cmd.Goto)
	})

	t.Run("Goto with EndNode for termination", func(t *testing.T) {
		targets := NewTargetSet("node_a", EndNode)
		updates := state.Updates{"key": "value"}

		// Explicit routing to EndNode using Goto
		cmd := targets.Goto(targets.Get(EndNode), updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{EndNode}, cmd.Goto)
	})

	t.Run("Goto with target and updates", func(t *testing.T) {
		targets := NewTargetSet("node_a", EndNode)
		updates := state.Updates{"key": "value"}

		cmd := targets.Goto(targets.Get("node_a"), updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{"node_a"}, cmd.Goto)
	})

	t.Run("Goto with EndNode", func(t *testing.T) {
		targets := NewTargetSet("node_a", EndNode)
		updates := state.Updates{"key": "value"}

		cmd := targets.Goto(targets.Get(EndNode), updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{EndNode}, cmd.Goto)
	})
}

func TestAddNodeWithTargetSet(t *testing.T) {
	t.Run("adds node with type-safe targets", func(t *testing.T) {
		builder, err := NewBuilder(NewMessagePregelExecutor())
		require.NoError(t, err)

		// Register messages key
		messagesKey := state.NewListKey[message.Message]("__messages__", 0)
		require.NoError(t, state.RegisterListKey(builder.Manager(), messagesKey))

		targets := NewTargetSet("node_b", EndNode)

		builder.AddCommandNode("node_a", targets,
			func(ctx context.Context, view *state.ReadView) (*Command, error) {
				return targets.Goto(targets.Get("node_b"), state.Updates{}), nil
			},
		)

		builder.AddCommandNode("node_b", NewTargetSet(EndNode),
			func(ctx context.Context, view *state.ReadView) (*Command, error) {
				return End(state.Updates{}), nil
			},
		)

		compiled, err := builder.
			SetEntryPoint("node_a").
			Compile()
		require.NoError(t, err)
		assert.NotNil(t, compiled)

		// Verify node was added with correct targets
		node := compiled.graph.Nodes["node_a"]
		require.NotNil(t, node)
		assert.Equal(t, "node_a", node.Name())
		assert.ElementsMatch(t, []string{"node_b", EndNode}, node.Targets())
	})

	t.Run("type-safe routing with retry policy", func(t *testing.T) {
		builder, err := NewBuilder(NewMessagePregelExecutor())
		require.NoError(t, err)

		// Register messages key
		messagesKey := state.NewListKey[message.Message]("__messages__", 0)
		require.NoError(t, state.RegisterListKey(builder.Manager(), messagesKey))

		targets := NewTargetSet("success", EndNode)

		// Just verify the node is added with retry policy
		builder.AddCommandNodeWithRetry("node_a", targets,
			func(ctx context.Context, view *state.ReadView) (*Command, error) {
				return targets.Goto(targets.Get("success"), state.Updates{}), nil
			},
			NewRetryPolicy().WithMaxAttempts(3).Build(),
		)

		builder.AddCommandNode("success", NewTargetSet(EndNode),
			func(ctx context.Context, view *state.ReadView) (*Command, error) {
				return End(state.Updates{}), nil
			},
		)

		compiled, err := builder.
			SetEntryPoint("node_a").
			Compile()
		require.NoError(t, err)

		// Verify the node has retry policy
		node := compiled.graph.Nodes["node_a"]
		require.NotNil(t, node)

		// Check if node implements NodeWithRetry
		if retryNode, ok := node.(NodeWithRetry); ok {
			policy := retryNode.RetryPolicy()
			assert.NotNil(t, policy)
		}
	})
}

func TestTargetSetGoto(t *testing.T) {
	t.Run("routes to target with updates", func(t *testing.T) {
		targets := NewTargetSet("next", EndNode)
		updates := state.Updates{"key": "value"}

		cmd := targets.Goto(targets.Get("next"), updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{"next"}, cmd.Goto)
	})

	t.Run("routes to EndNode with updates", func(t *testing.T) {
		targets := NewTargetSet("next", EndNode)
		updates := state.Updates{"key": "value"}

		cmd := targets.Goto(targets.Get(EndNode), updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{EndNode}, cmd.Goto)
	})

	t.Run("routes without updates", func(t *testing.T) {
		targets := NewTargetSet("a", "b", "c", EndNode)

		cmd := targets.Goto(targets.Get("a"))
		assert.Equal(t, []string{"a"}, cmd.Goto)
		assert.Nil(t, cmd.Updates)
	})
}

func TestTargetSetHelpers(t *testing.T) {
	t.Run("MustGet panics on missing target", func(t *testing.T) {
		targets := NewTargetSet("node_a")

		// This pattern demonstrates type-safe usage:
		// If Get returns empty string, routing will fail at runtime
		target := targets.Get("nonexistent")
		assert.Equal(t, "", target)

		// Users can create their own MustGet helper if desired:
		mustGet := func(ts *TargetSet, name string) string {
			if !ts.Has(name) {
				panic("target not found: " + name)
			}
			return ts.Get(name)
		}

		assert.Panics(t, func() {
			mustGet(targets, "nonexistent")
		})

		assert.NotPanics(t, func() {
			result := mustGet(targets, "node_a")
			assert.Equal(t, "node_a", result)
		})
	})

	t.Run("GotoAll routes to all targets in parallel", func(t *testing.T) {
		targets := NewTargetSet("a", "b", "c")
		updates := state.Updates{"key": "value"}

		cmd := targets.GotoAll(updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{"a", "b", "c"}, cmd.Goto)
	})

	t.Run("GotoFirst routes to first target", func(t *testing.T) {
		targets := NewTargetSet("primary", "secondary", "fallback")
		updates := state.Updates{"key": "value"}

		cmd := targets.GotoFirst(updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{"primary"}, cmd.Goto)
	})

	t.Run("GotoLast routes to last target", func(t *testing.T) {
		targets := NewTargetSet("step1", "step2", "aggregate")
		updates := state.Updates{"key": "value"}

		cmd := targets.GotoLast(updates)

		assert.NotNil(t, cmd)
		assert.Equal(t, updates, cmd.Updates)
		assert.Equal(t, []string{"aggregate"}, cmd.Goto)
	})

	t.Run("GotoFirst panics on empty TargetSet", func(t *testing.T) {
		targets := &TargetSet{
			targets: make(map[string]string),
			all:     []string{},
		}

		assert.Panics(t, func() {
			targets.GotoFirst(state.Updates{})
		})
	})

	t.Run("GotoLast panics on empty TargetSet", func(t *testing.T) {
		targets := &TargetSet{
			targets: make(map[string]string),
			all:     []string{},
		}

		assert.Panics(t, func() {
			targets.GotoLast(state.Updates{})
		})
	})

	t.Run("targets maintain declaration order", func(t *testing.T) {
		targets := NewTargetSet("z", "a", "m", "b")

		// Order should be preserved as declared, not alphabetically sorted
		assert.Equal(t, []string{"z", "a", "m", "b"}, targets.All())

		// First and last should respect declaration order
		firstCmd := targets.GotoFirst(state.Updates{})
		assert.Equal(t, []string{"z"}, firstCmd.Goto)

		lastCmd := targets.GotoLast(state.Updates{})
		assert.Equal(t, []string{"b"}, lastCmd.Goto)
	})
}
