package orderedset

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test basic Add behavior, uniqueness enforcement, and Len/Contains interplay.
func TestSet_AddAndContainsAndLen(t *testing.T) {
	s := New[string]()
	assert.Equal(t, 0, s.Len())

	added := s.Add("a")
	assert.True(t, added)
	assert.True(t, s.Contains("a"))
	assert.Equal(t, 1, s.Len())

	// duplicate
	added = s.Add("a")
	assert.False(t, added)
	assert.Equal(t, 1, s.Len())
}

// Test insertion order and that Values returns a copy preserving insertion order.
func TestSet_ValuesOrderAndCopy(t *testing.T) {
	s := New[int]()
	seq := []int{5, 3, 9, 3, 7, 5, 1}
	expectedUniqueOrder := []int{5, 3, 9, 7, 1}

	for _, v := range seq {
		s.Add(v)
	}

	values := s.Values()
	assert.Equal(t, expectedUniqueOrder, values)

	// mutate returned slice should not affect internal state
	if len(values) > 0 {
		values[0] = 42
	}
	values2 := s.Values()
	assert.Equal(t, expectedUniqueOrder, values2)
}

// Test Remove semantics including removing non-existent values and order maintenance.
func TestSet_Remove(t *testing.T) {
	s := New[string]()
	s.Add("alpha")
	s.Add("beta")
	s.Add("gamma")

	removed := s.Remove("beta")
	assert.True(t, removed)
	assert.False(t, s.Contains("beta"))
	assert.Equal(t, []string{"alpha", "gamma"}, s.Values())

	// remove again -> false
	removed = s.Remove("beta")
	assert.False(t, removed)

	// remove head
	removed = s.Remove("alpha")
	assert.True(t, removed)
	assert.Equal(t, []string{"gamma"}, s.Values())

	// remove tail / last
	removed = s.Remove("gamma")
	assert.True(t, removed)
	assert.Equal(t, 0, s.Len())
	assert.Equal(t, []string{}, s.Values())
}

// Ensure generics work for another comparable type (struct with comparable fields)
type id struct {
	A int
	B string
}

func TestSet_GenericStruct(t *testing.T) {
	s := New[id]()
	v1 := id{A: 1, B: "x"}
	v2 := id{A: 2, B: "y"}
	v3dup := id{A: 1, B: "x"}

	assert.True(t, s.Add(v1))
	assert.True(t, s.Add(v2))
	assert.False(t, s.Add(v3dup)) // duplicate by value

	assert.Equal(t, 2, s.Len())
	assert.Equal(t, []id{v1, v2}, s.Values())
}
