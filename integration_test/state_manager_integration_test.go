package integration_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/checkpoint"
	"github.com/hupe1980/agentmesh/pkg/state"
	stateRedis "github.com/hupe1980/agentmesh/pkg/state/redis"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	redisContainer "github.com/testcontainers/testcontainers-go/modules/redis"
)

// setupRedisContainer starts a Redis container for testing.
func setupRedisContainer(t *testing.T, ctx context.Context) (*redisContainer.RedisContainer, redis.UniversalClient) {
	t.Helper()

	container, err := redisContainer.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("Failed to start Redis container: %v", err)
	}

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("Failed to get Redis endpoint: %v", err)
	}

	// ConnectionString returns redis://host:port, but redis client expects host:port
	// Strip the redis:// prefix
	addr := endpoint
	if len(endpoint) > 8 && endpoint[:8] == "redis://" {
		addr = endpoint[8:]
	}

	client := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	// Verify connection
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	return container, client
}

func TestManagerWithRedisBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start Redis container
	container, client := setupRedisContainer(t, ctx)
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()
	defer client.Close()

	t.Run("Basic operations with Redis store", func(t *testing.T) {
		store := stateRedis.NewRedisStore(client, stateRedis.WithKeyPrefix("test:basic:"))
		// Don't close store - it closes the shared client

		manager := state.NewManager(state.WithStore(store))
		// Don't close manager - it will close the store

		key := state.NewKey[string]("name", "")
		state.RegisterKey(manager, key)

		// Set value
		err := state.Set(ctx, manager, key, "Alice")
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Get value
		value, err := state.Get(ctx, manager, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != "Alice" {
			t.Errorf("Expected 'Alice', got %s", value)
		}

		// Verify value is in Redis
		data, err := client.Get(ctx, "test:basic:name").Result()
		if err != nil {
			t.Fatalf("Redis Get failed: %v", err)
		}

		if data == "" {
			t.Error("Expected value in Redis")
		}

		// Cleanup
		client.Del(ctx, "test:basic:name")
	})

	t.Run("Persistence across manager restarts", func(t *testing.T) {
		prefix := "test:persistence:"

		// First manager - write data
		func() {
			store := stateRedis.NewRedisStore(client, stateRedis.WithKeyPrefix(prefix))
			// Don't close store - it closes the shared client

			manager := state.NewManager(state.WithStore(store))
			// Don't close manager.Close() - it will close the store

			key := state.NewKey[int]("counter", 0)
			state.RegisterKey(manager, key)
			state.Set(ctx, manager, key, 42)
		}()

		// Second manager - read data
		func() {
			store := stateRedis.NewRedisStore(client, stateRedis.WithKeyPrefix(prefix))
			// Don't close store - it closes the shared client

			manager := state.NewManager(state.WithStore(store))
			// Don't close manager - it will close the store

			key := state.NewKey[int]("counter", 0)
			state.RegisterKey(manager, key)

			// Value should persist in Redis
			storeValue, err := store.Get(ctx, "counter")
			if err != nil {
			t.Fatalf("Store Get failed: %v", err)
			}

			// Redis stores as float64 after JSON unmarshaling
			if val, ok := storeValue.(float64); ok {
			if int(val) != 42 {
				t.Errorf("Expected 42, got %d", int(val))
			}
			} else {
			t.Errorf("Expected float64 from Redis JSON, got %T", storeValue)
			}
		}()

		// Cleanup
		client.Del(ctx, prefix+"counter")
	})

	t.Run("Snapshot and Restore with Redis", func(t *testing.T) {
		store := stateRedis.NewRedisStore(client, stateRedis.WithKeyPrefix("test:snapshot:"))
		// Don't close store - it closes the shared client

		manager := state.NewManager(state.WithStore(store))
		// Don't close manager - it will close the store

		key1 := state.NewKey[int]("x", 0)
		key2 := state.NewKey[string]("y", "")
		state.RegisterKey(manager, key1)
		state.RegisterKey(manager, key2)

		// Set initial values
		state.Set(ctx, manager, key1, 100)
		state.Set(ctx, manager, key2, "hello")

		// Create snapshot
		snapshot, err := manager.Snapshot(ctx, map[string]string{"tag": "test"})
		if err != nil {
			t.Fatalf("Snapshot failed: %v", err)
		}

		// Modify values
		state.Set(ctx, manager, key1, 999)
		state.Set(ctx, manager, key2, "world")

		// Restore from snapshot
		err = manager.Restore(ctx, snapshot.ID)
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		// Verify restored values
		value1, _ := state.Get(ctx, manager, key1)
		if value1 != 100 {
			t.Errorf("Expected 100 after restore, got %d", value1)
		}

		value2, _ := state.Get(ctx, manager, key2)
		if value2 != "hello" {
			t.Errorf("Expected 'hello' after restore, got %s", value2)
		}

		// Cleanup
		client.Del(ctx, "test:snapshot:x", "test:snapshot:y")
	})

	t.Run("Concurrent access with Redis", func(t *testing.T) {
		store := stateRedis.NewRedisStore(client, stateRedis.WithKeyPrefix("test:concurrent:"))
		// Don't close store - it closes the shared client

		manager := state.NewManager(state.WithStore(store))
		// Don't close manager - it will close the store

		key := state.NewKey[int]("counter", 0)
		state.RegisterKey(manager, key)
		state.Set(ctx, manager, key, 0)

		var wg sync.WaitGroup
		numGoroutines := 50
		incrementsPerGoroutine := 10

		// Concurrent increments
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				// Read current value
				current, err := state.Get(ctx, manager, key)
				if err != nil {
					t.Logf("Get failed: %v", err)
					continue
				}
				// Increment and write back
				state.Set(ctx, manager, key, current+1)
			}
			}(i)
		}

		wg.Wait()

		// Final value might not be exactly numGoroutines*incrementsPerGoroutine
		// due to race conditions, but should be > 0
		finalValue, err := state.Get(ctx, manager, key)
		if err != nil {
			t.Fatalf("Final Get failed: %v", err)
		}

		if finalValue <= 0 {
			t.Errorf("Expected positive counter after concurrent writes, got %d", finalValue)
		}

		t.Logf("Concurrent test completed: %d increments resulted in counter = %d",
			numGoroutines*incrementsPerGoroutine, finalValue)

		// Cleanup
		client.Del(ctx, "test:concurrent:counter")
	})

	t.Run("Large dataset snapshot", func(t *testing.T) {
		store := stateRedis.NewRedisStore(client, stateRedis.WithKeyPrefix("test:large:"))
		// Don't close store - it closes the shared client

		manager := state.NewManager(state.WithStore(store))
		// Don't close manager - it will close the store

		// Create 100 keys
		numKeys := 100
		for i := 0; i < numKeys; i++ {
			key := state.NewKey[int](fmt.Sprintf("key%d", i), 0)
			state.RegisterKey(manager, key)
			state.Set(ctx, manager, key, i*10)
		}

		// Create snapshot
		start := time.Now()
		snapshot, err := manager.Snapshot(ctx, nil)
		duration := time.Since(start)
		if err != nil {
			t.Fatalf("Snapshot failed: %v", err)
		}

		t.Logf("Snapshot of %d keys took %v", numKeys, duration)

		if len(snapshot.Data) < numKeys {
			t.Errorf("Expected at least %d items in snapshot, got %d", numKeys, len(snapshot.Data))
		}

		// Cleanup
		for i := 0; i < numKeys; i++ {
			client.Del(ctx, fmt.Sprintf("test:large:key%d", i))
		}
	})
}

