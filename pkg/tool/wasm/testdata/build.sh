#!/bin/bash
set -e

echo "Building malicious test WASM modules..."

MODULES=(
    "simple_math"
    "network_attempt"
    "timeout_bomb"
    "memory_bomb"
    "filesystem_escape"
    "non_deterministic"
)

# Build each module in its own directory
for module in "${MODULES[@]}"; do
    echo ""
    echo "Building $module..."
    
    if [ ! -d "${module}" ]; then
        echo "❌ Module directory ${module}/ not found, skipping..."
        continue
    fi
    
    cd "${module}"
    
    if command -v docker &> /dev/null; then
        echo "Using Rust container..."
        docker run --rm \
            -v "$(pwd)":/usr/src/app \
            -w /usr/src/app \
            rust:1.83 \
            bash -c "rustup target add wasm32-unknown-unknown && cargo build --release --target wasm32-unknown-unknown" 2>&1 | grep -E "(Compiling|Finished|warning:)" || true
    else
        echo "Using local Rust..."
        rustup target add wasm32-unknown-unknown
        cargo build --release --target wasm32-unknown-unknown
    fi
    
    # Copy to parent directory for easy access
    if [ -f "target/wasm32-unknown-unknown/release/${module}.wasm" ]; then
        cp "target/wasm32-unknown-unknown/release/${module}.wasm" ../
        echo "✅ ${module}.wasm ($(du -h ../!${module}.wasm | cut -f1))"
    else
        echo "❌ Failed to build ${module}.wasm"
    fi
    
    cd ..
done

echo ""
echo "✅ Build complete! All WASM modules built."
echo ""
echo "Built modules:"
ls -lh *.wasm 2>/dev/null | awk '{print "  - " $9 " (" $5 ")"}'
