package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerManagedValues(t *testing.T) {
	ctx := context.Background()

	t.Run("register and access managed values", func(t *testing.T) {
		mgr := NewManager()

		// Register managed values
		configMV := NewManagedValue[string]("config")
		err := RegisterManagedValue(mgr, configMV)
		require.NoError(t, err)

		portMV := NewManagedValue[int]("port")
		err = RegisterManagedValue(mgr, portMV)
		require.NoError(t, err)

		// Set values
		err = SetManagedValue(mgr, ctx, "config", "production")
		require.NoError(t, err)

		err = SetManagedValue(mgr, ctx, "port", 8080)
		require.NoError(t, err)

		// Get values (type-safe)
		config, err := GetManagedValue[string](mgr, ctx, "config")
		require.NoError(t, err)
		assert.Equal(t, "production", config)

		port, err := GetManagedValue[int](mgr, ctx, "port")
		require.NoError(t, err)
		assert.Equal(t, 8080, port)
	})

	t.Run("duplicate registration", func(t *testing.T) {
		mgr := NewManager()

		mv := NewManagedValue[string]("test")
		err := RegisterManagedValue(mgr, mv)
		require.NoError(t, err)

		// Second registration should fail
		err = RegisterManagedValue(mgr, mv)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("type mismatch", func(t *testing.T) {
		mgr := NewManager()

		mv := NewManagedValue[string]("value")
		err := RegisterManagedValue(mgr, mv)
		require.NoError(t, err)

		err = SetManagedValue(mgr, ctx, "value", "string")
		require.NoError(t, err)

		// Try to get as wrong type
		_, err = GetManagedValue[int](mgr, ctx, "value")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "wrong type")
	})

	t.Run("nonexistent managed value", func(t *testing.T) {
		mgr := NewManager()

		_, err := GetManagedValue[string](mgr, ctx, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("list managed value names", func(t *testing.T) {
		mgr := NewManager()

		// Register multiple
		RegisterManagedValue(mgr, NewManagedValue[string]("a"))
		RegisterManagedValue(mgr, NewManagedValue[int]("b"))
		RegisterManagedValue(mgr, NewManagedValue[bool]("c"))

		names := mgr.GetManagedValueNames()
		assert.Len(t, names, 3)
		assert.Contains(t, names, "a")
		assert.Contains(t, names, "b")
		assert.Contains(t, names, "c")
	})

	t.Run("managed values separate from persistent state", func(t *testing.T) {
		mgr := NewManager()

		// Register persistent key
		counterKey := NewKey("counter", 0)
		err := RegisterKey(mgr, counterKey)
		require.NoError(t, err)

		// Register managed value
		configMV := NewManagedValue[string]("config")
		err = RegisterManagedValue(mgr, configMV)
		require.NoError(t, err)

		// Set both
		err = Set(ctx, mgr, counterKey, 42)
		require.NoError(t, err)

		err = SetManagedValue(mgr, ctx, "config", "test")
		require.NoError(t, err)

		// Verify both are accessible
		counter, err := Get[int](ctx, mgr, counterKey)
		require.NoError(t, err)
		assert.Equal(t, 42, counter)

		config, err := GetManagedValue[string](mgr, ctx, "config")
		require.NoError(t, err)
		assert.Equal(t, "test", config)

		// Key insight: Managed values are NOT in the registered keys
		// (they're separate from the channel-based state system)
		registeredKeys := mgr.RegisteredKeys()
		assert.Contains(t, registeredKeys, "counter")
		assert.NotContains(t, registeredKeys, "config") // config is managed, not a key
	})

	t.Run("struct type managed value", func(t *testing.T) {
		type RuntimeConfig struct {
			APIKey  string
			Timeout int
		}

		mgr := NewManager()

		configMV := NewManagedValue[*RuntimeConfig]("runtime_config")
		err := RegisterManagedValue(mgr, configMV)
		require.NoError(t, err)

		cfg := &RuntimeConfig{
			APIKey:  "secret",
			Timeout: 30,
		}
		err = SetManagedValue(mgr, ctx, "runtime_config", cfg)
		require.NoError(t, err)

		retrieved, err := GetManagedValue[*RuntimeConfig](mgr, ctx, "runtime_config")
		require.NoError(t, err)
		assert.Equal(t, "secret", retrieved.APIKey)
		assert.Equal(t, 30, retrieved.Timeout)
	})
}
