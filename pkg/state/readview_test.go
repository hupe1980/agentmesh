package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReadView(t *testing.T) {
	t.Run("create read view from snapshot", func(t *testing.T) {
		s := NewState()
		snap := s.Snapshot()
		
		view := NewReadView(snap)
		require.NotNil(t, view)
		assert.NotNil(t, view.snap)
	})
}

func TestGetFromView(t *testing.T) {
	t.Run("get value from view", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)
		
		Register(s, key)
		
		ctx := context.Background()
		Set(ctx, s, key, 100)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		val := GetFromView(view, key)
		assert.Equal(t, 100, val)
	})

	t.Run("get default value for unset key", func(t *testing.T) {
		s := NewState()
		key := NewKey[string]("name", "default")
		
		Register(s, key)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		val := GetFromView(view, key)
		assert.Equal(t, "default", val)
	})

	t.Run("view is immutable", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)
		
		Register(s, key)
		
		ctx := context.Background()
		Set(ctx, s, key, 100)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		// Verify initial value
		assert.Equal(t, 100, GetFromView(view, key))
		
		// Modify state
		Set(ctx, s, key, 200)
		
		// View should still see old value
		assert.Equal(t, 100, GetFromView(view, key))
	})
}

func TestReadViewHas(t *testing.T) {
	t.Run("has returns true for registered key", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)
		
		Register(s, key)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		assert.True(t, view.Has("counter"))
	})

	t.Run("has returns false for unregistered key", func(t *testing.T) {
		s := NewState()
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		assert.False(t, view.Has("nonexistent"))
	})
}

func TestReadViewVersion(t *testing.T) {
	t.Run("view version matches snapshot version", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)
		
		Register(s, key)
		
		ctx := context.Background()
		Set(ctx, s, key, 100)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		assert.Equal(t, snap.Version(), view.Version())
	})

	t.Run("view version is frozen", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)
		
		Register(s, key)
		
		ctx := context.Background()
		Set(ctx, s, key, 100)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		viewVersion := view.Version()
		
		// Modify state
		Set(ctx, s, key, 200)
		
		// View version should not change
		assert.Equal(t, viewVersion, view.Version())
	})
}

func TestReadViewKeys(t *testing.T) {
	t.Run("keys returns all registered keys", func(t *testing.T) {
		s := NewState()
		key1 := NewKey[int]("counter", 0)
		key2 := NewKey[string]("name", "")
		key3 := NewKey[bool]("flag", false)
		
		Register(s, key1)
		Register(s, key2)
		Register(s, key3)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		keys := view.Keys()
		assert.Len(t, keys, 3)
		assert.Contains(t, keys, "counter")
		assert.Contains(t, keys, "name")
		assert.Contains(t, keys, "flag")
	})

	t.Run("keys returns empty for empty state", func(t *testing.T) {
		s := NewState()
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		keys := view.Keys()
		assert.Empty(t, keys)
	})
}

func TestNewUpdates(t *testing.T) {
	t.Run("create empty updates", func(t *testing.T) {
		updates := NewUpdates()
		require.NotNil(t, updates)
		assert.Empty(t, updates)
	})
}

func TestSetInUpdates(t *testing.T) {
	t.Run("set value in updates", func(t *testing.T) {
		key := NewKey[int]("counter", 0)
		updates := NewUpdates()
		
		SetInUpdates(updates, key, 100)
		
		assert.Contains(t, updates, "counter")
		assert.Equal(t, 100, updates["counter"])
	})

	t.Run("set multiple values", func(t *testing.T) {
		key1 := NewKey[int]("counter", 0)
		key2 := NewKey[string]("name", "")
		updates := NewUpdates()
		
		SetInUpdates(updates, key1, 100)
		SetInUpdates(updates, key2, "Alice")
		
		assert.Len(t, updates, 2)
		assert.Equal(t, 100, updates["counter"])
		assert.Equal(t, "Alice", updates["name"])
	})

	t.Run("set returns updates for chaining", func(t *testing.T) {
		key1 := NewKey[int]("counter", 0)
		key2 := NewKey[string]("name", "")
		
		updates := SetInUpdates(NewUpdates(), key1, 100)
		updates = SetInUpdates(updates, key2, "Bob")
		
		assert.Len(t, updates, 2)
	})

	t.Run("overwrite existing value", func(t *testing.T) {
		key := NewKey[int]("counter", 0)
		updates := NewUpdates()
		
		SetInUpdates(updates, key, 100)
		SetInUpdates(updates, key, 200)
		
		assert.Equal(t, 200, updates["counter"])
	})
}

func TestAppendInUpdates(t *testing.T) {
	t.Run("append value in updates", func(t *testing.T) {
		key := NewListKey[int]("numbers", 0)
		updates := NewUpdates()
		
		AppendInUpdates(updates, key, 42)
		
		assert.Contains(t, updates, "numbers")
		assert.Equal(t, 42, updates["numbers"])
	})

	t.Run("append returns updates for chaining", func(t *testing.T) {
		key := NewListKey[int]("numbers", 0)
		
		updates := AppendInUpdates(NewUpdates(), key, 1)
		
		assert.Len(t, updates, 1)
	})
}

func TestUpdatesIntegration(t *testing.T) {
	t.Run("updates work with state", func(t *testing.T) {
		s := NewState()
		key1 := NewKey[int]("counter", 0)
		key2 := NewKey[string]("name", "")
		
		Register(s, key1)
		Register(s, key2)
		
		updates := NewUpdates()
		SetInUpdates(updates, key1, 100)
		SetInUpdates(updates, key2, "Alice")
		
		ctx := context.Background()
		err := s.ApplyUpdates(ctx, updates)
		require.NoError(t, err)
		
		assert.Equal(t, 100, Get(s, key1))
		assert.Equal(t, "Alice", Get(s, key2))
	})

	t.Run("build updates then apply", func(t *testing.T) {
		s := NewState()
		counterKey := NewKey[int]("counter", 0)
		nameKey := NewKey[string]("name", "")
		flagKey := NewKey[bool]("flag", false)
		
		Register(s, counterKey)
		Register(s, nameKey)
		Register(s, flagKey)
		
		// Build updates fluently
		updates := NewUpdates()
		updates = SetInUpdates(updates, counterKey, 42)
		updates = SetInUpdates(updates, nameKey, "Bob")
		updates = SetInUpdates(updates, flagKey, true)
		
		ctx := context.Background()
		err := s.ApplyUpdates(ctx, updates)
		require.NoError(t, err)
		
		snap := s.Snapshot()
		view := NewReadView(snap)
		
		assert.Equal(t, 42, GetFromView(view, counterKey))
		assert.Equal(t, "Bob", GetFromView(view, nameKey))
		assert.True(t, GetFromView(view, flagKey))
	})
}

func TestReadViewIsolation(t *testing.T) {
	t.Run("multiple views are independent", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)
		
		Register(s, key)
		
		ctx := context.Background()
		
		// Create view 1
		Set(ctx, s, key, 100)
		snap1 := s.Snapshot()
		view1 := NewReadView(snap1)
		
		// Create view 2
		Set(ctx, s, key, 200)
		snap2 := s.Snapshot()
		view2 := NewReadView(snap2)
		
		// Create view 3
		Set(ctx, s, key, 300)
		snap3 := s.Snapshot()
		view3 := NewReadView(snap3)
		
		// Each view should see its snapshot value
		assert.Equal(t, 100, GetFromView(view1, key))
		assert.Equal(t, 200, GetFromView(view2, key))
		assert.Equal(t, 300, GetFromView(view3, key))
	})
}
