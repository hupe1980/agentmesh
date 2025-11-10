# WASM Tool Example# WASM Tool Example# WASM Tool Example# WASM Tool Example



This example demonstrates how to use Rust tools compiled to WebAssembly with AgentMesh, both directly and integrated with a ReAct agent.



## OverviewThis example demonstrates how to use Rust tools compiled to WebAssembly with AgentMesh's sandboxing.



The calculator tool is a Rust library that performs arithmetic calculations. When compiled to WASM and executed through AgentMesh's `WASMTool`, it runs in a completely isolated sandbox with:



- ✅ **No network access** - Cannot make HTTP requests## OverviewThis example demonstrates how to use Rust tools compiled to WebAssembly with AgentMesh's sandboxing.This example demonstrates how to use Python tools compiled to WebAssembly with AgentMesh's sandboxing.

- ✅ **No filesystem access** - Cannot read or write files  

- ✅ **No environment access** - Cannot read environment variables

- ✅ **Memory limits** - Restricted to 200MB by default

- ✅ **Timeout enforcement** - Kills execution after 10 secondsThe calculator tool is a Rust library that performs arithmetic calculations. When compiled to WASM and executed through AgentMesh's `WASMTool`, it runs in a completely isolated sandbox with:

- ✅ **Memory safety** - Rust's safety guarantees carry through to WASM

- ✅ **Tiny binaries** - Only 128KB (vs 800KB+ for Go)



## Files- ✅ **No network access** - Cannot make HTTP requests## Overview## Overview



- `src/lib.rs` - Rust calculator implementation- ✅ **No filesystem access** - Cannot read or write files  

- `Cargo.toml` - Rust configuration with WASM optimization

- `main.go` - Example showing direct tool calls and ReAct agent integration- ✅ **No environment access** - Cannot read environment variables

- `build.sh` - Script to build Rust to WASM using Docker

- `calculator.wasm` - Compiled WASM module (generated after build)- ✅ **Memory limits** - Restricted to 200MB by default



## Why Rust?- ✅ **Timeout enforcement** - Kills execution after 10 secondsThe calculator tool is a Rust library that performs arithmetic calculations. When compiled to WASM and executed through AgentMesh's `WASMTool`, it runs in a completely isolated sandbox with:The calculator tool is a simple Python script that performs arithmetic calculations. When compiled to WASM and executed through AgentMesh's `WASMTool`, it runs in a completely isolated sandbox with:



Rust is the gold standard for WebAssembly:- ✅ **Memory safety** - Rust's safety guarantees carry through to WASM



- **First-class WASM support** - Built into the language and toolchain- ✅ **Tiny binaries** - Only 128KB (vs 800KB+ for Go)

- **Tiny binaries** - 128KB optimized (6.8x smaller than TinyGo)

- **Memory safety** - Zero-cost abstractions with guaranteed safety

- **Fast compilation** - Aggressive optimization with LTO

- **Production ready** - Used by Cloudflare Workers, Fastly Compute@Edge, etc.## Files- ✅ **No network access** - Cannot make HTTP requests- ✅ **No network access** - Cannot make HTTP requests

- **Full WASI support** - Via `wasm32-unknown-unknown` target



## Prerequisites

- `src/lib.rs` - Rust calculator implementation- ✅ **No filesystem access** - Cannot read or write files  - ✅ **No filesystem access** - Cannot read or write files

**No installation required!** The build script uses Docker with the official Rust image.

- `Cargo.toml` - Rust configuration with WASM optimization

For the ReAct agent testing, you'll need an OpenAI API key.

- `main.go` - Example showing how to use the WASM tool with AgentMesh- ✅ **No environment access** - Cannot read environment variables- ✅ **No environment access** - Cannot read environment variables

## Building the WASM Module

- `build.sh` - Script to build Rust to WASM using Docker

### Using Docker (Recommended)

- `calculator.wasm` - Compiled WASM module (generated after build)- ✅ **Memory limits** - Restricted to 200MB by default- ✅ **Memory limits** - Restricted to 200MB by default

