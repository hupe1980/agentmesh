package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/hupe1980/agentmesh/pkg/channel"
	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestChannelRegistry(t *testing.T) {
	ctx := context.Background()

	t.Run("GetOrCreateChannel", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		ch := registry.GetOrCreateChannel("test")
		if ch == nil {
			t.Fatal("Expected channel, got nil")
		}

		// Getting same channel twice should return the same instance
		ch2 := registry.GetOrCreateChannel("test")
		if ch != ch2 {
			t.Error("Expected same channel instance")
		}
	})

	t.Run("SetChannelBehavior", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		err := registry.SetChannelBehavior("topic1", state.TopicBehavior)
		if err != nil {
			t.Fatalf("SetChannelBehavior failed: %v", err)
		}

		meta := registry.GetChannelMetadata("topic1")
		if meta == nil {
			t.Fatal("Expected metadata, got nil")
		}

		if meta.Behavior != state.TopicBehavior {
			t.Errorf("Expected TopicBehavior, got %v", meta.Behavior)
		}
	})

	t.Run("RegisterChannel", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		ch := channel.NewLastValueChannel("custom")
		err := registry.RegisterChannel("custom", ch, state.LastValueBehavior)
		if err != nil {
			t.Fatalf("RegisterChannel failed: %v", err)
		}

		retrieved := registry.GetChannel("custom")
		if retrieved != ch {
			t.Error("Retrieved channel doesn't match registered channel")
		}
	})

	t.Run("RegisterChannel duplicate", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		ch1 := channel.NewLastValueChannel("dup")
		registry.RegisterChannel("dup", ch1, state.LastValueBehavior)

		ch2 := channel.NewLastValueChannel("dup2")
		err := registry.RegisterChannel("dup", ch2, state.LastValueBehavior)
		if err == nil {
			t.Error("Expected error when registering duplicate channel")
		}
	})

	t.Run("WriteValue and GetChannelValue", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		ch := registry.GetOrCreateChannel("data")
		if ch == nil {
			t.Fatal("Failed to create channel")
		}

		err := registry.WriteValue(ctx, "data", "hello")
		if err != nil {
			t.Fatalf("WriteValue failed: %v", err)
		}

		value, err := registry.GetChannelValue(ctx, "data")
		if err != nil {
			t.Fatalf("GetChannelValue failed: %v", err)
		}

		if value != "hello" {
			t.Errorf("Expected 'hello', got %v", value)
		}
	})

	t.Run("Channels", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		registry.GetOrCreateChannel("a")
		registry.GetOrCreateChannel("b")
		registry.GetOrCreateChannel("c")

		channels := registry.Channels()
		if len(channels) != 3 {
			t.Errorf("Expected 3 channels, got %d", len(channels))
		}
	})

	t.Run("Snapshot and Restore", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		// Write some values
		registry.GetOrCreateChannel("x")
		registry.WriteValue(ctx, "x", 100)
		registry.GetOrCreateChannel("y")
		registry.WriteValue(ctx, "y", 200)

		// Create snapshot
		snapshot, err := registry.Snapshot(ctx)
		if err != nil {
			t.Fatalf("Snapshot failed: %v", err)
		}

		if len(snapshot) != 2 {
			t.Errorf("Expected 2 items in snapshot, got %d", len(snapshot))
		}

		// Clear and restore
		registry.Clear()
		if registry.Len() != 0 {
			t.Errorf("Expected empty registry after clear, got %d", registry.Len())
		}

		err = registry.Restore(ctx, snapshot)
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		// Verify restored value
		value, _ := registry.GetChannelValue(ctx, "x")
		if value != 100 {
			t.Errorf("Expected 100 after restore, got %v", value)
		}
	})

	t.Run("GetChannel non-existent", func(t *testing.T) {
		registry := state.NewChannelRegistry()

		ch := registry.GetChannel("nonexistent")
		if ch != nil {
			t.Error("Expected nil for non-existent channel")
		}
	})
}

