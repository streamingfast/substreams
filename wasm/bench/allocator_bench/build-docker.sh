#!/bin/bash
set -e

# Docker-based build script for WASM allocator benchmarks
ALLOCATOR=$1
OUTPUT_DIR="wasm_variants"

if [ -z "$ALLOCATOR" ]; then
  echo "Usage: $0 <allocator>"
  echo "  allocator: default, talc, rlsf, lol, mini"
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

# Build command
if [ "$ALLOCATOR" = "default" ]; then
  FEATURES=""
else
  FEATURES="--features alloc-${ALLOCATOR}"
fi

echo "Building with allocator: $ALLOCATOR (features: $FEATURES)"

# Run cargo build in Docker
sudo docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  rust:1.93 \
  bash -c "rustup target add wasm32-unknown-unknown && cargo build --target wasm32-unknown-unknown --release $FEATURES"

# Copy the output
cp target/wasm32-unknown-unknown/release/substreams.wasm "$OUTPUT_DIR/substreams-${ALLOCATOR}.wasm"

# Report size
SIZE=$(stat -c%s "$OUTPUT_DIR/substreams-${ALLOCATOR}.wasm" 2>/dev/null || stat -f%z "$OUTPUT_DIR/substreams-${ALLOCATOR}.wasm")
echo "Built: $OUTPUT_DIR/substreams-${ALLOCATOR}.wasm ($SIZE bytes)"