```bash

# Simple: Use the build script

./build.sh

## Why Rust?- ✅ **Timeout enforcement** - Kills execution after 10 seconds- ✅ **Timeout enforcement** - Kills execution after 10 seconds

# Or manually with Docker:

docker run --rm -v $(pwd):/usr/src/app -w /usr/src/app rust:1.83 \

  bash -c "rustup target add wasm32-unknown-unknown && \

  cargo build --release --target wasm32-unknown-unknown && \Rust is the gold standard for WebAssembly:- ✅ **Memory safety** - Rust's safety guarantees carry through to WASM

  cp target/wasm32-unknown-unknown/release/calculator.wasm ."

```



### Local Installation- **First-class WASM support** - Built into the language and toolchain- ✅ **Tiny binaries** - Only 128KB (vs 800KB+ for Go, 75MB+ for Python)## Files



If you have Rust installed:- **Tiny binaries** - 128KB optimized (6.8x smaller than TinyGo)



```bash- **Memory safety** - Zero-cost abstractions with guaranteed safety

rustup target add wasm32-unknown-unknown

cargo build --release --target wasm32-unknown-unknown- **Fast compilation** - Aggressive optimization with LTO

cp target/wasm32-unknown-unknown/release/calculator.wasm .

```- **Production ready** - Used by Cloudflare Workers, Fastly Compute@Edge, etc.## Files- `calculator.py` - Original Python tool (for reference)



Install Rust: https://rustup.rs/- **Full WASI support** - Via `wasm32-unknown-unknown` target



## Running the Example- `calculator.go` - Go implementation compiled to WASM



The example demonstrates two ways to use the WASM calculator:## Prerequisites



1. **Direct tool calls** - Call the WASM tool directly with JSON input- `src/lib.rs` - Rust calculator implementation- `main.go` - Example showing how to use the WASM tool with AgentMesh

2. **ReAct agent integration** - Use the tool with an AI agent that reasons about when to use it

**No installation required!** The build script uses Docker with the official Rust image.

```bash

# Set your OpenAI API key for agent testing- `Cargo.toml` - Rust configuration with WASM optimization- `build.sh` - Script to build Go to WASM using TinyGo in Docker

export OPENAI_API_KEY=sk-...

## Building the WASM Module

# Run the example

go run main.go- `calculator.py` - Original Python tool (for reference)- `calculator.wasm` - Compiled WASM module (generated after build)

```

### Using Docker (Recommended)

Expected output:

```- `main.go` - Example showing how to use the WASM tool with AgentMesh

Tool Definition:

{```bash

  "type": "function",

  "function": {# Simple: Use the build script- `build.sh` - Script to build Rust to WASM using Docker## Why Go instead of Python?

    "name": "calculator",

    "description": "Performs arithmetic calculations",./build.sh

    "parameters": { ... }

  }- `calculator.wasm` - Compiled WASM module (generated after build)

}

# Or manually with Docker:

=== Direct Tool Testing ===

Input:  {"expression": "2 + 2"}docker run --rm -v $(pwd):/usr/src/app -w /usr/src/app rust:1.83 \While Python-to-WASM compilers exist (like py2wasm), they're experimental and have limitations. TinyGo provides a production-ready solution for compiling to WASM with:

Output: {"result": 4}

  bash -c "rustup target add wasm32-unknown-unknown && \

Input:  {"expression": "10 * 5 + 3"}

Output: {"result": 53}  cargo build --release --target wasm32-unknown-unknown && \## Why Rust?- Small binary sizes (typically <100KB)



=== ReAct Agent Testing ===  cp target/wasm32-unknown-unknown/release/calculator.wasm ."

Question: What is 15 multiplied by 8?

Answer: The result of 15 multiplied by 8 is 120.```- Fast compilation



Question: Calculate 100 divided by 4 plus 10

Answer: The result is 35.

### Local InstallationRust is the gold standard for WebAssembly:- Full WASI support

Question: What's the result of (5 + 3) * 2?

Answer: The result is 16.

```

If you have Rust installed:- Easy integration with AgentMesh

## How It Works



### 1. Rust Implementation (src/lib.rs)

```bash- **First-class WASM support** - Built into the language and toolchain

The calculator exports functions that the Go host can call:

rustup target add wasm32-unknown-unknown

```rust

#[no_mangle]cargo build --release --target wasm32-unknown-unknown- **Tiny binaries** - 128KB optimized (6.8x smaller than TinyGo, 600x smaller than Python)The calculator logic from Python has been ported to Go for reliability.