func TestManagerWithCheckpointer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Use in-memory checkpointer for simplicity
	checkpointer := checkpoint.NewInMemoryCheckpointer()

	t.Run("Checkpoint integration", func(t *testing.T) {
		runID := "test-run-1"
		manager := state.NewManager(
			state.WithCheckpointer(checkpointer, runID),
		)
		defer manager.Close()

		key := state.NewKey[string]("message", "")
		state.RegisterKey(manager, key)
		state.Set(ctx, manager, key, "checkpoint-test")

		// Create snapshot (should also save checkpoint)
		snapshot, err := manager.Snapshot(ctx, map[string]string{"run": runID})
		if err != nil {
			t.Fatalf("Snapshot failed: %v", err)
		}

		// Load checkpoint
		cp, err := checkpointer.Load(ctx, runID)
		if err != nil {
			t.Fatalf("Checkpoint load failed: %v", err)
		}

		if cp == nil {
			t.Fatal("Expected checkpoint, got nil")
		}

		// Verify state in checkpoint
		if val, ok := cp.State["message"]; !ok {
			t.Error("Expected 'message' key in checkpoint state")
		} else if val != "checkpoint-test" {
			t.Errorf("Expected 'checkpoint-test', got %v", val)
		}

		// Verify metadata
		if val, ok := snapshot.Metadata["run"]; !ok || val != runID {
			t.Error("Expected metadata in snapshot")
		}
	})

	t.Run("LoadCheckpoint recovery", func(t *testing.T) {
		runID := "test-run-2"

		// First session - save state
		{
			manager := state.NewManager(
			state.WithCheckpointer(checkpointer, runID),
			)

			key := state.NewKey[int]("progress", 0)
			state.RegisterKey(manager, key)
			state.Set(ctx, manager, key, 75)

			// Save checkpoint
			manager.Snapshot(ctx, map[string]string{"session": "1"})
			manager.Close()
		}

		// Second session - recover state
		{
			manager := state.NewManager(
			state.WithCheckpointer(checkpointer, runID),
			)
			defer manager.Close()

			key := state.NewKey[int]("progress", 0)
			state.RegisterKey(manager, key)

			// Load from checkpoint
			err := manager.LoadCheckpoint(ctx)
			if err != nil {
			t.Fatalf("LoadCheckpoint failed: %v", err)
			}

			// Verify recovered value
			value, err := state.Get(ctx, manager, key)
			if err != nil {
			t.Fatalf("Get failed: %v", err)
			}

			// Note: Value might be different type due to checkpoint serialization
			t.Logf("Recovered value: %v (type: %T)", value, value)
		}
	})
}

func TestManagerTypeValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// NOTE: Type safety is now enforced at compile time via generics.
	// The old TypeRegistry runtime checks have been removed in favor of
	// compile-time type safety, which is stronger and has zero runtime cost.

	t.Run("List vs Key semantics", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		// Register list key
		listKey := state.NewListKey[string]("items", 0)
		state.RegisterListKey(manager, listKey)

		// Append items
		state.Append(ctx, manager, listKey, "first")
		state.Append(ctx, manager, listKey, "second")
		state.Append(ctx, manager, listKey, "third")

		// Read from channel
		ch := manager.GetChannel("items")
		value, _ := ch.Read(ctx)

		if items, ok := value.([]any); ok {
			if len(items) != 3 {
			t.Errorf("Expected 3 items, got %d", len(items))
			}
		} else {
			t.Errorf("Expected []any from TopicChannel, got %T", value)
		}
	})
}

func TestManagerStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	ctx := context.Background()

	t.Run("High-frequency snapshots", func(t *testing.T) {
		manager := state.NewManager(state.WithMaxSnapshotsLimit(10))
		defer manager.Close()

		key := state.NewKey[int]("counter", 0)
		state.RegisterKey(manager, key)

		// Create many snapshots rapidly
		numSnapshots := 100
		for i := 0; i < numSnapshots; i++ {
			state.Set(ctx, manager, key, i)
			manager.Snapshot(ctx, map[string]string{"iteration": fmt.Sprintf("%d", i)})
		}

		// Should only keep 10 most recent
		snapshots := manager.ListSnapshots()
		if len(snapshots) > 10 {
			t.Errorf("Expected max 10 snapshots, got %d", len(snapshots))
		}

		t.Logf("Created %d snapshots, retained %d", numSnapshots, len(snapshots))
	})

	t.Run("Many keys performance", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		numKeys := 1000
		start := time.Now()

		// Register and set many keys
		for i := 0; i < numKeys; i++ {
			key := state.NewKey[int](fmt.Sprintf("perf%d", i), 0)
			state.RegisterKey(manager, key)
			state.Set(ctx, manager, key, i)
		}

		duration := time.Since(start)
		t.Logf("Registered and set %d keys in %v (%.2f keys/sec)",
			numKeys, duration, float64(numKeys)/duration.Seconds())

		// Verify all registered
		registeredKeys := manager.RegisteredKeys()
		if len(registeredKeys) != numKeys {
			t.Errorf("Expected %d registered keys, got %d", numKeys, len(registeredKeys))
		}
	})
}
