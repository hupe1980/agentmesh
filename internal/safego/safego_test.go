package safego

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		called := false
		err := Run(func() error {
			called = true
			return nil
		})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("returns error", func(t *testing.T) {
		expectedErr := errors.New("test error")
		err := Run(func() error {
			return expectedErr
		})
		assert.Equal(t, expectedErr, err)
	})

	t.Run("recovers panic", func(t *testing.T) {
		err := Run(func() error {
			panic("test panic")
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "panic recovered: test panic")
		assert.Contains(t, err.Error(), "safego_test.go") // Stack trace
	})

	t.Run("panic with nil", func(t *testing.T) {
		err := Run(func() error {
			panic(nil)
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "panic recovered: panic called with nil argument")
	})
}

func TestCall(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		result, err := Call(func() (string, error) {
			return "success", nil
		})
		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})

	t.Run("returns error", func(t *testing.T) {
		expectedErr := errors.New("test error")
		result, err := Call(func() (string, error) {
			return "partial", expectedErr
		})
		assert.Equal(t, expectedErr, err)
		assert.Equal(t, "partial", result)
	})

	t.Run("recovers panic", func(t *testing.T) {
		result, err := Call(func() (int, error) {
			panic("test panic")
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "panic recovered: test panic")
		assert.Equal(t, 0, result) // Zero value
	})

	t.Run("generic types", func(t *testing.T) {
		// Test with different types
		strResult, err := Call(func() (string, error) {
			return "test", nil
		})
		assert.NoError(t, err)
		assert.Equal(t, "test", strResult)

		intResult, err := Call(func() (int, error) {
			return 42, nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 42, intResult)

		boolResult, err := Call(func() (bool, error) {
			return true, nil
		})
		assert.NoError(t, err)
		assert.True(t, boolResult)
	})
}

func TestRunWith(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		handlerCalled := false
		err := RunWith(
			func() error {
				return nil
			},
			func(r any) error {
				handlerCalled = true
				return fmt.Errorf("should not be called")
			},
		)
		assert.NoError(t, err)
		assert.False(t, handlerCalled)
	})

	t.Run("custom panic handler", func(t *testing.T) {
		var panicValue any
		err := RunWith(
			func() error {
				panic("custom panic")
			},
			func(r any) error {
				panicValue = r
				return fmt.Errorf("custom error: %v", r)
			},
		)
		assert.Error(t, err)
		assert.Equal(t, "custom panic", panicValue)
		assert.Equal(t, "custom error: custom panic", err.Error())
	})
}

func TestCallWith(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		result, err := CallWith(
			func() (string, error) {
				return "success", nil
			},
			func(r any) error {
				return fmt.Errorf("should not be called")
			},
		)
		assert.NoError(t, err)
		assert.Equal(t, "success", result)
	})

	t.Run("custom panic handler", func(t *testing.T) {
		result, err := CallWith(
			func() (int, error) {
				panic("custom panic")
			},
			func(r any) error {
				return fmt.Errorf("custom error: %v", r)
			},
		)
		assert.Error(t, err)
		assert.Equal(t, "custom error: custom panic", err.Error())
		assert.Equal(t, 0, result) // Zero value
	})
}

func TestGo(t *testing.T) {
	t.Run("normal execution", func(t *testing.T) {
		done := make(chan bool)
		errorHandlerCalled := false

		Go(
			func() error {
				close(done)
				return nil
			},
			func(err error) {
				errorHandlerCalled = true
			},
		)

		<-done
		assert.False(t, errorHandlerCalled)
	})

	t.Run("error handling", func(t *testing.T) {
		done := make(chan error, 1)

		Go(
			func() error {
				return errors.New("goroutine error")
			},
			func(err error) {
				done <- err
			},
		)

		err := <-done
		assert.Error(t, err)
		assert.Equal(t, "goroutine error", err.Error())
	})

	t.Run("panic recovery", func(t *testing.T) {
		done := make(chan error, 1)

		Go(
			func() error {
				panic("goroutine panic")
			},
			func(err error) {
				done <- err
			},
		)

		err := <-done
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "panic recovered: goroutine panic")
	})
}

func TestStackTracePresence(t *testing.T) {
	err := Run(func() error {
		panic("test")
	})

	assert.Error(t, err)
	errMsg := err.Error()

	// Verify stack trace components
	assert.Contains(t, errMsg, "panic recovered: test")
	assert.Contains(t, errMsg, "goroutine")
	// Should contain file and line information
	assert.True(t, strings.Contains(errMsg, ".go:"), "should contain file references")
}

func TestPanicWithStructuredData(t *testing.T) {
	type CustomPanic struct {
		Message string
		Code    int
	}

	err := Run(func() error {
		panic(CustomPanic{Message: "custom", Code: 42})
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "custom")
	assert.Contains(t, err.Error(), "42")
}