pub extern "C" fn allocate(size: usize) -> *mut u8 {

    // Allocate memory for inputcp target/wasm32-unknown-unknown/release/calculator.wasm .

}

```- **Memory safety** - Zero-cost abstractions with guaranteed safety

#[no_mangle]

pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) {

    // Parse JSON, evaluate expression, store result

}Install Rust: https://rustup.rs/- **Fast compilation** - Aggressive optimization with LTO## Prerequisites



#[no_mangle]

pub extern "C" fn get_result_ptr() -> *const u8 {

    // Return pointer to result## Running the Example- **Production ready** - Used by Cloudflare Workers, Fastly Compute@Edge, etc.

}



#[no_mangle]

pub extern "C" fn get_result_len() -> usize {```bash- **Full WASI support** - Via `wasm32-unknown-unknown` target**No installation required!** Uses Docker with TinyGo to build WASM.

    // Return length of result

}go run main.go

```

```

The calculator:

1. Reads JSON input from WASM memory

2. Parses arithmetic expressions using a recursive descent parser

3. Evaluates expressions with support for +, -, *, /, and parenthesesExpected output:## Prerequisites## Building the WASM Module

4. Returns JSON output via global state

```

### 2. Cargo Configuration

Tool Definition:

The `Cargo.toml` is optimized for small WASM binaries:

{

```toml

[lib]  "type": "function",**No installation required!** The build script uses Docker with the official Rust image.### Using Docker (Recommended)

