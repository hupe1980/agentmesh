package state

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewState(t *testing.T) {
	s := NewState()
	require.NotNil(t, s)
	assert.NotNil(t, s.data)
	assert.NotNil(t, s.registered)
	assert.NotNil(t, s.listKeys)
	assert.Equal(t, uint64(0), s.version)
}

func TestRegister(t *testing.T) {
	t.Run("register new key", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		err := Register(s, key)
		require.NoError(t, err)

		// Verify registration
		assert.Contains(t, s.registered, "counter")
		assert.Equal(t, 0, Get(s, key))
	})

	t.Run("register same key twice", func(t *testing.T) {
		s := NewState()
		key := NewKey[string]("name", "")

		err := Register(s, key)
		require.NoError(t, err)

		// Should succeed (idempotent)
		err = Register(s, key)
		require.NoError(t, err)
	})

	t.Run("register same key with different type", func(t *testing.T) {
		s := NewState()
		key1 := NewKey[int]("value", 0)
		key2 := NewKey[string]("value", "")

		err := Register(s, key1)
		require.NoError(t, err)

		// Should fail
		err = Register(s, key2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already registered with different type")
	})
}

func TestRegisterList(t *testing.T) {
	t.Run("register list key", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		err := RegisterList(s, key)
		require.NoError(t, err)

		// Verify registration
		assert.Contains(t, s.registered, "numbers")
		assert.Contains(t, s.listKeys, "numbers")
		assert.Equal(t, 0, s.listKeys["numbers"])
	})

	t.Run("register list key with max size", func(t *testing.T) {
		s := NewState()
		key := NewListKey[string]("items", 10)

		err := RegisterList(s, key)
		require.NoError(t, err)

		assert.Equal(t, 10, s.listKeys["items"])
	})
}

func TestGetSet(t *testing.T) {
	t.Run("get default value for unregistered key", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 42)

		// Should return default value
		val := Get(s, key)
		assert.Equal(t, 42, val)
	})

	t.Run("set and get value", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		err := Register(s, key)
		require.NoError(t, err)

		ctx := context.Background()
		err = Set(ctx, s, key, 100)
		require.NoError(t, err)

		val := Get(s, key)
		assert.Equal(t, 100, val)
	})

	t.Run("set updates version", func(t *testing.T) {
		s := NewState()
		key := NewKey[string]("name", "")

		err := Register(s, key)
		require.NoError(t, err)

		initialVersion := s.version

		ctx := context.Background()
		err = Set(ctx, s, key, "Alice")
		require.NoError(t, err)

		assert.Greater(t, s.version, initialVersion)
	})

	t.Run("set unregistered key fails", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("unknown", 0)

		ctx := context.Background()
		err := Set(ctx, s, key, 100)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKeyNotRegistered)
	})

	t.Run("set with cancelled context", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		err := Register(s, key)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = Set(ctx, s, key, 100)
		require.Error(t, err)
	})
}

func TestAppend(t *testing.T) {
	t.Run("append to empty list", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		err := RegisterList(s, key)
		require.NoError(t, err)

		ctx := context.Background()
		err = Append(ctx, s, key, 42)
		require.NoError(t, err)

		list := Get(s, key.Key)
		assert.Equal(t, []int{42}, list)
	})

	t.Run("append multiple items", func(t *testing.T) {
		s := NewState()
		key := NewListKey[string]("items", 0)

		err := RegisterList(s, key)
		require.NoError(t, err)

		ctx := context.Background()
		for _, item := range []string{"a", "b", "c"} {
			err = Append(ctx, s, key, item)
			require.NoError(t, err)
		}

		list := Get(s, key.Key)
		assert.Equal(t, []string{"a", "b", "c"}, list)
	})

	t.Run("append with max size truncates", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 3)

		err := RegisterList(s, key)
		require.NoError(t, err)

		ctx := context.Background()
		for i := 1; i <= 5; i++ {
			err = Append(ctx, s, key, i)
			require.NoError(t, err)
		}

		list := Get(s, key.Key)
		assert.Equal(t, []int{3, 4, 5}, list) // Only last 3
	})

	t.Run("append to unregistered key fails", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("unknown", 0)

		ctx := context.Background()
		err := Append(ctx, s, key, 42)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKeyNotRegistered)
	})

	t.Run("append to non-list key fails", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)
		listKey := NewListKey[int]("counter", 0)

		err := Register(s, key)
		require.NoError(t, err)

		ctx := context.Background()
		err = Append(ctx, s, listKey, 42)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKeyNotList)
	})
}

