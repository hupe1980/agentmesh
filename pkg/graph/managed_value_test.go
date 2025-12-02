package graph

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticManagedValue(t *testing.T) {
	ctx := context.Background()

	t.Run("basic get/set", func(t *testing.T) {
		mv := NewManagedValue("test", "")
		assert.Equal(t, "test", mv.Name())

		// Initial value
		val, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "", val)

		// Set value
		err = mv.Set(ctx, "hello")
		require.NoError(t, err)

		val, err = mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "hello", val)
	})

	t.Run("with initial value", func(t *testing.T) {
		mv := NewManagedValue("timeout", 30*time.Second)

		val, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, val)
	})

	t.Run("struct type", func(t *testing.T) {
		type Config struct {
			APIKey  string
			Timeout time.Duration
		}

		mv := NewManagedValue("config", &Config{APIKey: "sk_test", Timeout: 10 * time.Second})

		config, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "sk_test", config.APIKey)
		assert.Equal(t, 10*time.Second, config.Timeout)
	})

	t.Run("thread safety", func(t *testing.T) {
		mv := NewManagedValue("counter", 0)
		var wg sync.WaitGroup

		// Concurrent writes
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(val int) {
				defer wg.Done()
				_ = mv.Set(ctx, val)
			}(i)
		}

		// Concurrent reads
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = mv.Get(ctx)
			}()
		}

		wg.Wait()
		// Should not panic
	})
}

func TestManagedValueProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("recomputes on every access without cache", func(t *testing.T) {
		var counter int64
		mv := NewManagedValueProvider("counter", func(ctx context.Context) (int64, error) {
			return atomic.AddInt64(&counter, 1), nil
		})

		assert.Equal(t, "counter", mv.Name())

		val1, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val1)

		val2, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), val2)

		val3, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(3), val3)
	})

	t.Run("current time example", func(t *testing.T) {
		mv := NewManagedValueProvider("current_time", func(ctx context.Context) (time.Time, error) {
			return time.Now(), nil
		})

		time1, err := mv.Get(ctx)
		require.NoError(t, err)

		time.Sleep(10 * time.Millisecond)

		time2, err := mv.Get(ctx)
		require.NoError(t, err)

		assert.True(t, time2.After(time1))
	})

	t.Run("set is no-op", func(t *testing.T) {
		mv := NewManagedValueProvider("fixed", func(ctx context.Context) (string, error) {
			return "computed", nil
		})

		// Set should be ignored
		err := mv.Set(ctx, "ignored")
		require.NoError(t, err)

		val, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "computed", val)
	})

	t.Run("caches value within TTL", func(t *testing.T) {
		var fetchCount int64
		mv := NewManagedValueProvider("cached", func(ctx context.Context) (int64, error) {
			return atomic.AddInt64(&fetchCount, 1), nil
		}, WithCacheTTL(100*time.Millisecond))

		assert.Equal(t, "cached", mv.Name())

		// First call fetches
		val1, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val1)

		// Second call uses cache
		val2, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val2) // Same value, not refetched

		assert.Equal(t, int64(1), atomic.LoadInt64(&fetchCount))
	})

	t.Run("refetches after TTL expires", func(t *testing.T) {
		var fetchCount int64
		mv := NewManagedValueProvider("cached", func(ctx context.Context) (int64, error) {
			return atomic.AddInt64(&fetchCount, 1), nil
		}, WithCacheTTL(10*time.Millisecond))

		val1, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val1)

		// Wait for TTL to expire
		time.Sleep(20 * time.Millisecond)

		val2, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), val2) // Refetched

		assert.Equal(t, int64(2), atomic.LoadInt64(&fetchCount))
	})

	t.Run("invalidate clears cache", func(t *testing.T) {
		var fetchCount int64
		mv := NewManagedValueProvider("cached", func(ctx context.Context) (int64, error) {
			return atomic.AddInt64(&fetchCount, 1), nil
		}, WithCacheTTL(1*time.Hour))

		val1, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val1)

		mv.Invalidate()

		val2, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(2), val2) // Refetched after invalidation
	})

	t.Run("invalidate is no-op without cache", func(t *testing.T) {
		var fetchCount int64
		mv := NewManagedValueProvider("no_cache", func(ctx context.Context) (int64, error) {
			return atomic.AddInt64(&fetchCount, 1), nil
		})

		// Should not panic
		mv.Invalidate()

		val, err := mv.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, int64(1), val)
	})
}

