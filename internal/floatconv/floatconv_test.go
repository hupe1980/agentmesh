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

func TestToFloat64(t *testing.T) {
	input := []float32{1.1, 2.2, 3.3, 4.4}
	result := ToFloat64(input)

	assert.Len(t, result, 4)
	assert.InDelta(t, 1.1, result[0], 0.001)
	assert.InDelta(t, 2.2, result[1], 0.001)
	assert.InDelta(t, 3.3, result[2], 0.001)
	assert.InDelta(t, 4.4, result[3], 0.001)
}

func TestToFloat64_Empty(t *testing.T) {
	result := ToFloat64(nil)
	assert.Empty(t, result)

	result = ToFloat64([]float32{})
	assert.Empty(t, result)
}

func TestToFloat64FromAny(t *testing.T) {
	input := []any{1.1, 2.2, 3.3, 4.4}
	result := ToFloat64FromAny(input)

	assert.Len(t, result, 4)
	assert.InDelta(t, 1.1, result[0], 0.001)
	assert.InDelta(t, 2.2, result[1], 0.001)
	assert.InDelta(t, 3.3, result[2], 0.001)
	assert.InDelta(t, 4.4, result[3], 0.001)
}

func TestToFloat64FromAny_MixedTypes(t *testing.T) {
	input := []any{float64(1.5), float32(2.5), int(3), int64(4)}
	result := ToFloat64FromAny(input)

	assert.Len(t, result, 4)
	assert.InDelta(t, 1.5, result[0], 0.001)
	assert.InDelta(t, 2.5, result[1], 0.001)
	assert.InDelta(t, 3.0, result[2], 0.001)
	assert.InDelta(t, 4.0, result[3], 0.001)
}

func TestToFloat64FromAny_Empty(t *testing.T) {
	result := ToFloat64FromAny(nil)
	assert.Empty(t, result)

	result = ToFloat64FromAny([]any{})
	assert.Empty(t, result)
}

func TestToFloat64FromAny_NonNumeric(t *testing.T) {
	input := []any{"string", true, nil}
	result := ToFloat64FromAny(input)

	assert.Len(t, result, 3)
	assert.Equal(t, 0.0, result[0])
	assert.Equal(t, 0.0, result[1])
	assert.Equal(t, 0.0, result[2])
}
