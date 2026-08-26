#!/usr/bin/env bash
# Builds the wasm and packs it into thinstore-v0.2.0.spkg.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
cargo build --target wasm32-unknown-unknown --release
substreams pack -o thinstore-v0.2.0.spkg substreams.yaml