func TestManagedValueUseCases(t *testing.T) {
	ctx := context.Background()

	t.Run("runtime config pattern", func(t *testing.T) {
		type RuntimeConfig struct {
			APIKey     string
			Timeout    time.Duration
			MaxRetries int
			Debug      bool
		}

		config := NewManagedValue("runtime_config", &RuntimeConfig{
			APIKey:     "sk_live_abc123",
			Timeout:    30 * time.Second,
			MaxRetries: 3,
			Debug:      false,
		})

		// Access directly - no registry needed
		cfg, err := config.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "sk_live_abc123", cfg.APIKey)
		assert.Equal(t, 3, cfg.MaxRetries)
	})

	t.Run("session state pattern", func(t *testing.T) {
		type Session struct {
			UserID    string
			Token     string
			LoginTime time.Time
		}

		session := NewManagedValue("session", (*Session)(nil))

		// Initialize session at runtime
		err := session.Set(ctx, &Session{
			UserID:    "user123",
			Token:     "tok_abc",
			LoginTime: time.Now(),
		})
		require.NoError(t, err)

		// Access directly
		s, err := session.Get(ctx)
		require.NoError(t, err)
		assert.Equal(t, "user123", s.UserID)
	})

	t.Run("metrics collector pattern", func(t *testing.T) {
		type Metrics struct {
			mu         sync.Mutex
			Executions map[string]int
		}

		metrics := NewManagedValue("metrics", &Metrics{
			Executions: make(map[string]int),
		})

		// Record executions
		m, _ := metrics.Get(ctx)
		m.mu.Lock()
		m.Executions["node1"]++
		m.Executions["node2"]++
		m.Executions["node1"]++
		m.mu.Unlock()

		// Check metrics
		m, _ = metrics.Get(ctx)
		assert.Equal(t, 2, m.Executions["node1"])
		assert.Equal(t, 1, m.Executions["node2"])
	})
}

func TestGetManaged(t *testing.T) {
	ctx := context.Background()

	t.Run("GetManaged with StaticManagedValue", func(t *testing.T) {
		apiKey := NewManagedValue("api_key", "sk_test_123")

		// Create BSP state with managed values
		bspState := NewBSPState(nil)
		registry := newManagedValueRegistry()
		registry.register(apiKey)
		bspState.setManagedValues(registry)
		view := bspState.ReadView()

		// Access managed value via GetManaged
		val := GetManaged(ctx, view, apiKey)
		assert.Equal(t, "sk_test_123", val)
	})

	t.Run("GetManaged without registry returns zero", func(t *testing.T) {
		apiKey := NewManagedValue("api_key", "")

		// BSP state without managed values
		bspState := NewBSPState(nil)
		view := bspState.ReadView()

		val := GetManaged(ctx, view, apiKey)
		assert.Equal(t, "", val)
	})

	t.Run("GetManaged with unregistered value returns zero", func(t *testing.T) {
		registry := newManagedValueRegistry()
		// Don't register anything

		bspState := NewBSPState(nil)
		bspState.setManagedValues(registry)
		view := bspState.ReadView()

		apiKey := NewManagedValue("api_key", "")
		val := GetManaged(ctx, view, apiKey)
		assert.Equal(t, "", val)
	})

	t.Run("GetManaged with ManagedValueProvider", func(t *testing.T) {
		var count int64
		counter := NewManagedValueProvider("counter", func(ctx context.Context) (int64, error) {
			return atomic.AddInt64(&count, 1), nil
		})

		registry := newManagedValueRegistry()
		registry.register(counter)

		bspState := NewBSPState(nil)
		bspState.setManagedValues(registry)
		view := bspState.ReadView()

		// Each call increments
		assert.Equal(t, int64(1), GetManaged(ctx, view, counter))
		assert.Equal(t, int64(2), GetManaged(ctx, view, counter))
		assert.Equal(t, int64(3), GetManaged(ctx, view, counter))
	})
}
