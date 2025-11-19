package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleManagedValue(t *testing.T) {
	ctx := context.Background()

	t.Run("basic get/set", func(t *testing.T) {
		mv := NewManagedValue[string]("config")
		assert.Equal(t, "config", mv.Name())

		// Get before set should error
		_, err := mv.Get(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "has not been set")

		// Set and get
		err = mv.Set(ctx, "test-value")
		require.NoError(t, err)

		value, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "test-value", value)
	})

	t.Run("with default value", func(t *testing.T) {
		mv := NewManagedValueWithDefault("counter", 42)

		value, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 42, value)

		// Update value
		err = mv.Set(ctx, 100)
		require.NoError(t, err)

		value, err = mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 100, value)
	})

	t.Run("type safety", func(t *testing.T) {
		// String managed value
		strMV := NewManagedValue[string]("str")
		err := strMV.Set(ctx, "hello")
		require.NoError(t, err)

		value, err := strMV.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "hello", value)

		// Int managed value
		intMV := NewManagedValue[int]("int")
		err = intMV.Set(ctx, 123)
		require.NoError(t, err)

		intValue, err := intMV.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 123, intValue)
	})

	t.Run("struct type", func(t *testing.T) {
		type Config struct {
			Host string
			Port int
		}

		mv := NewManagedValue[Config]("server-config")
		err := mv.Set(ctx, Config{Host: "localhost", Port: 8080})
		require.NoError(t, err)

		config, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "localhost", config.Host)
		assert.Equal(t, 8080, config.Port)
	})

	t.Run("pointer type", func(t *testing.T) {
		type Session struct {
			UserID string
			Token  string
		}

		mv := NewManagedValue[*Session]("session")
		session := &Session{UserID: "user123", Token: "token456"}
		err := mv.Set(ctx, session)
		require.NoError(t, err)

		retrieved, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "user123", retrieved.UserID)
		assert.Equal(t, "token456", retrieved.Token)
		assert.Same(t, session, retrieved) // Same pointer
	})
}

func TestComputedManagedValue(t *testing.T) {
	ctx := context.Background()

	t.Run("computed value", func(t *testing.T) {
		callCount := 0
		mv := NewComputedManagedValue("timestamp", func(ctx context.Context) (int64, error) {
			callCount++
			return time.Now().Unix(), nil
		})

		assert.Equal(t, "timestamp", mv.Name())

		// First call
		val1, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Greater(t, val1, int64(0))
		assert.Equal(t, 1, callCount)

		// Second call (recomputes)
		val2, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, val2, val1) // May be same or greater
		assert.Equal(t, 2, callCount)
	})

	t.Run("computed error", func(t *testing.T) {
		expectedErr := errors.New("computation failed")
		mv := NewComputedManagedValue("failing", func(ctx context.Context) (string, error) {
			return "", expectedErr
		})

		_, err := mv.Get(ctx)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("set not supported", func(t *testing.T) {
		mv := NewComputedManagedValue("readonly", func(ctx context.Context) (string, error) {
			return "computed", nil
		})

		err := mv.Set(ctx, "new-value")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot set computed")
	})
}

func TestCachedManagedValue(t *testing.T) {
	t.Run("cache hit", func(t *testing.T) {
		callCount := 0
		source := NewComputedManagedValue("source", func(ctx context.Context) (int, error) {
			callCount++
			return 42 + callCount, nil
		})

		cached := NewCachedManagedValue("cached", source, 60) // 60 second TTL

		ctx := context.WithValue(context.Background(), "timestamp", int64(1000))

		// First call - cache miss
		val1, err := cached.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 43, val1) // 42 + 1
		assert.Equal(t, 1, callCount)

		// Second call within TTL - cache hit
		ctx = context.WithValue(context.Background(), "timestamp", int64(1030)) // +30s
		val2, err := cached.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 43, val2)     // Same cached value
		assert.Equal(t, 1, callCount) // No new call
	})

	t.Run("cache miss after expiry", func(t *testing.T) {
		callCount := 0
		source := NewComputedManagedValue("source", func(ctx context.Context) (string, error) {
			callCount++
			return "value-" + string(rune(callCount)), nil
		})

		cached := NewCachedManagedValue("cached", source, 30) // 30 second TTL

		ctx := context.WithValue(context.Background(), "timestamp", int64(1000))

		// First call
		val1, err := cached.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, callCount)

		// Call after expiry
		ctx = context.WithValue(context.Background(), "timestamp", int64(1100)) // +100s > 30s TTL
		val2, err := cached.Get(ctx)
		require.NoError(t, err)
		assert.NotEqual(t, val1, val2) // Different value
		assert.Equal(t, 2, callCount)  // New call made
	})

	t.Run("set invalidates cache", func(t *testing.T) {
		source := NewManagedValueWithDefault("source", 100)
		cached := NewCachedManagedValue("cached", source, 60)

		ctx := context.WithValue(context.Background(), "timestamp", int64(1000))

		// Get cached value
		val1, err := cached.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 100, val1)

		// Set new value (invalidates cache)
		err = cached.Set(ctx, 200)
		require.NoError(t, err)

		// Get should fetch new value even with same timestamp
		ctx = context.WithValue(context.Background(), "timestamp", int64(1000))
		val2, err := cached.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 200, val2)
	})
}

