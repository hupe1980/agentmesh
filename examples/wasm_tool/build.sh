#!/bin/bash
set -e

echo "Building calculator to WebAssembly using Rust..."

# Create lib structure if needed
if [ ! -d "src" ]; then
    mkdir -p src
    if [ -f "calculator.rs" ]; then
        cp calculator.rs src/lib.rs
    fi
fi

if command -v docker &> /dev/null; then
    echo "Using Rust container..."
    docker run --rm \
        -v "$(pwd)":/usr/src/app \
        -w /usr/src/app \
        rust:1.83 \
        bash -c "rustup target add wasm32-unknown-unknown && cargo build --release --target wasm32-unknown-unknown && cp target/wasm32-unknown-unknown/release/calculator.wasm ."
else
    echo "Using local Rust..."
    rustup target add wasm32-unknown-unknown
    cargo build --release --target wasm32-unknown-unknown
    cp target/wasm32-unknown-unknown/release/calculator.wasm .
fi

echo ""
echo "✅ Build complete! WASM module: calculator.wasm"
echo "   Size: $(du -h calculator.wasm | cut -f1)"
echo ""
echo "Run the example with: go run main.go"