func TestApplyUpdates(t *testing.T) {
	t.Run("apply simple updates", func(t *testing.T) {
		s := NewState()
		key1 := NewKey[int]("counter", 0)
		key2 := NewKey[string]("name", "")

		Register(s, key1)
		Register(s, key2)

		ctx := context.Background()
		updates := Updates{
			"counter": 100,
			"name":    "Alice",
		}

		err := s.ApplyUpdates(ctx, updates)
		require.NoError(t, err)

		assert.Equal(t, 100, Get(s, key1))
		assert.Equal(t, "Alice", Get(s, key2))
	})

	t.Run("apply updates to list key appends", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		RegisterList(s, key)

		ctx := context.Background()

		// Add initial values
		err := s.ApplyUpdates(ctx, Updates{
			"numbers": []int{1, 2, 3},
		})
		require.NoError(t, err)

		// Add more values - should append
		err = s.ApplyUpdates(ctx, Updates{
			"numbers": []int{4, 5},
		})
		require.NoError(t, err)

		list := Get(s, key.Key)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, list)
	})

	t.Run("apply updates to list with max size", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 3)

		RegisterList(s, key)

		ctx := context.Background()

		// Add initial values
		err := s.ApplyUpdates(ctx, Updates{
			"numbers": []int{1, 2},
		})
		require.NoError(t, err)

		// Add more values - should append and truncate
		err = s.ApplyUpdates(ctx, Updates{
			"numbers": []int{3, 4, 5},
		})
		require.NoError(t, err)

		list := Get(s, key.Key)
		assert.Equal(t, []int{3, 4, 5}, list) // Only last 3
	})

	t.Run("apply updates with type mismatch fails", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		ctx := context.Background()
		updates := Updates{
			"counter": "wrong type",
		}

		err := s.ApplyUpdates(ctx, updates)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTypeMismatch)
	})

	t.Run("apply updates with unregistered key fails", func(t *testing.T) {
		s := NewState()

		ctx := context.Background()
		updates := Updates{
			"unknown": 100,
		}

		err := s.ApplyUpdates(ctx, updates)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrKeyNotRegistered)
	})

	t.Run("apply updates is atomic", func(t *testing.T) {
		s := NewState()
		key1 := NewKey[int]("counter", 0)

		Register(s, key1)

		ctx := context.Background()
		updates := Updates{
			"counter": 100,
			"name":    "Alice", // Unregistered
		}

		err := s.ApplyUpdates(ctx, updates)
		require.Error(t, err)

		// First update should not have been applied
		assert.Equal(t, 0, Get(s, key1))
	})

	t.Run("apply updates increments version", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		initialVersion := s.version

		ctx := context.Background()
		updates := Updates{
			"counter": 100,
		}

		err := s.ApplyUpdates(ctx, updates)
		require.NoError(t, err)

		assert.Greater(t, s.version, initialVersion)
	})
}

func TestSnapshot(t *testing.T) {
	t.Run("snapshot creates immutable copy", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		ctx := context.Background()
		Set(ctx, s, key, 100)

		snap := s.Snapshot()
		require.NotNil(t, snap)

		// Modify original state
		Set(ctx, s, key, 200)

		// Snapshot should still have old value
		view := NewReadView(snap)
		val := GetFromView(view, key)
		assert.Equal(t, 100, val)
	})

	t.Run("snapshot captures version", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		ctx := context.Background()
		Set(ctx, s, key, 100)

		snap := s.Snapshot()
		snapVersion := snap.version

		// Modify state
		Set(ctx, s, key, 200)

		// Snapshot version should be unchanged
		assert.Equal(t, snapVersion, snap.version)
		assert.Greater(t, s.version, snapVersion)
	})
}

