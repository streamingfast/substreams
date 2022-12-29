---
description: StreamingFast Substreams Rust APIs
---

# Rust crates

### Substreams crates

The official [`substreams` crate](https://crates.io/crates/substreams) helps developers create module handlers.

There are also chain-specific `substreams-[network]` crates available:

* [`substreams-ethereum`](https://crates.io/crates/substreams-ethereum) for Ethereum and other Ethereum-compatible chains
* [`substreams-solana`](https://crates.io/crates/substreams-solana) for Solana

If a crate is not available for Substreams, you can use the `spkg` release for the chain, which includes the `Block` Protobuf model, and generate the Rust structs yourself.

### Third-party libraries

Any third-party library capable of compiling `wasm32` can be used for execution in Substreams services.&#x20;

Some libraries include kernel syscalls or other operations that are not available in the Substreams execution environment and cannot be compiled to WASM. Keep this in mind when selecting libraries to include in your Substreams project.

Here's a very inexhaustive list of things people found useful:

* [`tiny_keccak`](https://docs.rs/tiny-keccak): an implementation of Keccak-derived functions specified in FIPS-202, SP800-185, and KangarooTwelve.
