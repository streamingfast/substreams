#!/bin/bash

set -e

# Build the Rust code for WASM target
cargo build --target wasm32-unknown-unknown --release

# Create the .spkg file using substreams pack
substreams pack substreams.yaml
