package graph

import (
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLast_Success(t *testing.T) {
	seq := func(yield func(int, error) bool) {
		_ = yield(1, nil) && yield(2, nil) && yield(3, nil)
	}

	result, err := Last(seq)
	require.NoError(t, err)
	assert.Equal(t, 3, result)
}

func TestLast_EmptySequence(t *testing.T) {
	seq := func(yield func(int, error) bool) {
		// Empty - no yields
	}

	result, err := Last(seq)
	assert.ErrorIs(t, err, ErrEmptySequence)
	assert.Equal(t, 0, result)
}

func TestLast_ErrorInSequence(t *testing.T) {
	expectedErr := errors.New("test error")
	seq := func(yield func(int, error) bool) {
		_ = yield(1, nil) && yield(0, expectedErr) && yield(3, nil)
	}

	result, err := Last(seq)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 0, result)
}

func TestLast_OnlyError(t *testing.T) {
	expectedErr := errors.New("immediate error")
	seq := func(yield func(int, error) bool) {
		_ = yield(0, expectedErr)
	}

	result, err := Last(seq)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 0, result)
}

func TestLast_ErrorAfterValues(t *testing.T) {
	expectedErr := errors.New("late error")
	seq := func(yield func(int, error) bool) {
		_ = yield(10, nil) && yield(20, nil) && yield(0, expectedErr)
	}

	result, err := Last(seq)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, 0, result)
}

func TestCollect_Success(t *testing.T) {
	seq := func(yield func(int, error) bool) {
		_ = yield(1, nil) && yield(2, nil) && yield(3, nil)
	}

	results, err := Collect(seq)
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, results)
}

func TestCollect_EmptySequence(t *testing.T) {
	seq := func(yield func(int, error) bool) {
		// Empty
	}

	results, err := Collect(seq)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestCollect_WithError(t *testing.T) {
	expectedErr := errors.New("collection error")
	seq := func(yield func(int, error) bool) {
		_ = yield(1, nil) && yield(0, expectedErr)
	}

	results, err := Collect(seq)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []int{1}, results) // Should have partial results
}

func TestCollect_ImmediateError(t *testing.T) {
	expectedErr := errors.New("immediate error")
	seq := func(yield func(int, error) bool) {
		_ = yield(0, expectedErr)
	}

	results, err := Collect(seq)
	assert.ErrorIs(t, err, expectedErr)
	assert.Empty(t, results)
}

func TestCollect_WithZeroValues(t *testing.T) {
	seq := func(yield func(int, error) bool) {
		_ = yield(0, nil) && yield(1, nil) && yield(0, nil)
	}

	results, err := Collect(seq)
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1, 0}, results)
}

func TestErrEmptySequence(t *testing.T) {
	assert.Equal(t, "empty sequence", ErrEmptySequence.Error())
}

func TestIteratorError_Error(t *testing.T) {
	err := &IteratorError{msg: "test message"}
	assert.Equal(t, "test message", err.Error())
}

// Test with string type to ensure generics work
func TestLast_Strings(t *testing.T) {
	seq := func(yield func(string, error) bool) {
		_ = yield("first", nil) && yield("second", nil) && yield("third", nil)
	}

	result, err := Last(seq)
	require.NoError(t, err)
	assert.Equal(t, "third", result)
}

// Test Collect with struct type
func TestCollect_Structs(t *testing.T) {
	type data struct {
		Value int
		Name  string
	}

	seq := func(yield func(data, error) bool) {
		_ = yield(data{1, "one"}, nil) && yield(data{2, "two"}, nil) && yield(data{3, "three"}, nil)
	}

	results, err := Collect(seq)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, "one", results[0].Name)
	assert.Equal(t, 2, results[1].Value)
}

// Test that Last consumes the entire iterator (range-over-func protocol)
func TestLast_ConsumesEntireIterator(t *testing.T) {
	consumed := 0
	seq := func(yield func(int, error) bool) {
		for i := 1; i <= 5; i++ {
			consumed++
			if !yield(i, nil) {
				return
			}
		}
	}

	result, err := Last(seq)
	require.NoError(t, err)
	assert.Equal(t, 5, result)
	assert.Equal(t, 5, consumed, "should consume entire iterator")
}

// Test that Collect stops on first error
func TestCollect_StopsOnFirstError(t *testing.T) {
	consumed := 0
	expectedErr := errors.New("stop error")
	seq := func(yield func(int, error) bool) {
		for i := 1; i <= 5; i++ {
			consumed++
			if i == 3 {
				_ = yield(0, expectedErr)
				return
			}
			if !yield(i, nil) {
				return
			}
		}
	}

	results, err := Collect(seq)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []int{1, 2}, results)
	assert.Equal(t, 3, consumed)
}

// Real-world example: using with iter.Seq2
func TestLast_WithRealIterator(t *testing.T) {
	// Simulate a function that returns iter.Seq2
	generator := func() iter.Seq2[string, error] {
		return func(yield func(string, error) bool) {
			messages := []string{"start", "middle", "end"}
			for _, msg := range messages {
				if !yield(msg, nil) {
					return
				}
			}
		}
	}

	result, err := Last(generator())
	require.NoError(t, err)
	assert.Equal(t, "end", result)
}
