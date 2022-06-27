# Rust APIs

### Substreams crates

The official [`substreams`](https://crates.io/crates/substreams) crate helps you develop module handlers.

There are also chain-specific `substreams-[network]` crates available:

* [`substreams-ethereum`](https://crates.io/crates/substreams-ethereum) for Ethereum and other EVM chains
* [`substreams-solana`](https://crates.io/crates/substreams-solana) for Solana

Substreams should also be available for NEAR, Cosmos Hub, Osmosis (or perhaps Soon:tm:). Chain-specific libraries provide optional helpers, but the main `substreams` crate is sufficient to start.

### Third-party libraries

You can pull in any third-party library that is able to compile to the `wasm32` target necessary for execution in Substreams services. **However**, many libraries try to compile kernel syscalls or other operations which are not available within the Substreams execution environment, and will therefore not compile as a `wasm32` target.

Here are libraries to help you get off the ground for certain tasks:

* [https://docs.rs/tiny-keccak](https://docs.rs/tiny-keccak)
