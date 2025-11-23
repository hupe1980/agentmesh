package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNamespace(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid namespace",
			input:   "model",
			wantErr: false,
		},
		{
			name:    "valid with underscore",
			input:   "agent_1",
			wantErr: false,
		},
		{
			name:      "empty namespace",
			input:     "",
			wantErr:   true,
			errSubstr: "cannot be empty",
		},
		{
			name:      "namespace with dot",
			input:     "model.sub",
			wantErr:   true,
			errSubstr: "cannot contain dots",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, err := NewNamespace(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.input, ns.Name())
			}
		})
	}
}

func TestMustNamespace(t *testing.T) {
	t.Run("valid namespace", func(t *testing.T) {
		ns := MustNamespace("model")
		assert.Equal(t, "model", ns.Name())
	})

	t.Run("invalid namespace panics", func(t *testing.T) {
		assert.Panics(t, func() {
			MustNamespace("model.sub")
		})
	})
}

func TestGlobalNamespace(t *testing.T) {
	assert.True(t, Global.IsGlobal())
	assert.Equal(t, "", Global.Name())
}

func TestNamespaceTypedKey(t *testing.T) {
	modelNS := MustNamespace("model")

	t.Run("creates key with namespace prefix", func(t *testing.T) {
		key := TypedKey[int](modelNS, "counter", 0)
		assert.Equal(t, "model.counter", key.Name())
		assert.Equal(t, 0, key.Zero())
	})

	t.Run("global namespace has no prefix", func(t *testing.T) {
		key := TypedKey[string](Global, "status", "idle")
		assert.Equal(t, "status", key.Name())
		assert.Equal(t, "idle", key.Zero())
	})
}

func TestNamespaceTypedListKey(t *testing.T) {
	toolNS := MustNamespace("tool")

	t.Run("creates list key with namespace prefix", func(t *testing.T) {
		key := TypedListKey[string](toolNS, "results", 100, nil)
		assert.Equal(t, "tool.results", key.Name())
		assert.Equal(t, 100, key.MaxSize())
	})

	t.Run("global namespace has no prefix", func(t *testing.T) {
		key := TypedListKey[int](Global, "values", 50, nil)
		assert.Equal(t, "values", key.Name())
		assert.Equal(t, 50, key.MaxSize())
	})
}

func TestIsNamespaced(t *testing.T) {
	tests := []struct {
		name     string
		keyName  string
		expected bool
	}{
		{"namespaced key", "model.messages", true},
		{"global key", "messages", false},
		{"reserved key", "__messages__", false},
		{"multiple dots", "agent.sub.value", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNamespaced(tt.keyName))
		})
	}
}

func TestParseNamespacedKey(t *testing.T) {
	tests := []struct {
		name          string
		keyName       string
		wantNamespace string
		wantLocal     string
	}{
		{
			name:          "namespaced key",
			keyName:       "model.messages",
			wantNamespace: "model",
			wantLocal:     "messages",
		},
		{
			name:          "global key",
			keyName:       "messages",
			wantNamespace: "",
			wantLocal:     "messages",
		},
		{
			name:          "reserved key",
			keyName:       "__messages__",
			wantNamespace: "",
			wantLocal:     "__messages__",
		},
		{
			name:          "nested namespace",
			keyName:       "agent.sub.value",
			wantNamespace: "agent",
			wantLocal:     "sub.value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, local := ParseNamespacedKey(tt.keyName)
			assert.Equal(t, tt.wantNamespace, ns)
			assert.Equal(t, tt.wantLocal, local)
		})
	}
}

func TestExtractNamespace(t *testing.T) {
	tests := []struct {
		name     string
		keyName  string
		expected string
	}{
		{"namespaced", "model.messages", "model"},
		{"global", "messages", ""},
		{"nested", "agent.sub.value", "agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ExtractNamespace(tt.keyName))
		})
	}
}

func TestGetNamespaceView(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Create namespaced keys
	modelNS := MustNamespace("model")
	toolNS := MustNamespace("tool")

	modelCounter := TypedKey[int](modelNS, "counter", 0)
	modelStatus := TypedKey[string](modelNS, "status", "")
	toolResults := TypedListKey[string](toolNS, "results", 10, nil)
	globalConfig := TypedKey[string](Global, "config", "")

	// Register keys
	require.NoError(t, RegisterKey(mgr, modelCounter))
	require.NoError(t, RegisterKey(mgr, modelStatus))
	require.NoError(t, RegisterListKey(mgr, toolResults))
	require.NoError(t, RegisterKey(mgr, globalConfig))

	// Set values
	require.NoError(t, Set(ctx, mgr, modelCounter, 42))
	require.NoError(t, Set(ctx, mgr, modelStatus, "active"))
	require.NoError(t, Append(ctx, mgr, toolResults, "result1"))
	require.NoError(t, Set(ctx, mgr, globalConfig, "prod"))

	// Get view
	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	t.Run("model namespace view", func(t *testing.T) {
		modelView := GetNamespaceView(view, modelNS)
		assert.Len(t, modelView, 2)
		assert.Equal(t, 42, modelView["counter"])
		assert.Equal(t, "active", modelView["status"])
	})

	t.Run("tool namespace view", func(t *testing.T) {
		toolView := GetNamespaceView(view, toolNS)
		assert.Len(t, toolView, 1)
		// List keys return []interface{} when accessed via raw data
		results := toolView["results"].([]interface{})
		assert.Len(t, results, 1)
		assert.Equal(t, "result1", results[0])
	})

	t.Run("global namespace view", func(t *testing.T) {
		globalView := GetNamespaceView(view, Global)
		assert.Len(t, globalView, 1)
		assert.Equal(t, "prod", globalView["config"])
	})
}

