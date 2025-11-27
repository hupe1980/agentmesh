package state_test

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/state"
)

func TestManager_Freeze(t *testing.T) {
	t.Run("can register keys before freeze", func(t *testing.T) {
		mgr := state.NewManager()
		key := state.NewKey("test", 0)

		if err := state.RegisterKey(mgr, key); err != nil {
			t.Fatalf("Expected registration to succeed before freeze: %v", err)
		}
	})

	t.Run("cannot register keys after freeze", func(t *testing.T) {
		mgr := state.NewManager()
		mgr.Freeze()

		key := state.NewKey("test", 0)
		err := state.RegisterKey(mgr, key)
		if err == nil {
			t.Fatal("Expected error when registering key after freeze")
		}
		if err.Error() != `cannot register key "test": manager is frozen after compilation` {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("cannot register list keys after freeze", func(t *testing.T) {
		mgr := state.NewManager()
		mgr.Freeze()

		key := state.NewListKey[string]("messages", 10)
		err := state.RegisterListKey(mgr, key)
		if err == nil {
			t.Fatal("Expected error when registering list key after freeze")
		}
		if err.Error() != `cannot register list key "messages": manager is frozen after compilation` {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("cannot register managed values after freeze", func(t *testing.T) {
		mgr := state.NewManager()
		mgr.Freeze()

		mv := state.NewManagedValueWithDefault("config", "default")
		err := state.RegisterManagedValue(mgr, mv)
		if err == nil {
			t.Fatal("Expected error when registering managed value after freeze")
		}
		if err.Error() != `cannot register managed value "config": manager is frozen after compilation` {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("IsFrozen returns correct state", func(t *testing.T) {
		mgr := state.NewManager()

		if mgr.IsFrozen() {
			t.Error("Expected manager to not be frozen initially")
		}

		mgr.Freeze()

		if !mgr.IsFrozen() {
			t.Error("Expected manager to be frozen after calling Freeze()")
		}
	})

	t.Run("manager freezes channel registry too", func(t *testing.T) {
		mgr := state.NewManager()

		if mgr.IsFrozen() {
			t.Error("Expected manager to not be frozen initially")
		}

		mgr.Freeze()

		if !mgr.IsFrozen() {
			t.Error("Expected manager to be frozen after calling Freeze()")
		}

		// Verify that registering keys fails after freeze
		key := state.NewKey("late_key", 0)
		err := state.RegisterKey(mgr, key)
		if err == nil {
			t.Fatal("Expected error when registering key after manager is frozen")
		}
	})
}
