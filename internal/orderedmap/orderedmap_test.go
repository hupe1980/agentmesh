package orderedmap

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderedMap_SetAndGetPreservesOrder(t *testing.T) {
	m := New[string, int]()
	m.Set("a", 1)
	m.Set("b", 2)
	m.Set("c", 3)

	// Overwrite existing key (should not change order)
	m.Set("b", 20)

	keys := m.Keys()
	values := m.Values()

	require.Equal(t, []string{"a", "b", "c"}, keys, "key order should be preserved")
	require.Equal(t, []int{1, 20, 3}, values, "values should reflect update while preserving order")

	v, ok := m.Get("b")
	assert.True(t, ok)
	assert.Equal(t, 20, v)
}

func TestOrderedMap_Delete(t *testing.T) {
	m := New[int, string]()
	for i, k := range []int{10, 20, 30, 40} {
		m.Set(k, string(rune('a'+i)))
	}

	m.Delete(20)
	m.Delete(999) // no-op
	m.Delete(40)

	assert.Equal(t, []int{10, 30}, m.Keys())
	assert.Equal(t, []string{"a", "c"}, m.Values())
	assert.Equal(t, 2, m.Len())

	_, ok := m.Get(20)
	assert.False(t, ok)
}

func TestOrderedMap_Clear(t *testing.T) {
	m := New[string, string]()
	m.Set("x", "1")
	m.Set("y", "2")
	m.Clear()

	assert.Equal(t, 0, m.Len())
	assert.Empty(t, m.Keys())
	assert.Empty(t, m.Values())
}

func TestOrderedMap_EmptyBehavior(t *testing.T) {
	m := New[string, int]()
	_, ok := m.Get("missing")
	assert.False(t, ok)
	assert.Equal(t, 0, m.Len())
	assert.Empty(t, m.Keys())
	assert.Empty(t, m.Values())
}

func TestOrderedMap_ConcurrentAccess(t *testing.T) {
	m := New[int, int]()
	var wg sync.WaitGroup

	// Write some initial data
	for i := 0; i < 50; i++ {
		m.Set(i, i*i)
	}

	// Concurrent readers and writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%3 == 0 {
				m.Set(id, id+1000)
			} else if id%5 == 0 {
				m.Delete(id)
			} else {
				_, _ = m.Get(id)
			}
		}(i)
	}

	wg.Wait()

	// Basic invariants: no panic, Len matches unique keys in map, Keys length equals Values length
	keys := m.Keys()
	values := m.Values()
	assert.Equal(t, len(keys), len(values))
	assert.Equal(t, len(keys), m.Len())
}