func TestListNamespaces(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Create namespaced keys
	ns1 := MustNamespace("agent1")
	ns2 := MustNamespace("agent2")
	ns3 := MustNamespace("tool")

	key1 := TypedKey[int](ns1, "counter", 0)
	key2 := TypedKey[string](ns2, "status", "")
	key3 := TypedKey[bool](ns3, "active", false)
	globalKey := TypedKey[string](Global, "config", "")

	require.NoError(t, RegisterKey(mgr, key1))
	require.NoError(t, RegisterKey(mgr, key2))
	require.NoError(t, RegisterKey(mgr, key3))
	require.NoError(t, RegisterKey(mgr, globalKey))

	require.NoError(t, Set(ctx, mgr, key1, 1))
	require.NoError(t, Set(ctx, mgr, key2, "ok"))
	require.NoError(t, Set(ctx, mgr, key3, true))
	require.NoError(t, Set(ctx, mgr, globalKey, "prod"))

	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	namespaces := ListNamespaces(view)

	// Should have 3 namespaces (not including global)
	assert.Len(t, namespaces, 3)

	// Check they're sorted
	assert.Equal(t, "agent1", namespaces[0].Name())
	assert.Equal(t, "agent2", namespaces[1].Name())
	assert.Equal(t, "tool", namespaces[2].Name())
}

func TestCopyNamespace(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Create source namespace
	fromNS := MustNamespace("agent1")
	toNS := MustNamespace("agent2")

	fromCounterKey := TypedKey[int](fromNS, "counter", 0)
	fromStatusKey := TypedKey[string](fromNS, "status", "")
	toCounterKey := TypedKey[int](toNS, "counter", 0)
	toStatusKey := TypedKey[string](toNS, "status", "")

	// Register both source and target keys
	require.NoError(t, RegisterKey(mgr, fromCounterKey))
	require.NoError(t, RegisterKey(mgr, fromStatusKey))
	require.NoError(t, RegisterKey(mgr, toCounterKey))
	require.NoError(t, RegisterKey(mgr, toStatusKey))

	require.NoError(t, Set(ctx, mgr, fromCounterKey, 42))
	require.NoError(t, Set(ctx, mgr, fromStatusKey, "active"))

	// Copy to different namespace
	err := CopyNamespace(ctx, mgr, fromNS, toNS)
	require.NoError(t, err)

	// Verify copied keys exist with new namespace
	snap, err := mgr.Snapshot(ctx, nil)
	require.NoError(t, err)

	assert.Equal(t, 42, snap.Data["agent2.counter"])
	assert.Equal(t, "active", snap.Data["agent2.status"])

	// Original keys still exist
	assert.Equal(t, 42, snap.Data["agent1.counter"])
	assert.Equal(t, "active", snap.Data["agent1.status"])
}

func TestDeleteNamespace(t *testing.T) {
	t.Skip("DeleteNamespace not implemented - channels cannot be deleted from state")

	// TODO: Implement once channel deletion support is added
	// Current limitation: State channels can only have values updated, not deleted
	// This requires either:
	// 1. Channel removal API
	// 2. Special "clear" operation for list channels
	// 3. Metadata marking keys as "deleted"
}

func TestNamespaceIsolation(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Create two namespaces with same key names
	ns1 := MustNamespace("agent1")
	ns2 := MustNamespace("agent2")

	counter1 := TypedKey[int](ns1, "counter", 0)
	counter2 := TypedKey[int](ns2, "counter", 0)

	require.NoError(t, RegisterKey(mgr, counter1))
	require.NoError(t, RegisterKey(mgr, counter2))

	// Set different values
	require.NoError(t, Set(ctx, mgr, counter1, 10))
	require.NoError(t, Set(ctx, mgr, counter2, 20))

	// Verify isolation
	view, err := mgr.CreateReadView(ctx)
	require.NoError(t, err)

	val1 := GetFromView(view, counter1)
	val2 := GetFromView(view, counter2)

	assert.Equal(t, 10, val1)
	assert.Equal(t, 20, val2)
	assert.NotEqual(t, val1, val2, "namespaces should be isolated")
}
