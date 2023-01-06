#!/bin/bash -xe

# See https://wasmedge.org/book/en/sdk/go/function.html
# And https://wasmedge.org/book/en/write_wasm/rust/bindgen.html

cargo build --target wasm32-wasi --release
cp target/wasm32-wasi/release/eth_xfer.wasm .