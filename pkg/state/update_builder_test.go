package state_test

import (
	"testing"

	"github.com/hupe1980/agentmesh/pkg/state"
	"github.com/stretchr/testify/require"
)

func TestUpdateBuilder_Set(t *testing.T) {
	builder := state.NewUpdateBuilder()
	key := state.NewKey[int]("counter", 0)

	state.SetUpdate(builder, key, 42)
	updates, err := builder.Build()

	require.NoError(t, err)
	require.Equal(t, 42, updates["counter"])
}

func TestUpdateBuilder_Append(t *testing.T) {
	builder := state.NewUpdateBuilder()
	key := state.NewListKey[string]("items", 10)

	state.AppendUpdate(builder, key, "a", "b", "c")
	updates, err := builder.Build()

	require.NoError(t, err)
	slice := updates["items"].(state.SliceOf[string])
	require.Len(t, slice, 3)
	require.Equal(t, "a", slice[0])
	require.Equal(t, "b", slice[1])
	require.Equal(t, "c", slice[2])
}

func TestUpdateBuilder_Chaining(t *testing.T) {
	builder := state.NewUpdateBuilder()
	counterKey := state.NewKey[int]("counter", 0)
	itemsKey := state.NewListKey[string]("items", 10)

	state.SetUpdate(builder, counterKey, 42)
	state.AppendUpdate(builder, itemsKey, "x", "y")
	updates, err := builder.Build()

	require.NoError(t, err)
	require.Equal(t, 42, updates["counter"])
	require.Len(t, updates["items"].(state.SliceOf[string]), 2)
}

func TestUpdateBuilder_DuplicateKey(t *testing.T) {
	builder := state.NewUpdateBuilder()
	key := state.NewKey[int]("counter", 0)

	state.SetUpdate(builder, key, 1)
	state.SetUpdate(builder, key, 2) // Duplicate

	_, err := builder.Build()
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
}

func TestUpdateBuilder_SetRaw(t *testing.T) {
	builder := state.NewUpdateBuilder()

	builder.SetRaw("dynamic", "value")
	updates, err := builder.Build()

	require.NoError(t, err)
	require.Equal(t, "value", updates["dynamic"])
}

func TestUpdateBuilder_Delete(t *testing.T) {
	builder := state.NewUpdateBuilder()

	builder.Delete("old_key")
	updates, err := builder.Build()

	require.NoError(t, err)
	require.Contains(t, updates, "old_key")
}

func TestUpdateBuilder_IsEmpty(t *testing.T) {
	builder := state.NewUpdateBuilder()
	require.True(t, builder.IsEmpty())

	key := state.NewKey[int]("counter", 0)
	state.SetUpdate(builder, key, 1)
	require.False(t, builder.IsEmpty())
}

func TestUpdateBuilder_MustBuild(t *testing.T) {
	builder := state.NewUpdateBuilder()
	key := state.NewKey[int]("counter", 0)
	state.SetUpdate(builder, key, 42)

	updates := builder.MustBuild()
	require.Equal(t, 42, updates["counter"])
}

func TestUpdateBuilder_MustBuild_Panics(t *testing.T) {
	builder := state.NewUpdateBuilder()
	key := state.NewKey[int]("counter", 0)
	state.SetUpdate(builder, key, 1)
	state.SetUpdate(builder, key, 2) // Duplicate

	require.Panics(t, func() {
		builder.MustBuild()
	})
}

func TestUpdateBuilder_AppendEmpty(t *testing.T) {
	builder := state.NewUpdateBuilder()
	key := state.NewListKey[string]("items", 10)

	// Appending nothing should be a no-op
	state.AppendUpdate(builder, key)
	require.True(t, builder.IsEmpty())
}
