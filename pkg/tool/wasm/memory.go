package wasm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// allocateString allocates memory in the WASM module for a Go string.
// Returns (pointer, length, error).
// The WASM module must export an "allocate" function: allocate(size) -> ptr
func (w *WASMTool) allocateString(ctx context.Context, mod api.Module, s string) (ptr uint32, length uint32, err error) {
	bytes := []byte(s)
	length = uint32(len(bytes))

	if length == 0 {
		return 0, 0, nil
	}

	// Get the allocate function
	allocateFunc := mod.ExportedFunction("allocate")
	if allocateFunc == nil {
		return 0, 0, fmt.Errorf("WASM module does not export 'allocate' function")
	}

	// Call allocate(size) -> ptr
	results, err := allocateFunc.Call(ctx, uint64(length))
	if err != nil {
		return 0, 0, fmt.Errorf("allocate() failed: %w", err)
	}

	if len(results) == 0 {
		return 0, 0, fmt.Errorf("allocate() did not return a pointer")
	}

	ptr = uint32(results[0])
	if ptr == 0 {
		return 0, 0, fmt.Errorf("allocate() returned null pointer")
	}

	// Write the string bytes to WASM memory
	if !mod.Memory().Write(ptr, bytes) {
		return 0, 0, fmt.Errorf("failed to write %d bytes to WASM memory at offset %d", length, ptr)
	}

	return ptr, length, nil
}

// readString reads a string from WASM memory at the given pointer and length.
func (w *WASMTool) readString(mod api.Module, ptr, length uint32) (string, error) {
	if length == 0 {
		return "", nil
	}

	bytes, ok := mod.Memory().Read(ptr, length)
	if !ok {
		return "", fmt.Errorf("failed to read %d bytes at offset %d", length, ptr)
	}

	return string(bytes), nil
}
