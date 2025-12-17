package integration_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"testing"

	"github.com/hupe1980/agentmesh/pkg/graph"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GobSerializableState represents a state that can be serialized with gob
type GobSerializableState struct {
	Counter int
	Text    string
	Items   []string
}

func init() {
	// Register types for gob serialization
	gob.Register(GobSerializableState{})
	gob.Register([]string{})
}

// TestGobCodec_BasicSerialization tests basic gob serialization/deserialization.
func TestGobCodec_BasicSerialization(t *testing.T) {
	t.Parallel()

	original := GobSerializableState{
		Counter: 42,
		Text:    "hello world",
		Items:   []string{"a", "b", "c"},
	}

	// Encode
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(original)
	require.NoError(t, err)

	// Decode
	var decoded GobSerializableState
	dec := gob.NewDecoder(&buf)
	err = dec.Decode(&decoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)
}

// TestGobCodec_SliceSerialization tests gob serialization of slices.
func TestGobCodec_SliceSerialization(t *testing.T) {
	t.Parallel()

	// Test slice of strings
	original := []string{"hello", "world", "test"}

	// Encode
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(original)
	require.NoError(t, err)

	// Decode
	var decoded []string
	dec := gob.NewDecoder(&buf)
	err = dec.Decode(&decoded)
	require.NoError(t, err)

	require.Len(t, decoded, 3)
	assert.Equal(t, original, decoded)
}

// TestGobCodec_GraphStateIntegration tests gob encoding with graph state.
func TestGobCodec_GraphStateIntegration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stateKey := graph.NewKey[GobSerializableState]("state")

	var finalState GobSerializableState

	g := graph.New(stateKey)

	g.Node("increment", func(ctx context.Context, scope graph.Scope) (*graph.Command, error) {
		state := graph.Get(scope, stateKey)
		state.Counter++
		state.Items = append(state.Items, "step")
		finalState = state
		return graph.Set(stateKey, state).End()
	}, graph.END)

	g.Start("increment")

	compiled, err := g.Build()
	require.NoError(t, err)

	input := GobSerializableState{
		Counter: 10,
		Text:    "test",
		Items:   []string{"initial"},
	}

	for _, err := range compiled.Run(ctx, nil, graph.WithInitialValue(stateKey, input)) {
		require.NoError(t, err)
	}

	assert.Equal(t, 11, finalState.Counter)
	assert.Contains(t, finalState.Items, "step")
}

// TestGobCodec_MapSerialization tests gob serialization of maps.
func TestGobCodec_MapSerialization(t *testing.T) {
	t.Parallel()

	original := map[string]int{
		"one":   1,
		"two":   2,
		"three": 3,
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(original)
	require.NoError(t, err)

	var decoded map[string]int
	dec := gob.NewDecoder(&buf)
	err = dec.Decode(&decoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)
}

// TestGobCodec_NestedStructs tests gob serialization of nested structures.
func TestGobCodec_NestedStructs(t *testing.T) {
	t.Parallel()

	type Inner struct {
		Value int
	}
	type Outer struct {
		Name  string
		Inner Inner
	}

	original := Outer{
		Name:  "test",
		Inner: Inner{Value: 42},
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(original)
	require.NoError(t, err)

	var decoded Outer
	dec := gob.NewDecoder(&buf)
	err = dec.Decode(&decoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded)
}

// TestGobCodec_EmptyValues tests gob serialization of empty/zero values.
func TestGobCodec_EmptyValues(t *testing.T) {
	t.Parallel()

	original := GobSerializableState{
		Counter: 0,
		Text:    "",
		Items:   nil,
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(original)
	require.NoError(t, err)

	var decoded GobSerializableState
	dec := gob.NewDecoder(&buf)
	err = dec.Decode(&decoded)
	require.NoError(t, err)

	assert.Equal(t, original.Counter, decoded.Counter)
	assert.Equal(t, original.Text, decoded.Text)
}

// TestGobCodec_LargeData tests gob serialization with large data.
func TestGobCodec_LargeData(t *testing.T) {
	t.Parallel()

	// Create large slice
	items := make([]string, 1000)
	for i := range items {
		items[i] = "item"
	}

	original := GobSerializableState{
		Counter: 999999,
		Text:    string(make([]byte, 10000)),
		Items:   items,
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(original)
	require.NoError(t, err)

	var decoded GobSerializableState
	dec := gob.NewDecoder(&buf)
	err = dec.Decode(&decoded)
	require.NoError(t, err)

	assert.Equal(t, len(original.Items), len(decoded.Items))
	assert.Equal(t, len(original.Text), len(decoded.Text))
}