func TestManager(t *testing.T) {
	ctx := context.Background()

	t.Run("NewManager with defaults", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		if manager == nil {
			t.Fatal("Expected manager, got nil")
		}

		keys := manager.RegisteredKeys()
		if len(keys) != 0 {
			t.Errorf("Expected 0 registered keys, got %d", len(keys))
		}
	})

	t.Run("Register and Get/Set with Key", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		counterKey := state.NewKey[int]("counter", 0)

		// Register key
		err := state.RegisterKey(manager, counterKey)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Set value
		err = state.SetInManager(ctx, manager, counterKey, 42)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Get value
		value, err := state.GetFromManager(ctx, manager, counterKey)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != 42 {
			t.Errorf("Expected 42, got %d", value)
		}
	})

	t.Run("Register and Append with ListKey", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		messagesKey := state.NewListKey[string]("messages", 100)

		// Register list key
		err := state.RegisterListKey(manager, messagesKey)
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		// Append values
		err = state.AppendToManager(ctx, manager, messagesKey, "first")
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}

		err = state.AppendToManager(ctx, manager, messagesKey, "second")
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}

		// Get list from channel (returns []any from TopicChannel)
		ch := manager.GetChannel("messages")
		value, err := ch.Read(ctx)
		if err != nil {
			t.Fatalf("Channel read failed: %v", err)
		}

		// TopicChannel returns []any, check count
		if messages, ok := value.([]any); ok {
			if len(messages) != 2 {
				t.Errorf("Expected 2 messages, got %d", len(messages))
			}
			// Verify values
			if messages[0] != "first" {
				t.Errorf("Expected 'first', got %v", messages[0])
			}
			if messages[1] != "second" {
				t.Errorf("Expected 'second', got %v", messages[1])
			}
		} else {
			t.Errorf("Expected []any from TopicChannel, got %T", value)
		}
	})

	t.Run("GetChannel", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		key := state.NewKey[string]("name", "")
		state.RegisterKey(manager, key)

		ch := manager.GetChannel("name")
		if ch == nil {
			t.Error("Expected channel, got nil")
		}

		// Direct channel write
		ch.Write(ctx, "direct")

		// Verify via Get
		value, _ := state.GetFromManager(ctx, manager, key)
		if value != "direct" {
			t.Errorf("Expected 'direct', got %s", value)
		}
	})

	t.Run("Snapshot and Restore", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		// Setup state
		key1 := state.NewKey[int]("x", 0)
		key2 := state.NewKey[string]("y", "")
		state.RegisterKey(manager, key1)
		state.RegisterKey(manager, key2)

		state.SetInManager(ctx, manager, key1, 100)
		state.SetInManager(ctx, manager, key2, "hello")

		// Create snapshot
		snapshot, err := manager.Snapshot(ctx, map[string]string{"tag": "test"})
		if err != nil {
			t.Fatalf("Snapshot failed: %v", err)
		}

		// Modify state
		state.SetInManager(ctx, manager, key1, 999)

		// Restore
		err = manager.Restore(ctx, snapshot.ID)
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		// Verify restored value
		value, _ := state.GetFromManager(ctx, manager, key1)
		if value != 100 {
			t.Errorf("Expected 100 after restore, got %d", value)
		}
	})

	t.Run("ListSnapshots", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		key := state.NewKey[int]("counter", 0)
		state.RegisterKey(manager, key)

		// Create multiple snapshots
		state.SetInManager(ctx, manager, key, 1)
		manager.Snapshot(ctx, nil)

		state.SetInManager(ctx, manager, key, 2)
		manager.Snapshot(ctx, nil)

		snapshots := manager.ListSnapshots()
		if len(snapshots) < 1 {
			t.Errorf("Expected at least 1 snapshot, got %d", len(snapshots))
		}
	})

	t.Run("DeleteSnapshot", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		key := state.NewKey[int]("test", 0)
		state.RegisterKey(manager, key)
		state.SetInManager(ctx, manager, key, 42)

		snapshot, _ := manager.Snapshot(ctx, nil)

		err := manager.DeleteSnapshot(snapshot.ID)
		if err != nil {
			t.Fatalf("DeleteSnapshot failed: %v", err)
		}

		err = manager.Restore(ctx, snapshot.ID)
		if err == nil {
			t.Error("Expected error restoring deleted snapshot")
		}
	})

	t.Run("RegisteredKeys", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		key1 := state.NewKey[int]("a", 0)
		key2 := state.NewKey[string]("b", "")
		key3 := state.NewListKey[float64]("c", 0)

		state.RegisterKey(manager, key1)
		state.RegisterKey(manager, key2)
		state.RegisterListKey(manager, key3)

		keys := manager.RegisteredKeys()
		if len(keys) != 3 {
			t.Errorf("Expected 3 registered keys, got %d", len(keys))
		}
	})

	t.Run("WithStore option", func(t *testing.T) {
		customStore := state.NewMemoryStore()
		manager := state.NewManager(state.WithStore(customStore))
		defer manager.Close()

		key := state.NewKey[int]("stored", 0)
		state.RegisterKey(manager, key)
		state.SetInManager(ctx, manager, key, 123)

		// Verify value is in custom store
		value, err := customStore.Get(ctx, "stored")
		if err != nil {
			t.Fatalf("Store Get failed: %v", err)
		}

		if value != 123 {
			t.Errorf("Expected 123 in store, got %v", value)
		}
	})

	t.Run("Type safety validation", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		key := state.NewKey[int]("number", 0)
		state.RegisterKey(manager, key)

		// This should fail at runtime (type validation)
		// We can't easily test this with generics, but the TypeRegistry
		// will catch type mismatches if someone bypasses the type system
	})

	t.Run("Concurrent access", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		key := state.NewKey[int]("counter", 0)
		state.RegisterKey(manager, key)
		state.SetInManager(ctx, manager, key, 0)

		done := make(chan bool)

		// Concurrent writes
		for i := 0; i < 10; i++ {
			go func(val int) {
				state.SetInManager(ctx, manager, key, val)
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Should not panic or race
		value, err := state.GetFromManager(ctx, manager, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		// Value should be between 0-9
		if value < 0 || value > 9 {
			t.Errorf("Unexpected value after concurrent writes: %d", value)
		}
	})

	t.Run("Get with default value", func(t *testing.T) {
		manager := state.NewManager()
		defer manager.Close()

		key := state.NewKey[string]("missing", "default")
		state.RegisterKey(manager, key)

		// Get without setting should return default
		value, err := state.GetFromManager(ctx, manager, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != "default" {
			t.Errorf("Expected default value, got %s", value)
		}
	})
}

func TestManagerWithMaxSnapshots(t *testing.T) {
	ctx := context.Background()

	manager := state.NewManager(state.WithMaxSnapshotsLimit(2))
	defer manager.Close()

	key := state.NewKey[int]("counter", 0)
	state.RegisterKey(manager, key)

	// Create 3 snapshots
	state.SetInManager(ctx, manager, key, 1)
	manager.Snapshot(ctx, nil)

	time.Sleep(time.Millisecond) // Ensure different timestamps

	state.SetInManager(ctx, manager, key, 2)
	manager.Snapshot(ctx, nil)

	time.Sleep(time.Millisecond)

	state.SetInManager(ctx, manager, key, 3)
	manager.Snapshot(ctx, nil)

	// Should only keep 2 most recent
	snapshots := manager.ListSnapshots()
	if len(snapshots) > 2 {
		t.Errorf("Expected max 2 snapshots with limit, got %d", len(snapshots))
	}
}
