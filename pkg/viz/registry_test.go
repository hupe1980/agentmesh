package viz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_NewRegistry(t *testing.T) {
	registry := NewRegistry()
	assert.NotNil(t, registry)
	assert.NotNil(t, registry.runnables)
	assert.Empty(t, registry.List())
}

func TestRegistry_Register(t *testing.T) {
	t.Run("successful registration", func(t *testing.T) {
		registry := NewRegistry()
		runnable := &mockRunnable{}

		err := registry.Register("test-id", runnable)
		require.NoError(t, err)

		// Verify it was registered
		ids := registry.List()
		assert.Contains(t, ids, "test-id")
	})

	t.Run("duplicate registration fails", func(t *testing.T) {
		registry := NewRegistry()
		runnable := &mockRunnable{}

		err := registry.Register("test-id", runnable)
		require.NoError(t, err)

		// Try to register again with same ID
		err = registry.Register("test-id", runnable)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("multiple different registrations", func(t *testing.T) {
		registry := NewRegistry()

		err := registry.Register("id1", &mockRunnable{})
		require.NoError(t, err)

		err = registry.Register("id2", &mockRunnable{})
		require.NoError(t, err)

		ids := registry.List()
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, "id1")
		assert.Contains(t, ids, "id2")
	})
}

func TestRegistry_Get(t *testing.T) {
	t.Run("get existing runnable", func(t *testing.T) {
		registry := NewRegistry()
		runnable := &mockRunnable{nodes: []string{"custom-node"}}

		err := registry.Register("test-id", runnable)
		require.NoError(t, err)

		retrieved, err := registry.Get("test-id")
		require.NoError(t, err)
		assert.Equal(t, runnable, retrieved)
		assert.Equal(t, []string{"custom-node"}, retrieved.GetNodes())
	})

	t.Run("get non-existent runnable", func(t *testing.T) {
		registry := NewRegistry()

		retrieved, err := registry.Get("non-existent")
		assert.Error(t, err)
		assert.Nil(t, retrieved)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistry_List(t *testing.T) {
	t.Run("empty registry", func(t *testing.T) {
		registry := NewRegistry()
		ids := registry.List()
		assert.Empty(t, ids)
		assert.NotNil(t, ids) // Should be empty slice, not nil
	})

	t.Run("with registered runnables", func(t *testing.T) {
		registry := NewRegistry()

		registry.Register("id1", &mockRunnable{})
		registry.Register("id2", &mockRunnable{})
		registry.Register("id3", &mockRunnable{})

		ids := registry.List()
		assert.Len(t, ids, 3)
		// Order is not guaranteed, so check containment
		assert.Contains(t, ids, "id1")
		assert.Contains(t, ids, "id2")
		assert.Contains(t, ids, "id3")
	})
}

func TestRegistry_Unregister(t *testing.T) {
	t.Run("unregister existing runnable", func(t *testing.T) {
		registry := NewRegistry()
		runnable := &mockRunnable{}

		err := registry.Register("test-id", runnable)
		require.NoError(t, err)

		err = registry.Unregister("test-id")
		require.NoError(t, err)

		// Verify it was removed
		ids := registry.List()
		assert.Empty(t, ids)

		// Verify Get fails
		_, err = registry.Get("test-id")
		assert.Error(t, err)
	})

	t.Run("unregister non-existent runnable", func(t *testing.T) {
		registry := NewRegistry()

		err := registry.Unregister("non-existent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("unregister one of multiple", func(t *testing.T) {
		registry := NewRegistry()

		registry.Register("id1", &mockRunnable{})
		registry.Register("id2", &mockRunnable{})
		registry.Register("id3", &mockRunnable{})

		err := registry.Unregister("id2")
		require.NoError(t, err)

		ids := registry.List()
		assert.Len(t, ids, 2)
		assert.Contains(t, ids, "id1")
		assert.Contains(t, ids, "id3")
		assert.NotContains(t, ids, "id2")
	})
}

func TestRegistry_ThreadSafety(t *testing.T) {
	// This test verifies concurrent access doesn't cause races
	registry := NewRegistry()

	done := make(chan bool)
	iterations := 100

	// Concurrent registrations
	go func() {
		for i := 0; i < iterations; i++ {
			registry.Register("concurrent-1", &mockRunnable{})
			registry.Unregister("concurrent-1")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < iterations; i++ {
			registry.Register("concurrent-2", &mockRunnable{})
			registry.Unregister("concurrent-2")
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < iterations; i++ {
			registry.List()
			registry.Get("concurrent-1")
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done

	// If we get here without data races, test passes
	assert.True(t, true)
}

func TestRegistry_RegisterAfterUnregister(t *testing.T) {
	registry := NewRegistry()
	runnable1 := &mockRunnable{nodes: []string{"node1"}}
	runnable2 := &mockRunnable{nodes: []string{"node2"}}

	// Register, unregister, then register again with same ID
	err := registry.Register("test-id", runnable1)
	require.NoError(t, err)

	err = registry.Unregister("test-id")
	require.NoError(t, err)

	err = registry.Register("test-id", runnable2)
	require.NoError(t, err)

	// Verify the new runnable is registered
	retrieved, err := registry.Get("test-id")
	require.NoError(t, err)
	assert.Equal(t, runnable2, retrieved)
	assert.Equal(t, []string{"node2"}, retrieved.GetNodes())
}
