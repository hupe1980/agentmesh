package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKey(t *testing.T) {
	t.Run("create key with default value", func(t *testing.T) {
		key := NewKey[int]("counter", 42)
		
		assert.Equal(t, "counter", key.Name())
		assert.Equal(t, 42, key.Zero())
	})

	t.Run("create key with string", func(t *testing.T) {
		key := NewKey[string]("name", "default")
		
		assert.Equal(t, "name", key.Name())
		assert.Equal(t, "default", key.Zero())
	})

	t.Run("create key with nil default", func(t *testing.T) {
		key := NewKey[*int]("ptr", nil)
		
		assert.Equal(t, "ptr", key.Name())
		assert.Nil(t, key.Zero())
	})

	t.Run("create key with zero struct", func(t *testing.T) {
		type Point struct {
			X, Y int
		}
		key := NewKey[Point]("point", Point{})
		
		assert.Equal(t, "point", key.Name())
		assert.Equal(t, Point{}, key.Zero())
	})
}

func TestNewListKey(t *testing.T) {
	t.Run("create list key unbounded", func(t *testing.T) {
		key := NewListKey[int]("numbers", 0)
		
		assert.Equal(t, "numbers", key.Name())
		assert.Equal(t, 0, key.MaxSize())
		assert.Nil(t, key.Zero())
	})

	t.Run("create list key with max size", func(t *testing.T) {
		key := NewListKey[string]("items", 100)
		
		assert.Equal(t, "items", key.Name())
		assert.Equal(t, 100, key.MaxSize())
	})

	t.Run("list key has embedded Key", func(t *testing.T) {
		key := NewListKey[int]("numbers", 10)
		
		// Should be able to access Key methods
		assert.Equal(t, "numbers", key.Key.Name())
		assert.Nil(t, key.Key.Zero())
	})
}

func TestKeyTypeUniqueness(t *testing.T) {
	t.Run("different names create different keys", func(t *testing.T) {
		key1 := NewKey[int]("counter1", 0)
		key2 := NewKey[int]("counter2", 0)
		
		assert.NotEqual(t, key1.Name(), key2.Name())
	})

	t.Run("same name but different types", func(t *testing.T) {
		key1 := NewKey[int]("value", 0)
		key2 := NewKey[string]("value", "")
		
		// Same name, different types - registration will fail
		assert.Equal(t, key1.Name(), key2.Name())
	})
}

func TestListKeyMaxSize(t *testing.T) {
	t.Run("zero means unbounded", func(t *testing.T) {
		key := NewListKey[int]("unbounded", 0)
		assert.Equal(t, 0, key.MaxSize())
	})

	t.Run("positive value sets limit", func(t *testing.T) {
		key := NewListKey[int]("limited", 50)
		assert.Equal(t, 50, key.MaxSize())
	})

	t.Run("negative value is allowed but treated as limit", func(t *testing.T) {
		// Note: The implementation doesn't validate this,
		// but negative values could be used for special semantics
		key := NewListKey[int]("special", -1)
		assert.Equal(t, -1, key.MaxSize())
	})
}
