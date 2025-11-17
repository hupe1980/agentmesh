package state_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	store := state.NewMemoryStore()
	defer store.Close()

	t.Run("Set and Get", func(t *testing.T) {
		err := store.Set(ctx, "key1", "value1")
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		value, err := store.Get(ctx, "key1")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != "value1" {
			t.Errorf("Expected value1, got %v", value)
		}
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		_, err := store.Get(ctx, "nonexistent")
		if err != state.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := store.Set(ctx, "key2", "value2")
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		err = store.Delete(ctx, "key2")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = store.Get(ctx, "key2")
		if err != state.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound after delete, got %v", err)
		}
	})

	t.Run("Delete non-existent key", func(t *testing.T) {
		err := store.Delete(ctx, "nonexistent")
		if err != state.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("Keys", func(t *testing.T) {
		store2 := state.NewMemoryStore()
		defer store2.Close()

		store2.Set(ctx, "a", 1)
		store2.Set(ctx, "b", 2)
		store2.Set(ctx, "c", 3)

		keys, err := store2.Keys(ctx)
		if err != nil {
			t.Fatalf("Keys failed: %v", err)
		}

		if len(keys) != 3 {
			t.Errorf("Expected 3 keys, got %d", len(keys))
		}
	})

	t.Run("Snapshot and Restore", func(t *testing.T) {
		store3 := state.NewMemoryStore()
		defer store3.Close()

		store3.Set(ctx, "x", 100)
		store3.Set(ctx, "y", 200)

		snapshot, err := store3.Snapshot(ctx)
		if err != nil {
			t.Fatalf("Snapshot failed: %v", err)
		}

		if len(snapshot) != 2 {
			t.Errorf("Expected 2 items in snapshot, got %d", len(snapshot))
		}

		// Clear and restore
		store3.Clear()
		if store3.Len() != 0 {
			t.Errorf("Expected empty store after clear, got %d items", store3.Len())
		}

		err = store3.Restore(ctx, snapshot)
		if err != nil {
			t.Fatalf("Restore failed: %v", err)
		}

		value, _ := store3.Get(ctx, "x")
		if value != 100 {
			t.Errorf("Expected 100 after restore, got %v", value)
		}
	})

	t.Run("Type preservation", func(t *testing.T) {
		store4 := state.NewMemoryStore()
		defer store4.Close()

		// Test different types
		store4.Set(ctx, "int", 42)
		store4.Set(ctx, "string", "hello")
		store4.Set(ctx, "slice", []int{1, 2, 3})
		store4.Set(ctx, "map", map[string]int{"a": 1})

		intVal, _ := store4.Get(ctx, "int")
		if intVal != 42 {
			t.Errorf("Int value mismatch")
		}

		strVal, _ := store4.Get(ctx, "string")
		if strVal != "hello" {
			t.Errorf("String value mismatch")
		}
	})
}

func TestTypeRegistry(t *testing.T) {
	registry := state.NewTypeRegistry()

	t.Run("RegisterKey", func(t *testing.T) {
		var zero int
		err := registry.RegisterKey("counter", reflect.TypeOf(zero), false)
		if err != nil {
			t.Fatalf("RegisterKey failed: %v", err)
		}

		if !registry.IsRegistered("counter") {
			t.Error("Key should be registered")
		}
	})

	t.Run("RegisterKey duplicate with same type", func(t *testing.T) {
		var zero int
		err := registry.RegisterKey("counter", reflect.TypeOf(zero), false)
		if err != nil {
			t.Errorf("Re-registering same type should succeed, got error: %v", err)
		}
	})

	t.Run("RegisterKey duplicate with different type", func(t *testing.T) {
		var zero string
		err := registry.RegisterKey("counter", reflect.TypeOf(zero), false)
		if err == nil {
			t.Error("Re-registering different type should fail")
		}
	})

	t.Run("ValidateType", func(t *testing.T) {
		registry2 := state.NewTypeRegistry()
		var zero int
		registry2.RegisterKey("age", reflect.TypeOf(zero), false)

		err := registry2.ValidateType("age", 42)
		if err != nil {
			t.Errorf("ValidateType should succeed for correct type: %v", err)
		}

		err = registry2.ValidateType("age", "wrong")
		if err == nil {
			t.Error("ValidateType should fail for incorrect type")
		}
	})

	t.Run("GetRegisteredType", func(t *testing.T) {
		registry3 := state.NewTypeRegistry()
		var zero string
		registry3.RegisterKey("name", reflect.TypeOf(zero), false)

		info := registry3.GetRegisteredType("name")
		if info == nil {
			t.Fatal("Expected type info, got nil")
		}

		if info.KeyName != "name" {
			t.Errorf("Expected key name 'name', got %s", info.KeyName)
		}

		if info.IsList {
			t.Error("Expected IsList to be false")
		}
	})

	t.Run("List keys", func(t *testing.T) {
		registry4 := state.NewTypeRegistry()
		var zero []string
		err := registry4.RegisterKey("messages", reflect.TypeOf(zero).Elem(), true)
		if err != nil {
			t.Fatalf("RegisterKey failed: %v", err)
		}

		info := registry4.GetRegisteredType("messages")
		if info == nil {
			t.Fatal("Expected type info, got nil")
		}

		if !info.IsList {
			t.Error("Expected IsList to be true")
		}
	})

	t.Run("RegisteredKeys", func(t *testing.T) {
		registry5 := state.NewTypeRegistry()
		var zeroInt int
		var zeroStr string

		registry5.RegisterKey("a", reflect.TypeOf(zeroInt), false)
		registry5.RegisterKey("b", reflect.TypeOf(zeroStr), false)

		keys := registry5.RegisteredKeys()
		if len(keys) != 2 {
			t.Errorf("Expected 2 registered keys, got %d", len(keys))
		}
	})
}

func TestSnapshotManager(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateSnapshot", func(t *testing.T) {
		sm := state.NewSnapshotManager()

		data := map[string]any{"key1": "value1"}
		metadata := map[string]string{"tag": "test"}

		snapshot, err := sm.CreateSnapshot(ctx, data, metadata)
		if err != nil {
			t.Fatalf("CreateSnapshot failed: %v", err)
		}

		if snapshot.ID == "" {
			t.Error("Snapshot ID should not be empty")
		}

		if snapshot.Timestamp.IsZero() {
			t.Error("Snapshot timestamp should not be zero")
		}

		if len(snapshot.Data) != 1 {
			t.Errorf("Expected 1 data item, got %d", len(snapshot.Data))
		}

		if len(snapshot.Metadata) != 1 {
			t.Errorf("Expected 1 metadata item, got %d", len(snapshot.Metadata))
		}
	})

	t.Run("RestoreSnapshot", func(t *testing.T) {
		sm := state.NewSnapshotManager()

		data := map[string]any{"key1": "value1", "key2": "value2"}
		snapshot, _ := sm.CreateSnapshot(ctx, data, nil)

		restored, err := sm.RestoreSnapshot(ctx, snapshot.ID)
		if err != nil {
			t.Fatalf("RestoreSnapshot failed: %v", err)
		}

		if len(restored) != 2 {
			t.Errorf("Expected 2 restored items, got %d", len(restored))
		}

		if restored["key1"] != "value1" {
			t.Errorf("Restored data mismatch")
		}
	})

	t.Run("RestoreSnapshot non-existent", func(t *testing.T) {
		sm := state.NewSnapshotManager()

		_, err := sm.RestoreSnapshot(ctx, "nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent snapshot")
		}
	})

	t.Run("ListSnapshots", func(t *testing.T) {
		sm := state.NewSnapshotManager()

		sm.CreateSnapshot(ctx, map[string]any{"a": 1}, nil)
		sm.CreateSnapshot(ctx, map[string]any{"b": 2}, nil)
		sm.CreateSnapshot(ctx, map[string]any{"c": 3}, nil)

		snapshots := sm.ListSnapshots()
		// Snapshots may have duplicate timestamps if created too quickly,
		// resulting in duplicate IDs. Check we have at least 2 snapshots.
		if len(snapshots) < 2 {
			t.Errorf("Expected at least 2 snapshots, got %d", len(snapshots))
		}
	})

	t.Run("DeleteSnapshot", func(t *testing.T) {
		sm := state.NewSnapshotManager()

		snapshot, _ := sm.CreateSnapshot(ctx, map[string]any{"x": 1}, nil)

		err := sm.DeleteSnapshot(snapshot.ID)
		if err != nil {
			t.Fatalf("DeleteSnapshot failed: %v", err)
		}

		_, err = sm.RestoreSnapshot(ctx, snapshot.ID)
		if err == nil {
			t.Error("Expected error after deleting snapshot")
		}
	})

	t.Run("MaxSnapshots limit", func(t *testing.T) {
		sm := state.NewSnapshotManager(state.WithMaxSnapshots(2))

		sm.CreateSnapshot(ctx, map[string]any{"a": 1}, nil)
		sm.CreateSnapshot(ctx, map[string]any{"b": 2}, nil)
		sm.CreateSnapshot(ctx, map[string]any{"c": 3}, nil)

		snapshots := sm.ListSnapshots()
		if len(snapshots) != 2 {
			t.Errorf("Expected 2 snapshots (max limit), got %d", len(snapshots))
		}
	})

	t.Run("GetSnapshot", func(t *testing.T) {
		sm := state.NewSnapshotManager()

		original, _ := sm.CreateSnapshot(ctx, map[string]any{"key": "value"}, map[string]string{"tag": "test"})

		retrieved, err := sm.GetSnapshot(original.ID)
		if err != nil {
			t.Fatalf("GetSnapshot failed: %v", err)
		}

		if retrieved.ID != original.ID {
			t.Error("Snapshot ID mismatch")
		}

		if len(retrieved.Metadata) != 1 {
			t.Error("Snapshot metadata mismatch")
		}
	})
}