func TestWrapManagedValue(t *testing.T) {
	ctx := context.Background()

	t.Run("wrap and unwrap", func(t *testing.T) {
		mv := NewManagedValue[string]("wrapped")
		err := mv.Set(ctx, "original")
		require.NoError(t, err)

		// Wrap
		wrapped := WrapManagedValue(mv)
		assert.Equal(t, "wrapped", wrapped.Name())

		// Get (type-erased)
		value, err := wrapped.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "original", value.(string))

		// Set (type-erased)
		err = wrapped.Set(ctx, "updated")
		require.NoError(t, err)

		// Verify through original
		direct, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "updated", direct)
	})

	t.Run("type mismatch on set", func(t *testing.T) {
		mv := NewManagedValue[int]("number")
		wrapped := WrapManagedValue(mv)

		// Try to set wrong type
		err := wrapped.Set(ctx, "not-a-number")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "type mismatch")
	})

	t.Run("heterogeneous map", func(t *testing.T) {
		// Simulate manager's storage
		managedValues := map[string]*ManagedValueAny{
			"config":  WrapManagedValue(NewManagedValue[string]("config")),
			"port":    WrapManagedValue(NewManagedValue[int]("port")),
			"enabled": WrapManagedValue(NewManagedValue[bool]("enabled")),
		}

		// Set values
		err := managedValues["config"].Set(ctx, "prod")
		require.NoError(t, err)
		err = managedValues["port"].Set(ctx, 8080)
		require.NoError(t, err)
		err = managedValues["enabled"].Set(ctx, true)
		require.NoError(t, err)

		// Get values (with type assertions)
		config, err := managedValues["config"].Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "prod", config.(string))

		port, err := managedValues["port"].Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 8080, port.(int))

		enabled, err := managedValues["enabled"].Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, true, enabled.(bool))
	})
}

func TestConcurrency(t *testing.T) {
	ctx := context.Background()

	t.Run("concurrent read/write", func(t *testing.T) {
		mv := NewManagedValueWithDefault("counter", 0)

		const goroutines = 100
		const iterations = 100

		// Concurrent writes
		done := make(chan bool, goroutines)
		for i := 0; i < goroutines; i++ {
			go func(id int) {
				for j := 0; j < iterations; j++ {
					_ = mv.Set(ctx, id*iterations+j)
				}
				done <- true
			}(i)
		}

		// Concurrent reads
		for i := 0; i < goroutines; i++ {
			go func() {
				for j := 0; j < iterations; j++ {
					_, _ = mv.Get(ctx)
				}
			}()
		}

		// Wait for writers
		for i := 0; i < goroutines; i++ {
			<-done
		}

		// Final read should succeed
		_, err := mv.Get(ctx)
		assert.NoError(t, err)
	})
}

// Benchmark SimpleManagedValue operations
func BenchmarkSimpleManagedValue(b *testing.B) {
	ctx := context.Background()
	mv := NewManagedValueWithDefault("bench", 42)

	b.Run("Get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = mv.Get(ctx)
		}
	})

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = mv.Set(ctx, i)
		}
	})
}
