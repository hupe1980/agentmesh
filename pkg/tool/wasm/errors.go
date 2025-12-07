// Package wasm provides sentinel errors for the wasm tool package.
package wasm

import "errors"

var (
	// ErrEmptyResult is returned when WASM returns an empty result.
	ErrEmptyResult = errors.New("tool/wasm: returned empty result")

	// ErrNoAllocateFunction is returned when WASM module doesn't export allocate function.
	ErrNoAllocateFunction = errors.New("tool/wasm: module does not export 'allocate' function")

	// ErrAllocateNoPointer is returned when allocate() doesn't return a pointer.
	ErrAllocateNoPointer = errors.New("tool/wasm: allocate() did not return a pointer")

	// ErrAllocateNullPointer is returned when allocate() returns null pointer.
	ErrAllocateNullPointer = errors.New("tool/wasm: allocate() returned null pointer")
)
