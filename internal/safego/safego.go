// Package safego provides utilities for safe goroutine execution with panic recovery.
//
// This package centralizes panic recovery logic that was previously duplicated
// across multiple packages. It provides consistent panic handling with stack traces
// for debugging production issues.
//
// # Design Goals
//
//   - **Eliminate Code Duplication:** Single source of truth for panic recovery
//   - **Consistent Error Messages:** Standardized panic error format
//   - **Stack Traces:** Include stack traces for debugging
//   - **Zero Dependencies:** Pure stdlib implementation
//
// # Usage Examples
//
// Simple function execution with panic recovery:
//
//	err := safego.Run(func() error {
//	   // Code that might panic
//	   return doSomething()
//	})
//
//	if err != nil {
//	   // Handle panic or regular error
//	}
//
// Execution with return value:
//
//	result, err := safego.Call(func() (string, error) {
//	   // Code that might panic and returns a value
//	   return fetchData()
//	})
//
// Custom panic handler:
//
//	err := safego.RunWith(func() error {
//	   return doSomething()
//	}, func(r any) error {
//
//	   log.Error("Custom panic handler", "panic", r)
//	   return fmt.Errorf("custom panic: %v", r)
//	})
package safego

import (
	"fmt"
	"runtime/debug"
)

// Run executes a function with panic recovery.
// If the function panics, the panic is recovered and returned as an error
// with a stack trace for debugging.
//
// Example:
//
//	err := safego.Run(func() error {
//	   // Potentially panicking code
//	   return riskyOperation()
//	})
func Run(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}

// Call executes a function that returns a value and error, with panic recovery.
// If the function panics, the panic is recovered, the zero value is returned,
// and the panic is converted to an error with stack trace.
//
// Example:
//
//	result, err := safego.Call(func() (string, error) {
//	   return fetchData()
//	})
func Call[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			var zero T
			result = zero
			err = fmt.Errorf("panic recovered: %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}

// RunWith executes a function with a custom panic handler.
// The panic handler receives the recovered panic value and can return
// a custom error. If the handler itself panics, that panic is not recovered.
//
// Example:
//
// err := safego.RunWith(
//
//	func() error { return operation() },
//	func(r any) error {
//	    log.Error("operation panicked", "panic", r)
//	    return fmt.Errorf("operation failed: %v", r)
//	},
//
// )
func RunWith(fn func() error, handler func(any) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = handler(r)
		}
	}()
	return fn()
}

// CallWith executes a function with return value and custom panic handler.
// Similar to RunWith but for functions that return values.
func CallWith[T any](fn func() (T, error), handler func(any) error) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			var zero T
			result = zero
			err = handler(r)
		}
	}()
	return fn()
}

// Go starts a goroutine with panic recovery.
// If the goroutine panics, the panic is recovered and passed to the error handler.
// This is useful for background goroutines where panics should be logged but not crash the program.
//
// Example:
//
//	safego.Go(func() error {
//	   return backgroundTask()
//	}, func(err error) {
//
//	   log.Error("background task failed", "error", err)
//	})
func Go(fn func() error, onError func(error)) {
	go func() {
		if err := Run(fn); err != nil {
			onError(err)
		}
	}()
}
