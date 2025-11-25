package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNotNil(t *testing.T) {
	t.Run("nil pointer", func(t *testing.T) {
		var p *string
		err := NotNil(p, "test pointer")
		assert.EqualError(t, err, "test pointer must not be nil")
	})

	t.Run("non-nil pointer", func(t *testing.T) {
		s := "value"
		err := NotNil(&s, "test pointer")
		assert.NoError(t, err)
	})

	t.Run("generic types", func(t *testing.T) {
		var intPtr *int
		err := NotNil(intPtr, "integer")
		assert.EqualError(t, err, "integer must not be nil")

		i := 42
		err = NotNil(&i, "integer")
		assert.NoError(t, err)
	})
}

func TestNotEmpty(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		err := NotEmpty("", "test string")
		assert.EqualError(t, err, "test string cannot be empty")
	})

	t.Run("non-empty string", func(t *testing.T) {
		err := NotEmpty("value", "test string")
		assert.NoError(t, err)
	})

	t.Run("whitespace only", func(t *testing.T) {
		err := NotEmpty("   ", "test string")
		assert.NoError(t, err)
	})
}

func TestNotEmptySlice(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		err := NotEmptySlice([]string{}, "test slice")
		assert.EqualError(t, err, "test slice must not be empty")
	})

	t.Run("nil slice", func(t *testing.T) {
		var s []string
		err := NotEmptySlice(s, "test slice")
		assert.EqualError(t, err, "test slice must not be empty")
	})

	t.Run("non-empty slice", func(t *testing.T) {
		err := NotEmptySlice([]string{"a", "b"}, "test slice")
		assert.NoError(t, err)
	})

	t.Run("generic types", func(t *testing.T) {
		err := NotEmptySlice([]int{}, "integers")
		assert.EqualError(t, err, "integers must not be empty")

		err = NotEmptySlice([]int{1, 2, 3}, "integers")
		assert.NoError(t, err)
	})
}

func TestAll(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		err := All(nil, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("first error returned", func(t *testing.T) {
		err1 := NotEmpty("", "field1")
		err2 := NotNil((*string)(nil), "field2")

		err := All(err1, err2)
		assert.EqualError(t, err, "field1 cannot be empty")
	})

	t.Run("empty list", func(t *testing.T) {
		err := All()
		assert.NoError(t, err)
	})

	t.Run("chaining validations", func(t *testing.T) {
		name := "test"
		var model *int

		err := All(
			NotEmpty(name, "name"),
			NotNil(model, "model"),
		)
		assert.EqualError(t, err, "model must not be nil")
	})
}