func TestStream(t *testing.T) {
	t.Run("stream over empty list", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		RegisterList(s, key)

		var items []int
		for item := range Stream(s, key) {
			items = append(items, item)
		}

		assert.Empty(t, items)
	})

	t.Run("stream over list", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		RegisterList(s, key)

		ctx := context.Background()
		for _, n := range []int{1, 2, 3, 4, 5} {
			Append(ctx, s, key, n)
		}

		var items []int
		for item := range Stream(s, key) {
			items = append(items, item)
		}

		assert.Equal(t, []int{1, 2, 3, 4, 5}, items)
	})

	t.Run("stream can break early", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		RegisterList(s, key)

		ctx := context.Background()
		for _, n := range []int{1, 2, 3, 4, 5} {
			Append(ctx, s, key, n)
		}

		var items []int
		for item := range Stream(s, key) {
			items = append(items, item)
			if item == 3 {
				break
			}
		}

		assert.Equal(t, []int{1, 2, 3}, items)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Run("concurrent reads are safe", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		ctx := context.Background()
		Set(ctx, s, key, 100)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				val := Get(s, key)
				assert.Equal(t, 100, val)
			}()
		}

		wg.Wait()
	})

	t.Run("concurrent writes are safe", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		var wg sync.WaitGroup
		ctx := context.Background()

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(val int) {
				defer wg.Done()
				Set(ctx, s, key, val)
			}(i)
		}

		wg.Wait()

		// Should have some value between 0-99
		val := Get(s, key)
		assert.GreaterOrEqual(t, val, 0)
		assert.Less(t, val, 100)
	})

	t.Run("concurrent appends are safe", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		RegisterList(s, key)

		var wg sync.WaitGroup
		ctx := context.Background()

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(val int) {
				defer wg.Done()
				Append(ctx, s, key, val)
			}(i)
		}

		wg.Wait()

		list := Get(s, key.Key)
		assert.Len(t, list, 100)
	})
}

func TestMustSet(t *testing.T) {
	t.Run("must set succeeds", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		ctx := context.Background()
		assert.NotPanics(t, func() {
			MustSet(ctx, s, key, 100)
		})

		assert.Equal(t, 100, Get(s, key))
	})

	t.Run("must set panics on error", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("unregistered", 0)

		ctx := context.Background()
		assert.Panics(t, func() {
			MustSet(ctx, s, key, 100)
		})
	})
}

func TestVersion(t *testing.T) {
	t.Run("version starts at zero", func(t *testing.T) {
		s := NewState()
		assert.Equal(t, uint64(0), s.Version())
	})

	t.Run("version increments on set", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		ctx := context.Background()
		initialVersion := s.Version()

		Set(ctx, s, key, 100)
		assert.Equal(t, initialVersion+1, s.Version())

		Set(ctx, s, key, 200)
		assert.Equal(t, initialVersion+2, s.Version())
	})

	t.Run("version increments on append", func(t *testing.T) {
		s := NewState()
		key := NewListKey[int]("numbers", 0)

		RegisterList(s, key)

		ctx := context.Background()
		initialVersion := s.Version()

		Append(ctx, s, key, 1)
		assert.Equal(t, initialVersion+1, s.Version())
	})

	t.Run("version increments on apply updates", func(t *testing.T) {
		s := NewState()
		key := NewKey[int]("counter", 0)

		Register(s, key)

		ctx := context.Background()
		initialVersion := s.Version()

		s.ApplyUpdates(ctx, Updates{"counter": 100})
		assert.Equal(t, initialVersion+1, s.Version())
	})
}

func TestInterfaceTypes(t *testing.T) {
	t.Run("register and use interface key", func(t *testing.T) {
		s := NewState()
		key := NewKey[any]("data", nil)

		err := Register(s, key)
		require.NoError(t, err)

		ctx := context.Background()

		// Set string value
		err = Set(ctx, s, key, "hello")
		require.NoError(t, err)

		val := Get(s, key)
		assert.Equal(t, "hello", val)

		// Set int value
		err = Set(ctx, s, key, 42)
		require.NoError(t, err)

		val = Get(s, key)
		assert.Equal(t, 42, val)
	})
}

func TestComplexTypes(t *testing.T) {
	type Person struct {
		Name string
		Age  int
	}

	t.Run("struct values", func(t *testing.T) {
		s := NewState()
		key := NewKey[Person]("person", Person{})

		Register(s, key)

		ctx := context.Background()
		person := Person{Name: "Alice", Age: 30}

		err := Set(ctx, s, key, person)
		require.NoError(t, err)

		val := Get(s, key)
		assert.Equal(t, person, val)
	})

	t.Run("pointer values", func(t *testing.T) {
		s := NewState()
		key := NewKey[*Person]("person_ptr", nil)

		Register(s, key)

		ctx := context.Background()
		person := &Person{Name: "Bob", Age: 25}

		err := Set(ctx, s, key, person)
		require.NoError(t, err)

		val := Get(s, key)
		assert.Equal(t, person, val)
	})

	t.Run("list of structs", func(t *testing.T) {
		s := NewState()
		key := NewListKey[Person]("people", 0)

		RegisterList(s, key)

		ctx := context.Background()
		people := []Person{
			{Name: "Alice", Age: 30},
			{Name: "Bob", Age: 25},
		}

		for _, p := range people {
			err := Append(ctx, s, key, p)
			require.NoError(t, err)
		}

		list := Get(s, key.Key)
		assert.Equal(t, people, list)
	})
}
