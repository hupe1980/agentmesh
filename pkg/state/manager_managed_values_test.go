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
		builder := NewManagerBuilder()

		// Register managed values
		configMV := NewManagedValue[string]("config")
		err := RegisterManagedValue(builder, configMV)
		require.NoError(t, err)

		portMV := NewManagedValue[int]("port")
		err = RegisterManagedValue(builder, portMV)
		require.NoError(t, err)

		// Build the manager (freezes registrations)
		mgr := builder.Build()

		// Set values
		err = SetManagedValue(ctx, mgr, "config", "production")
		require.NoError(t, err)

		err = SetManagedValue(ctx, mgr, "port", 8080)
		require.NoError(t, err)

		// Get values (type-safe)
		config, err := GetManagedValue[string](ctx, mgr, "config")
		require.NoError(t, err)
		assert.Equal(t, "production", config)

		port, err := GetManagedValue[int](ctx, mgr, "port")
		require.NoError(t, err)
		assert.Equal(t, 8080, port)
	})

	t.Run("duplicate registration", func(t *testing.T) {
		builder := NewManagerBuilder()

		mv := NewManagedValue[string]("test")
		err := RegisterManagedValue(builder, mv)
		require.NoError(t, err)

		// Second registration should fail
		err = RegisterManagedValue(builder, mv)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("type mismatch", func(t *testing.T) {
		builder := NewManagerBuilder()

		mv := NewManagedValue[string]("value")
		err := RegisterManagedValue(builder, mv)
		require.NoError(t, err)

		mgr := builder.Build()

		err = SetManagedValue(ctx, mgr, "value", "string")
		require.NoError(t, err)

		// Try to get as wrong type
		_, err = GetManagedValue[int](ctx, mgr, "value")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "wrong type")
	})

	t.Run("nonexistent managed value", func(t *testing.T) {
		builder := NewManagerBuilder()

		mgr := builder.Build()
		_, err := GetManagedValue[string](ctx, mgr, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("list managed value names", func(t *testing.T) {
		builder := NewManagerBuilder()

		// Register multiple
		RegisterManagedValue(builder, NewManagedValue[string]("a"))
		RegisterManagedValue(builder, NewManagedValue[int]("b"))
		RegisterManagedValue(builder, NewManagedValue[bool]("c"))

		mgr := builder.Build()
		names := mgr.GetManagedValueNames()
		assert.Len(t, names, 3)
		assert.Contains(t, names, "a")
		assert.Contains(t, names, "b")
		assert.Contains(t, names, "c")
	})

	t.Run("managed values separate from persistent state", func(t *testing.T) {
		builder := NewManagerBuilder()

		// Register persistent key
		counterKey := NewKey("counter", 0)
		err := RegisterKey(builder, counterKey)
		require.NoError(t, err)

		// Register managed value
		configMV := NewManagedValue[string]("config")
		err = RegisterManagedValue(builder, configMV)
		require.NoError(t, err)

		// Build the manager
		mgr := builder.Build()

		// Set both
		err = Set(ctx, mgr, counterKey, 42)
		require.NoError(t, err)

		err = SetManagedValue(ctx, mgr, "config", "test")
		require.NoError(t, err)

		// Verify both are accessible
		counter, err := Get[int](ctx, mgr, counterKey)
		require.NoError(t, err)
		assert.Equal(t, 42, counter)

		config, err := GetManagedValue[string](ctx, mgr, "config")
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

		builder := NewManagerBuilder()

		configMV := NewManagedValue[*RuntimeConfig]("runtime_config")
		err := RegisterManagedValue(builder, configMV)
		require.NoError(t, err)

		mgr := builder.Build()

		cfg := &RuntimeConfig{
			APIKey:  "secret",
			Timeout: 30,
		}
		err = SetManagedValue(ctx, mgr, "runtime_config", cfg)
		require.NoError(t, err)

		retrieved, err := GetManagedValue[*RuntimeConfig](ctx, mgr, "runtime_config")
		require.NoError(t, err)
		assert.Equal(t, "secret", retrieved.APIKey)
		assert.Equal(t, 30, retrieved.Timeout)
	})
}
