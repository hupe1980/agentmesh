package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSomeAndNone(t *testing.T) {
	o := Some(42)
	assert.True(t, o.IsSet())
	val, ok := o.Get()
	assert.True(t, ok)
	assert.Equal(t, 42, val)

	none := None[int]()
	assert.False(t, none.IsSet())
	val, ok = none.Get()
	assert.False(t, ok)
	assert.Equal(t, 0, val) // zero value
}

func TestOr(t *testing.T) {
	o := None[string]()
	assert.Equal(t, "default", o.Or("default"))

	o.Set("value")
	assert.Equal(t, "value", o.Or("default"))
}

func TestSetAndClear(t *testing.T) {
	var o Opt[string]
	o.Set("hello")
	assert.True(t, o.IsSet())
	assert.Equal(t, "hello", o.Or("fallback"))

	o.Clear()
	assert.False(t, o.IsSet())
	assert.Equal(t, "fallback", o.Or("fallback"))
}

func TestMarshalJSON(t *testing.T) {
	o := None[string]()
	data, err := json.Marshal(o)
	require.NoError(t, err)
	assert.Equal(t, "null", string(data))

	o.Set("hello")
	data, err = json.Marshal(o)
	require.NoError(t, err)
	assert.Equal(t, `"hello"`, string(data))
}

func TestUnmarshalJSON(t *testing.T) {
	var o Opt[string]

	// Unmarshal null
	err := json.Unmarshal([]byte("null"), &o)
	require.NoError(t, err)
	assert.False(t, o.IsSet())

	// Unmarshal value
	err = json.Unmarshal([]byte(`"world"`), &o)
	require.NoError(t, err)
	val, ok := o.Get()
	assert.True(t, ok)
	assert.Equal(t, "world", val)

	// Invalid JSON
	err = json.Unmarshal([]byte("{"), &o)
	assert.Error(t, err)
}

func TestMergeMap(t *testing.T) {
	dst := Some(map[string]int{"a": 1})
	src := Some(map[string]int{"b": 2})

	merged := MergeMap(dst, src)
	val, ok := merged.Get()
	require.True(t, ok)

	assert.Equal(t, map[string]int{
		"a": 1,
		"b": 2,
	}, val)

	// Empty dst
	merged = MergeMap(None[map[string]int](), src)
	val, ok = merged.Get()
	require.True(t, ok)
	assert.Equal(t, map[string]int{"b": 2}, val)
}
