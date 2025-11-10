// Package wasm provides WebAssembly-based tool sandboxing for secure execution
// of untrusted code with memory isolation and resource limits.
//
// WASM tools run inside a lightweight, memory-safe sandbox enforced by the
// WebAssembly runtime. Each tool operates in its own isolated environment with
// strict resource limits and no access to the host system unless explicitly
// granted through controlled interfaces (e.g., WASI capabilities).
//
// When combined with containerization or process isolation, this approach
// achieves defense-in-depth comparable to kernel-level isolation—but with
// the speed and portability of WebAssembly.
//
// # Security Model
//
// WASM tools provide runtime-enforced security guarantees:
//   - Memory isolation: WASM modules have isolated linear memory
//   - No syscalls by default: Network, filesystem blocked unless granted via WASI
//   - Resource limits: Memory and CPU usage strictly enforced
//   - Controlled capabilities: All host access through explicitly imported functions
//
// # Example Usage
//
//	// Load WASM module
//	wasmBytes, _ := os.ReadFile("calculator.wasm")
//
//	// Create sandboxed tool with compute-only policy
//	tool, err := wasm.NewWASMTool(
//	    context.Background(),
//	    "calculator",
//	    "Evaluate mathematical expressions",
//	    wasmBytes,
//	    wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
//	)
//
//	// Execute safely
//	result, err := tool.Call(ctx, `{"operation": "add", "a": 5, "b": 3}`)
//
// # Security Policies
//
// The package provides several preset security policies:
//   - ComputeOnlyPolicy: Pure computation, no external access
//   - NetworkOnlyPolicy: HTTP/API access without filesystem
//   - FileProcessingPolicy: Access specific directories only
//   - DeterministicPolicy: Fresh instance per call for reproducibility
//   - PermissiveSandboxPolicy: All capabilities enabled (trusted tools only)
//
// Custom policies can be created for fine-grained control over capabilities,
// resource limits, and security levels.
package wasm
