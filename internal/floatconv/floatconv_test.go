package floatconv

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToFloat32(t *testing.T) {
	input := []float64{1.1, 2.2, 3.3, 4.4}
	result := ToFloat32(input)

	assert.Len(t, result, 4)
	assert.InDelta(t, float32(1.1), result[0], 0.001)
	assert.InDelta(t, float32(2.2), result[1], 0.001)
	assert.InDelta(t, float32(3.3), result[2], 0.001)
	assert.InDelta(t, float32(4.4), result[3], 0.001)
}

func TestToFloat32_Empty(t *testing.T) {
	result := ToFloat32(nil)
	assert.Empty(t, result)

	result = ToFloat32([]float64{})
	assert.Empty(t, result)
}

func TestToFloat32FromAny(t *testing.T) {
	input := []any{1.1, 2.2, 3.3, 4.4}
	result := ToFloat32FromAny(input)

	assert.Len(t, result, 4)
	assert.InDelta(t, float32(1.1), result[0], 0.001)
	assert.InDelta(t, float32(2.2), result[1], 0.001)
	assert.InDelta(t, float32(3.3), result[2], 0.001)
	assert.InDelta(t, float32(4.4), result[3], 0.001)
}

func TestToFloat32FromAny_MixedTypes(t *testing.T) {
	input := []any{float64(1.5), float32(2.5), int(3), int64(4)}
	result := ToFloat32FromAny(input)

	assert.Len(t, result, 4)
	assert.InDelta(t, float32(1.5), result[0], 0.001)
	assert.InDelta(t, float32(2.5), result[1], 0.001)
	assert.InDelta(t, float32(3.0), result[2], 0.001)
	assert.InDelta(t, float32(4.0), result[3], 0.001)
}

func TestToFloat32FromAny_Empty(t *testing.T) {
	result := ToFloat32FromAny(nil)
	assert.Empty(t, result)

	result = ToFloat32FromAny([]any{})
	assert.Empty(t, result)
}

func TestToFloat32FromAny_NonNumeric(t *testing.T) {
	input := []any{"string", true, nil}
	result := ToFloat32FromAny(input)

	assert.Len(t, result, 3)
	assert.Equal(t, float32(0.0), result[0])
	assert.Equal(t, float32(0.0), result[1])
	assert.Equal(t, float32(0.0), result[2])
}
