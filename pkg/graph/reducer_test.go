package graph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceReducer(t *testing.T) {
	r := ReplaceReducer[string]{}

	t.Run("Zero returns zero value", func(t *testing.T) {
		assert.Equal(t, "", r.Zero())
	})

	t.Run("Reduce returns incoming", func(t *testing.T) {
		assert.Equal(t, "new", r.Reduce("old", "new"))
		assert.Equal(t, "first", r.Reduce("", "first"))
	})
}

func TestAppendReducer(t *testing.T) {
	r := AppendReducer[int]{}

	t.Run("Zero returns nil", func(t *testing.T) {
		assert.Nil(t, r.Zero())
	})

	t.Run("Reduce appends slices", func(t *testing.T) {
		result := r.Reduce([]int{1, 2}, []int{3, 4})
		assert.Equal(t, []int{1, 2, 3, 4}, result)
	})

	t.Run("Reduce with nil existing", func(t *testing.T) {
		result := r.Reduce(nil, []int{1, 2})
		assert.Equal(t, []int{1, 2}, result)
	})

	t.Run("Reduce with nil incoming", func(t *testing.T) {
		result := r.Reduce([]int{1, 2}, nil)
		assert.Equal(t, []int{1, 2}, result)
	})
}

func TestPrependReducer(t *testing.T) {
	r := PrependReducer[string]{}

	t.Run("Zero returns nil", func(t *testing.T) {
		assert.Nil(t, r.Zero())
	})

	t.Run("Reduce prepends slices", func(t *testing.T) {
		result := r.Reduce([]string{"c", "d"}, []string{"a", "b"})
		assert.Equal(t, []string{"a", "b", "c", "d"}, result)
	})
}

func TestSumReducer(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		r := SumReducer[int]{}
		assert.Equal(t, 0, r.Zero())
		assert.Equal(t, 15, r.Reduce(10, 5))
		assert.Equal(t, -5, r.Reduce(0, -5))
	})

	t.Run("float64", func(t *testing.T) {
		r := SumReducer[float64]{}
		assert.Equal(t, 0.0, r.Zero())
		assert.Equal(t, 3.5, r.Reduce(1.5, 2.0))
	})
}

func TestMaxReducer(t *testing.T) {
	r := MaxReducer[int]{}

	t.Run("Zero returns zero value", func(t *testing.T) {
		assert.Equal(t, 0, r.Zero())
	})

	t.Run("Reduce returns max", func(t *testing.T) {
		assert.Equal(t, 10, r.Reduce(5, 10))
		assert.Equal(t, 10, r.Reduce(10, 5))
		assert.Equal(t, 0, r.Reduce(-5, 0))
	})
}

func TestMinReducer(t *testing.T) {
	r := MinReducer[int]{}

	t.Run("Zero returns zero value", func(t *testing.T) {
		assert.Equal(t, 0, r.Zero())
	})

	t.Run("Reduce returns min", func(t *testing.T) {
		assert.Equal(t, 5, r.Reduce(5, 10))
		assert.Equal(t, 5, r.Reduce(10, 5))
		assert.Equal(t, -5, r.Reduce(-5, 0))
	})
}

func TestMergeMapReducer(t *testing.T) {
	r := MergeMapReducer[string, int]{}

	t.Run("Zero returns nil", func(t *testing.T) {
		assert.Nil(t, r.Zero())
	})

	t.Run("Reduce merges maps", func(t *testing.T) {
		existing := map[string]int{"a": 1, "b": 2}
		incoming := map[string]int{"b": 3, "c": 4}
		result := r.Reduce(existing, incoming)
		assert.Equal(t, map[string]int{"a": 1, "b": 3, "c": 4}, result)
	})

	t.Run("Reduce with nil existing", func(t *testing.T) {
		incoming := map[string]int{"a": 1}
		result := r.Reduce(nil, incoming)
		assert.Equal(t, map[string]int{"a": 1}, result)
	})
}

func TestFirstReducer(t *testing.T) {
	r := FirstReducer[string]{}

	t.Run("Zero returns zero value", func(t *testing.T) {
		assert.Equal(t, "", r.Zero())
	})

	t.Run("Reduce keeps first non-zero", func(t *testing.T) {
		assert.Equal(t, "first", r.Reduce("first", "second"))
		assert.Equal(t, "new", r.Reduce("", "new"))
	})
}

func TestSkipZeroReducer(t *testing.T) {
	r := NewSkipZeroReducer[string](ReplaceReducer[string]{})

	t.Run("Zero delegates to inner", func(t *testing.T) {
		assert.Equal(t, "", r.Zero())
	})

	t.Run("Reduce skips zero incoming", func(t *testing.T) {
		assert.Equal(t, "existing", r.Reduce("existing", ""))
	})

	t.Run("Reduce applies non-zero incoming", func(t *testing.T) {
		assert.Equal(t, "new", r.Reduce("existing", "new"))
	})
}

func TestWrapReducer(t *testing.T) {
	r := WrapReducer(SumReducer[int]{})

	t.Run("ZeroFn returns zero", func(t *testing.T) {
		assert.Equal(t, 0, r.ZeroFn())
	})

	t.Run("ReduceFn merges values", func(t *testing.T) {
		result := r.ReduceFn(10, 5)
		assert.Equal(t, 15, result)
	})

	t.Run("ReduceFn handles nil existing", func(t *testing.T) {
		result := r.ReduceFn(nil, 5)
		assert.Equal(t, 5, result)
	})
}