crate-type = ["cdylib"]  # Dynamic library for WASM

  "function": {

[profile.release]

opt-level = "z"      # Optimize for size    "name": "calculator",

lto = true          # Link-time optimization

strip = true        # Strip debug symbols    "description": "Performs arithmetic calculations",

panic = "abort"     # Smaller panic handler

codegen-units = 1   # Better optimization    "parameters": { ... }## Building the WASM Module```bash

```

  }

These settings reduce the binary from ~400KB to ~128KB.

}# Simple: Use the build script

### 3. Go Integration (main.go)



The example shows two usage patterns:

Input:  {"expression": "2 + 2"}### Using Docker (Recommended)./build.sh

**Direct tool calls:**

```goOutput: {"result": 4}

tool, err := wasm.NewWASMTool(

    ctx,

    "calculator",

    "Performs arithmetic calculations",Input:  {"expression": "10 * 5 + 3"}

    wasmBytes,

    wasm.WithPolicy(wasm.ComputeOnlyPolicy()),Output: {"result": 53}```bash# Or manually with Docker:

    wasm.WithSchema(schema),

)...



result, err := tool.Call(ctx, `{"expression": "2 + 2"}`)```# Simple: Use the build scriptdocker run --rm -v $(pwd):/src -w /src tinygo/tinygo:0.33.0 \

```



**ReAct agent integration:**

```go## How It Works./build.sh  tinygo build -o calculator.wasm -target=wasi calculator.go

reactAgent, err := agent.NewReActAgent(

    openai.NewModel(),

    agent.WithTools(tool),

)### 1. Rust Implementation (src/lib.rs)```



system := message.NewSystemMessageFromText(

    "You are a helpful math assistant. Use the calculator tool.",

)The calculator exports functions that the Go host can call:# Or manually with Docker:

human := message.NewHumanMessageFromText("What is 15 multiplied by 8?")



results, err := reactAgent.Invoke(ctx, []message.Message{system, human})

``````rustdocker run --rm -v $(pwd):/usr/src/app -w /usr/src/app rust:1.83 \### Local Installation



The agent will:#[no_mangle]

1. Analyze the user's question

2. Decide to use the calculator toolpub extern "C" fn allocate(size: usize) -> *mut u8 {  bash -c "rustup target add wasm32-unknown-unknown && \

3. Call the WASM tool with the appropriate expression

4. Generate a natural language response with the result    // Allocate memory for input



## Security Features}  cargo build --release --target wasm32-unknown-unknown && \If you have TinyGo installed locally:



The WASM sandbox provides kernel-level isolation:



1. **Memory Isolation** - WASM has its own linear memory, isolated from the host#[no_mangle]  cp target/wasm32-unknown-unknown/release/calculator.wasm ."

2. **No System Calls** - Cannot make syscalls unless explicitly granted

3. **Deterministic Execution** - Same input always produces same outputpub extern "C" fn execute(input_ptr: *const u8, input_len: usize) {

4. **Resource Limits** - Memory and timeout enforced by the runtime

5. **Cannot Be Bypassed** - Enforced by the Wazero runtime, not user-space code    // Parse JSON, evaluate expression, store result``````bash



The example uses `ComputeOnlyPolicy()`:}

- ❌ No filesystem access

- ❌ No network access  tinygo build -o calculator.wasm -target=wasi calculator.go

- ❌ No stdin/stdout/stderr

- ✅ Deterministic mode enabled#[no_mangle]

- ✅ 200MB memory limit

- ✅ 10 second timeoutpub extern "C" fn get_result_ptr() -> *const u8 {### Local Installation```



## Architecture    // Return pointer to result



```}

┌─────────────────────────────────────────┐

│         AgentMesh (Go)                  │

│                                         │

│  ┌───────────────────────────────┐    │#[no_mangle]If you have Rust installed:This creates a compact `calculator.wasm` file (typically ~50KB) that can be loaded by the AgentMesh WASM tool.

│  │    ReAct Agent (Optional)    │    │

│  │  ┌───────────────────────┐   │    │pub extern "C" fn get_result_len() -> usize {

│  │  │    WASMTool           │   │    │

│  │  │  ┌─────────────────┐  │   │    │    // Return length of result

│  │  │  │ Wazero Runtime  │  │   │    │

│  │  │  │                 │  │   │    │}

│  │  │  │  ┌──────────┐   │  │   │    │

│  │  │  │  │  WASM    │   │  │   │    │``````bash## Quick Start

│  │  │  │  │  Module  │   │  │   │    │

│  │  │  │  │  (Rust)  │   │  │   │    │

│  │  │  │  └──────────┘   │  │   │    │

│  │  │  └─────────────────┘  │   │    │The calculator:rustup target add wasm32-unknown-unknown

│  │  └───────────────────────┘   │    │

│  └───────────────────────────────┘    │1. Reads JSON input from WASM memory

└─────────────────────────────────────────┘

```2. Parses arithmetic expressions using a recursive descent parsercargo build --release --target wasm32-unknown-unknown```bash



## Troubleshooting3. Evaluates expressions with support for +, -, *, /, and parentheses



### Build fails with "cargo: command not found"4. Returns JSON output via global statecp target/wasm32-unknown-unknown/release/calculator.wasm .# 1. Build the WASM module using Docker

Use the Docker method (recommended), or install Rust from https://rustup.rs/



### Docker not running

Make sure Docker is installed and running:### 2. Cargo Configuration```./build.sh

```bash

docker info

```

The `Cargo.toml` is optimized for small WASM binaries:

### WASM execution errors

Check that:

1. You built with `--target wasm32-unknown-unknown`

2. The `[lib] crate-type = ["cdylib"]` is set in Cargo.toml```tomlInstall Rust: https://rustup.rs/# 2. Run the example

3. Functions are marked with `#[no_mangle]` and `extern "C"`

4. Global state is properly managed (see `RESULT` variable)[lib]



### Agent testing failscrate-type = ["cdylib"]  # Dynamic library for WASMgo run main.go

Make sure you have set the `OPENAI_API_KEY` environment variable:

```bash

export OPENAI_API_KEY=sk-...

```[profile.release]## Running the Example```



### Warning: "creating a shared reference to mutable static"opt-level = "z"      # Optimize for size

This is expected and safe in our single-threaded WASM context. The warning can be suppressed by using `&raw const` in future Rust versions.

lto = true          # Link-time optimization

## Extending the Example

strip = true        # Strip debug symbols

To create your own Rust WASM tools:

panic = "abort"     # Smaller panic handler```bashExpected output:

1. **Create a new Rust library:**

   ```bashcodegen-units = 1   # Better optimization

   cargo new --lib my_tool

   ``````go run main.go```



2. **Configure Cargo.toml:**

   ```toml

   [lib]These settings reduce the binary from ~400KB to ~128KB.```Tool Definition:

   crate-type = ["cdylib"]

   

   [dependencies]

   serde = { version = "1.0", features = ["derive"] }### 3. Go Integration (main.go){

   serde_json = "1.0"

   

   [profile.release]

   opt-level = "z"The Go code:Expected output:  "type": "function",

   lto = true

   strip = true1. Loads the WASM binary

   panic = "abort"

   codegen-units = 12. Creates a `WASMTool` with schema and security policy```  "function": {

   ```

3. Calls the tool like any other AgentMesh tool

3. **Implement required exports in src/lib.rs:**

   ```rust4. The `WASMTool` handles all memory management and sandboxingTool Definition:    "name": "calculator",

   #[no_mangle]

   pub extern "C" fn allocate(size: usize) -> *mut u8 { ... }

   

   #[no_mangle]## Security Features{    "description": "Performs arithmetic calculations on mathematical expressions",

   pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) { ... }

   

   #[no_mangle]

   pub extern "C" fn get_result_ptr() -> *const u8 { ... }The WASM sandbox provides kernel-level isolation:  "type": "function",    "parameters": {

   

   #[no_mangle]

   pub extern "C" fn get_result_len() -> usize { ... }

   ```1. **Memory Isolation** - WASM has its own linear memory, isolated from the host  "function": {      "type": "object",



4. **Build to WASM:**2. **No System Calls** - Cannot make syscalls unless explicitly granted

   ```bash

   rustup target add wasm32-unknown-unknown3. **Deterministic Execution** - Same input always produces same output    "name": "calculator",      "properties": {

   cargo build --release --target wasm32-unknown-unknown

   ```4. **Resource Limits** - Memory and timeout enforced by the runtime



5. **Use with AgentMesh:**5. **Cannot Be Bypassed** - Enforced by the Wazero runtime, not user-space code    "description": "Performs arithmetic calculations",        "expression": {

   ```go

   // Direct usage

   tool, err := wasm.NewWASMTool(

       wasmBytes,The example uses `ComputeOnlyPolicy()`:    "parameters": { ... }          "type": "string",

       schema,

       wasm.ComputeOnlyPolicy(),- ❌ No filesystem access

   )

   - ❌ No network access    }          "description": "A mathematical expression to evaluate (e.g., '2 + 2', '10 * 5 - 3')"

   // Or with a ReAct agent

   agent, err := agent.NewReActAgent(- ❌ No stdin/stdout/stderr

       openai.NewModel(),

       agent.WithTools(tool),- ✅ Deterministic mode enabled}        }

   )

   ```- ✅ 200MB memory limit



## Other Languages- ✅ 10 second timeout      },



While this example uses Rust, you can compile many languages to WASM:



| Language | Binary Size | Maturity | Recommendation |## ArchitectureInput:  {"expression": "2 + 2"}      "required": ["expression"]

|----------|-------------|----------|----------------|

| **Rust** | ~100-200KB | ⭐⭐⭐⭐⭐ | **Best choice** |

| Go (TinyGo) | ~800KB | ⭐⭐⭐⭐ | Good alternative |

| C/C++ | ~50-150KB | ⭐⭐⭐⭐⭐ | Great for existing code |```Output: {"result": 4}    }

| AssemblyScript | ~50-100KB | ⭐⭐⭐ | TypeScript-like syntax |

┌─────────────────────────────────────────┐

## Performance

│         AgentMesh (Go)                  │  }

Benchmark on Apple M1:

│                                         │

- **Binary size**: 128KB (optimized)

- **Load time**: ~2-5ms (first time), <1ms (cached)│  ┌───────────────────────────────┐    │Input:  {"expression": "10 * 5 + 3"}}

- **Execution time**: <1ms for simple expressions

- **Memory usage**: <1MB for typical operations│  │      WASMTool                 │    │

- **Agent overhead**: +100-500ms for LLM reasoning

│  │  ┌─────────────────────────┐  │    │Output: {"result": 53}

## References

│  │  │   Wazero Runtime        │  │    │

- [Rust and WebAssembly Book](https://rustwasm.github.io/docs/book/)

- [wasm32-unknown-unknown target](https://doc.rust-lang.org/rustc/platform-support/wasm32-unknown-unknown.html)│  │  │                         │  │    │...Input:  {"expression": "2 + 2"}

- [Wazero Runtime](https://wazero.io/)

- [AgentMesh WASM Design](../../SANDBOX.md)│  │  │  ┌──────────────────┐   │  │    │

- [ReAct Pattern](https://arxiv.org/abs/2210.03629)

│  │  │  │  WASM Module     │   │  │    │```Output: {

│  │  │  │  (Rust code)     │   │  │    │

│  │  │  │                  │   │  │    │  "result": 4

│  │  │  │  - Isolated mem  │   │  │    │

│  │  │  │  - No syscalls   │   │  │    │## How It Works}

│  │  │  │  - Enforced caps │   │  │    │

│  │  │  └──────────────────┘   │  │    │

│  │  │                         │  │    │

│  │  └─────────────────────────┘  │    │### 1. Rust Implementation (src/lib.rs)Input:  {"expression": "10 * 5 + 3"}

│  └───────────────────────────────┘    │

└─────────────────────────────────────────┘Output: {

```

The calculator exports functions that the Go host can call:  "result": 53

## Troubleshooting

}

### Build fails with "cargo: command not found"

Use the Docker method (recommended), or install Rust from https://rustup.rs/```rust```



### Docker not running#[no_mangle]

Make sure Docker is installed and running:

```bashpub extern "C" fn allocate(size: usize) -> *mut u8 {## How It Works

docker info

```    // Allocate memory for input



### WASM execution errors}1. **Python Source** → The `calculator.py` script reads JSON from stdin, performs calculation, writes JSON to stdout

Check that:

1. You built with `--target wasm32-unknown-unknown`2. **Compilation** → Pyodide compiles Python to WebAssembly bytecode

2. The `[lib] crate-type = ["cdylib"]` is set in Cargo.toml

3. Functions are marked with `#[no_mangle]` and `extern "C"`#[no_mangle]3. **Loading** → `wasm.NewWASMTool()` loads and validates the WASM module

4. Global state is properly managed (see `RESULT` variable)

pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) {4. **Schema** → We provide the tool schema externally via `WithSchema()`

### Warning: "creating a shared reference to mutable static"

This is expected and safe in our single-threaded WASM context. The warning can be suppressed by using `&raw const` in future Rust versions.    // Parse JSON, evaluate expression, store result5. **Execution** → `tool.Call()` instantiates the module, passes JSON input, reads JSON output



## Extending the Example}6. **Isolation** → Wazero enforces the sandbox policy at the kernel level



To create your own Rust WASM tools:



1. **Create a new Rust library:**#[no_mangle]## Security

   ```bash

   cargo new --lib my_toolpub extern "C" fn get_result_ptr() -> *const u8 {

   ```

    // Return pointer to resultThe WASM tool runs with `ComputeOnlyPolicy()`:

2. **Configure Cargo.toml:**

   ```toml}

   [lib]

   crate-type = ["cdylib"]```go

   

   [dependencies]#[no_mangle]policy := wasm.ComputeOnlyPolicy()

   serde = { version = "1.0", features = ["derive"] }

   serde_json = "1.0"pub extern "C" fn get_result_len() -> usize {// - Timeout: 10 seconds

   

   [profile.release]    // Return length of result// - Max Memory: 200MB

   opt-level = "z"

   lto = true}// - No network

   strip = true

   panic = "abort"```// - No filesystem

   codegen-units = 1

   ```// - No environment variables



3. **Implement required exports in src/lib.rs:**The calculator:// - Stdout/stderr allowed for debugging

   ```rust

   #[no_mangle]1. Reads JSON input from WASM memory```

   pub extern "C" fn allocate(size: usize) -> *mut u8 { ... }

   2. Parses arithmetic expressions using a recursive descent parser

   #[no_mangle]

   pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) { ... }3. Evaluates expressions with support for +, -, *, /, and parenthesesThis ensures that even if the Python code is malicious, it cannot:

   

   #[no_mangle]4. Returns JSON output via global state- Exfiltrate data over the network

   pub extern "C" fn get_result_ptr() -> *const u8 { ... }

   - Read sensitive files

   #[no_mangle]

   pub extern "C" fn get_result_len() -> usize { ... }### 2. Cargo Configuration- Execute arbitrary system commands

   ```

- Consume excessive resources

4. **Build to WASM:**

   ```bashThe `Cargo.toml` is optimized for small WASM binaries:

   rustup target add wasm32-unknown-unknown

   cargo build --release --target wasm32-unknown-unknown## Customizing the Policy

   ```

```toml

5. **Use with AgentMesh:**

   ```go[lib]```go

   tool, err := wasm.NewWASMTool(

       wasmBytes,crate-type = ["cdylib"]  # Dynamic library for WASM// Create a custom policy

       schema,

       wasm.ComputeOnlyPolicy(),policy := wasm.DefaultSandboxPolicy()

   )

   ```[profile.release]policy.Timeout = 30 * time.Second



## Other Languagesopt-level = "z"      # Optimize for sizepolicy.MaxMemory = 500 * 1024 * 1024 // 500MB



While this example uses Rust, you can compile many languages to WASM:lto = true          # Link-time optimizationpolicy.AllowStdout = false            // Disable output



| Language | Binary Size | Maturity | Recommendation |strip = true        # Strip debug symbols

|----------|-------------|----------|----------------|

| **Rust** | ~100-200KB | ⭐⭐⭐⭐⭐ | **Best choice** |panic = "abort"     # Smaller panic handlertool, err := wasm.NewWASMTool(ctx, "calculator", "...", wasmBytes,

| Go (TinyGo) | ~800KB | ⭐⭐⭐⭐ | Good alternative |

| C/C++ | ~50-150KB | ⭐⭐⭐⭐⭐ | Great for existing code |codegen-units = 1   # Better optimization    wasm.WithPolicy(policy),

| AssemblyScript | ~50-100KB | ⭐⭐⭐ | TypeScript-like syntax |

```)

## Performance

```

Benchmark on Apple M1:

These settings reduce the binary from ~400KB to ~128KB.

- **Binary size**: 128KB (optimized)

- **Load time**: ~2-5ms (first time), <1ms (cached)## Creating Your Own Python WASM Tools

- **Execution time**: <1ms for simple expressions

- **Memory usage**: <1MB for typical operations### 3. Go Integration (main.go)



## References1. Write a Python script that:



- [Rust and WebAssembly Book](https://rustwasm.github.io/docs/book/)The Go code:   - Reads JSON from `sys.stdin.read()`

- [wasm32-unknown-unknown target](https://doc.rust-lang.org/rustc/platform-support/wasm32-unknown-unknown.html)

- [Wazero Runtime](https://wazero.io/)1. Loads the WASM binary   - Processes the input

- [AgentMesh WASM Design](../../SANDBOX.md)

2. Creates a `WASMTool` with schema and security policy   - Writes JSON result to `stdout`

3. Calls the tool like any other AgentMesh tool

4. The `WASMTool` handles all memory management and sandboxing2. Compile to WASM using TinyGo:

   ```bash

## Security Features   # Using Docker (no installation needed)

   docker run --rm -v $(pwd):/src -w /src tinygo/tinygo:0.33.0 \

The WASM sandbox provides kernel-level isolation:     tinygo build -o your_tool.wasm -target=wasi your_tool.go

   

1. **Memory Isolation** - WASM has its own linear memory, isolated from the host   # Or locally if TinyGo is installed

2. **No System Calls** - Cannot make syscalls unless explicitly granted   tinygo build -o your_tool.wasm -target=wasi your_tool.go

3. **Deterministic Execution** - Same input always produces same output   ```

4. **Resource Limits** - Memory and timeout enforced by the runtime

5. **Cannot Be Bypassed** - Enforced by the Wazero runtime, not user-space code3. Load in Go:

   ```go

The example uses `ComputeOnlyPolicy()`:   wasmBytes, _ := os.ReadFile("your_tool.wasm")

- ❌ No filesystem access   tool, _ := wasm.NewWASMTool(ctx, "tool_name", "description", wasmBytes,

- ❌ No network access         wasm.WithSchema(schema),

- ❌ No stdin/stdout/stderr       wasm.WithPolicy(wasm.ComputeOnlyPolicy()),

- ✅ Deterministic mode enabled   )

- ✅ 200MB memory limit   ```

- ✅ 10 second timeout

4. Use with LLM agents:

## Architecture   ```go

   // The tool implements the tool.Tool interface

```   // Pass it to your agent just like any other tool

┌─────────────────────────────────────────┐   agent := agent.NewReActAgent(model, []tool.Tool{tool})

│         AgentMesh (Go)                  │   ```

│                                         │

│  ┌───────────────────────────────┐    │## Troubleshooting

│  │      WASMTool                 │    │

│  │  ┌─────────────────────────┐  │    │**WASM module fails to load:**

│  │  │   Wazero Runtime        │  │    │- Ensure the WASM file is compiled with Pyodide or compatible runtime

│  │  │                         │  │    │- Check that the module exports the required functions

│  │  │  ┌──────────────────┐   │  │    │

│  │  │  │  WASM Module     │   │  │    │**Execution timeout:**

│  │  │  │  (Rust code)     │   │  │    │- Increase the timeout in the policy

│  │  │  │                  │   │  │    │- Optimize your Python code

│  │  │  │  - Isolated mem  │   │  │    │

│  │  │  │  - No syscalls   │   │  │    │**Memory errors:**

│  │  │  │  - Enforced caps │   │  │    │- Increase MaxMemory in the policy

│  │  │  └──────────────────┘   │  │    │- Reduce memory usage in your Python script

│  │  │                         │  │    │

│  │  └─────────────────────────┘  │    │## Next Steps

│  └───────────────────────────────┘    │

└─────────────────────────────────────────┘- Explore other sandbox policies: `NetworkOnlyPolicy()`, `FileProcessingPolicy()`

```- Create more complex tools (API clients, data processors, etc.)

- Integrate with AgentMesh agents for autonomous task execution

## Troubleshooting

### Build fails with "cargo: command not found"
Use the Docker method (recommended), or install Rust from https://rustup.rs/

### Docker not running
Make sure Docker is installed and running:
```bash
docker info
```

### WASM execution errors
Check that:
1. You built with `--target wasm32-unknown-unknown`
2. The `[lib] crate-type = ["cdylib"]` is set in Cargo.toml
3. Functions are marked with `#[no_mangle]` and `extern "C"`
4. Global state is properly managed (see `RESULT` variable)

### Warning: "creating a shared reference to mutable static"
This is expected and safe in our single-threaded WASM context. The warning can be suppressed by using `&raw const` in future Rust versions.

## Extending the Example

To create your own Rust WASM tools:

1. **Create a new Rust library:**
   ```bash
   cargo new --lib my_tool
   ```

2. **Configure Cargo.toml:**
   ```toml
   [lib]
   crate-type = ["cdylib"]
   
   [dependencies]
   serde = { version = "1.0", features = ["derive"] }
   serde_json = "1.0"
   
   [profile.release]
   opt-level = "z"
   lto = true
   strip = true
   panic = "abort"
   codegen-units = 1
   ```

3. **Implement required exports in src/lib.rs:**
   ```rust
   #[no_mangle]
   pub extern "C" fn allocate(size: usize) -> *mut u8 { ... }
   
   #[no_mangle]
   pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) { ... }
   
   #[no_mangle]
   pub extern "C" fn get_result_ptr() -> *const u8 { ... }
   
   #[no_mangle]
   pub extern "C" fn get_result_len() -> usize { ... }
   ```

4. **Build to WASM:**
   ```bash
   rustup target add wasm32-unknown-unknown
   cargo build --release --target wasm32-unknown-unknown
   ```

5. **Use with AgentMesh:**
   ```go
   tool, err := wasm.NewWASMTool(
       wasmBytes,
       schema,
       wasm.ComputeOnlyPolicy(),
   )
   ```

## Other Languages

While this example uses Rust, you can compile many languages to WASM:

| Language | Binary Size | Maturity | Recommendation |
|----------|-------------|----------|----------------|
| **Rust** | ~100-200KB | ⭐⭐⭐⭐⭐ | **Best choice** |
| Go (TinyGo) | ~800KB | ⭐⭐⭐⭐ | Good alternative |
| C/C++ | ~50-150KB | ⭐⭐⭐⭐⭐ | Great for existing code |
| AssemblyScript | ~50-100KB | ⭐⭐⭐ | TypeScript-like syntax |
| Python | 75MB+ | ⭐⭐ | Experimental, large binaries |

## Performance

Benchmark on Apple M1:

- **Binary size**: 128KB (optimized)
- **Load time**: ~2-5ms (first time), <1ms (cached)
- **Execution time**: <1ms for simple expressions
- **Memory usage**: <1MB for typical operations

## References

- [Rust and WebAssembly Book](https://rustwasm.github.io/docs/book/)
- [wasm32-unknown-unknown target](https://doc.rust-lang.org/rustc/platform-support/wasm32-unknown-unknown.html)
- [Wazero Runtime](https://wazero.io/)
- [AgentMesh WASM Design](../../SANDBOX.md)
