#!/bin/bash

cargo build --target wasm32-unknown-unknown --release
substreams pack ./substreams.yaml
mv substreams-test-v0.1.0.spkg ../
